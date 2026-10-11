//go:build linux && amd64

// Command crawler-browser-launcher applies a process-only network sandbox before starting Chrome.
package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/Laisky/errors/v2"
	"golang.org/x/sys/unix"
)

// main preserves CDP pipe descriptors, installs inherited restrictions, and executes the installed browser.
func main() {
	if err := run(os.Args[1:]); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, "crawler browser launcher failed:", err); writeErr != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// run validates only the launcher-owned descriptors and starts Chrome with a minimal environment.
func run(args []string) error {
	const browser = "/usr/bin/google-chrome"
	arguments := append([]string{browser}, args...)
	environment := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "TZ=UTC"}
	if _, err := os.Stat(browser); err != nil {
		return errors.Wrap(err, "installed Chrome unavailable")
	}
	if err := validateControlPipes(); err != nil {
		return errors.Wrap(err, "validate CDP pipes")
	}
	runtime.LockOSThread()
	if err := unix.CloseRange(5, ^uint(0), unix.CLOSE_RANGE_UNSHARE); err != nil {
		return errors.Wrap(err, "close unexpected inherited descriptors")
	}
	if err := installNetworkSandbox(); err != nil {
		return errors.Wrap(err, "install browser network sandbox")
	}
	return errors.Wrap(syscall.Exec(browser, arguments, environment), "execute isolated browser")
}

// validateControlPipes rejects socket-backed standard descriptors and requires the CDP command/response pipes.
func validateControlPipes() error {
	for fd := 0; fd <= 4; fd++ {
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil {
			return errors.Wrap(err, "inspect launcher descriptor")
		}
		kind := stat.Mode & unix.S_IFMT
		if kind == unix.S_IFSOCK {
			return errors.Errorf("inherited socket descriptor denied")
		}
		if fd >= 3 && kind != unix.S_IFIFO {
			return errors.Errorf("CDP descriptor must be a pipe")
		}
	}
	return nil
}

// installNetworkSandbox permits Unix IPC sockets and denies every Internet/other socket and io_uring interface.
// The architecture and x32 guards prevent switching ABI to bypass the inherited filter.
func installNetworkSandbox() error {
	deny := unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 4},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.AUDIT_ARCH_X86_64, Jt: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_KILL_PROCESS},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: 0x40000000, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_IO_URING_SETUP, Jt: 4},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_IO_URING_ENTER, Jt: 3},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_IO_URING_REGISTER, Jt: 2},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_SOCKET, Jt: 2},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_SOCKETPAIR, Jt: 1, Jf: 4},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 16},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.AF_UNIX, Jt: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: deny},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return errors.Wrap(err, "set launcher no-new-privileges")
	}
	result, _, errno := unix.RawSyscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&program)))
	runtime.KeepAlive(filter)
	if errno != 0 {
		return errors.Wrap(errno, "apply inherited seccomp filter")
	}
	if result != 0 {
		return errors.Errorf("seccomp thread synchronization failed")
	}
	return nil
}
