//go:build linux && amd64

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestNetworkSandboxProcess runs kernel checks outside the unrestricted test runner and verifies exec inheritance.
func TestNetworkSandboxProcess(t *testing.T) {
	if phase := os.Getenv("CRAWLER_SANDBOX_TEST_PHASE"); phase != "" {
		runtime.LockOSThread()
		if phase == "install" {
			ready, probe := make(chan struct{}), make(chan struct{})
			result := make(chan error, 1)
			go func() {
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				close(ready)
				<-probe
				fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
				if fd >= 0 {
					unix.Close(fd)
				}
				result <- err
			}()
			<-ready
			require.NoError(t, installNetworkSandbox())
			close(probe)
			require.ErrorIs(t, <-result, unix.EPERM, "existing thread synchronized")
		}
		for _, domain := range []int{unix.AF_INET, unix.AF_INET6, unix.AF_PACKET, unix.AF_VSOCK} {
			for _, kind := range []int{unix.SOCK_STREAM, unix.SOCK_DGRAM, unix.SOCK_RAW} {
				fd, err := unix.Socket(domain, kind, 0)
				if fd >= 0 {
					require.NoError(t, unix.Close(fd))
				}
				require.ErrorIs(t, err, unix.EPERM, "domain=%d type=%d", domain, kind)
			}
		}
		pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
		require.NoError(t, err)
		require.NoError(t, unix.Close(pair[0]))
		require.NoError(t, unix.Close(pair[1]))
		for _, number := range []uintptr{unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER, 0x40000000 | unix.SYS_SOCKET} {
			_, _, errno := unix.RawSyscall(number, 0, 0, 0)
			require.Equal(t, unix.EPERM, errno)
		}
		privileges, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
		require.NoError(t, err)
		require.Equal(t, 1, privileges)
		if phase == "install" {
			cmd := exec.Command(os.Args[0], "-test.run=^TestNetworkSandboxProcess$")
			cmd.Env = append(os.Environ(), "CRAWLER_SANDBOX_TEST_PHASE=inherited")
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "exec inherited filter: %s", out)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestNetworkSandboxProcess$")
	cmd.Env = append(os.Environ(), "CRAWLER_SANDBOX_TEST_PHASE=install")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kernel sandbox subprocess: %s", out)
	// Parent networking remains available because restrictions belong only to the launcher process tree.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

// TestNetworkSandboxRejectsForeignArchitecture executes a reachable i386 syscall fixture under the inherited architecture guard.
func TestNetworkSandboxRejectsForeignArchitecture(t *testing.T) {
	if fixture := os.Getenv("CRAWLER_SANDBOX_I386_FIXTURE"); fixture != "" {
		runtime.LockOSThread()
		require.NoError(t, unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{}))
		require.NoError(t, installNetworkSandbox())
		require.NoError(t, syscall.Exec(fixture, []string{fixture}, []string{"PATH=/usr/bin:/bin"}))
		return
	}
	// A tiny static i386 ELF calls getpid and then exit(0), without accessing any network or files.
	code := []byte{0xb8, 20, 0, 0, 0, 0xcd, 0x80, 0xb8, 1, 0, 0, 0, 0x31, 0xdb, 0xcd, 0x80}
	fixture := make([]byte, 84+len(code))
	copy(fixture, []byte{0x7f, 'E', 'L', 'F', 1, 1, 1})
	binary.LittleEndian.PutUint16(fixture[16:], 2)
	binary.LittleEndian.PutUint16(fixture[18:], 3)
	binary.LittleEndian.PutUint32(fixture[20:], 1)
	binary.LittleEndian.PutUint32(fixture[24:], 0x08048054)
	binary.LittleEndian.PutUint32(fixture[28:], 52)
	binary.LittleEndian.PutUint16(fixture[40:], 52)
	binary.LittleEndian.PutUint16(fixture[42:], 32)
	binary.LittleEndian.PutUint16(fixture[44:], 1)
	for offset, value := range map[int]uint32{52: 1, 60: 0x08048000, 64: 0x08048000, 68: uint32(len(fixture)), 72: uint32(len(fixture)), 76: 5, 80: 4096} {
		binary.LittleEndian.PutUint32(fixture[offset:], value)
	}
	copy(fixture[84:], code)
	path := t.TempDir() + "/i386-syscall-control"
	require.NoError(t, os.WriteFile(path, fixture, 0700))
	controlErr := exec.Command(path).Run()
	if errors.Is(controlErr, syscall.ENOEXEC) {
		t.Skip("kernel does not execute i386; foreign ABI cannot bypass here")
	}
	require.NoError(t, controlErr, "unrestricted foreign ABI fixture must execute")
	cmd := exec.Command(os.Args[0], "-test.run=^TestNetworkSandboxRejectsForeignArchitecture$")
	cmd.Env = append(os.Environ(), "CRAWLER_SANDBOX_I386_FIXTURE="+path)
	err := cmd.Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.True(t, status.Signaled())
	require.Equal(t, syscall.SIGSYS, status.Signal())
}

// TestLauncherRequiresPipes rejects a launch lacking allocator-created CDP pipes before Chrome starts.
func TestLauncherRequiresPipes(t *testing.T) {
	require.Error(t, validateControlPipes())
}

// TestLauncherClosesUnexpectedDescriptors proves a passed Internet socket is closed before an inherited child starts.
func TestLauncherClosesUnexpectedDescriptors(t *testing.T) {
	if os.Getenv("CRAWLER_SANDBOX_TEST_PHASE") == "close-fds" {
		runtime.LockOSThread()
		require.NoError(t, validateControlPipes())
		var stat unix.Stat_t
		require.NoError(t, unix.Fstat(5, &stat))
		require.EqualValues(t, unix.S_IFSOCK, stat.Mode&unix.S_IFMT)
		require.NoError(t, unix.CloseRange(5, ^uint(0), unix.CLOSE_RANGE_UNSHARE))
		require.ErrorIs(t, unix.Fstat(5, &stat), unix.EBADF)
		require.NoError(t, installNetworkSandbox())
		_, err := fmt.Fprintln(os.Stdout, "unexpected socket closed")
		require.NoError(t, err)
		return
	}
	firstRead, firstWrite, err := os.Pipe()
	require.NoError(t, err)
	defer firstRead.Close()
	defer firstWrite.Close()
	secondRead, secondWrite, err := os.Pipe()
	require.NoError(t, err)
	defer secondRead.Close()
	defer secondWrite.Close()
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	socket := os.NewFile(uintptr(fd), "unexpected-internet-socket")
	defer socket.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLauncherClosesUnexpectedDescriptors$")
	cmd.Env = append(os.Environ(), "CRAWLER_SANDBOX_TEST_PHASE=close-fds")
	cmd.ExtraFiles = []*os.File{firstRead, secondWrite, socket}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "descriptor closure subprocess: %s", out)
	require.Contains(t, string(out), "unexpected socket closed")
}
