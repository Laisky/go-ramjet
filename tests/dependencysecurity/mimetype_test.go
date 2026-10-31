package dependencysecurity

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/stretchr/testify/require"
)

// TestMIMEDetectionControls checks ordinary file signatures retain their
// expected types when the detector limit is raised for CDF inspection.
func TestMIMEDetectionControls(t *testing.T) {
	mimetype.SetLimit(1 << 18)
	defer mimetype.SetLimit(4096)
	require.Equal(t, "application/pdf", mimetype.Detect([]byte("%PDF-1.7\n")).String())
	require.Equal(t, "image/png", mimetype.Detect([]byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10}).String())
	require.Equal(t, "text/plain; charset=utf-8", mimetype.Detect([]byte("ordinary readable text")).String())
}

// TestMIMEDetectionCraftedCDFBounds checks a looped CDF chain in a bounded
// subprocess so a vulnerable detector cannot continue consuming CPU.
func TestMIMEDetectionCraftedCDFBounds(t *testing.T) {
	const size = 1 << 18
	if os.Getenv("RAMJET_CDF_TEST_CHILD") == "1" {
		mimetype.SetLimit(size)
		require.Equal(t, "application/x-ole-storage", mimetype.Detect(craftedCDF(size)).String())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMIMEDetectionCraftedCDFBounds$")
	cmd.Env = append(os.Environ(), "RAMJET_CDF_TEST_CHILD=1")
	output, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "crafted CDF detection exceeded deadline")
	require.NoError(t, err, "child output: %s", output)
}

// craftedCDF constructs a bounded synthetic CDF with a self-looping short
// sector chain, exercising GHSA-jjmc-jx7j-hp68 without using real files.
func craftedCDF(size int) []byte {
	const sector = 128
	data := make([]byte, size)
	put16 := func(offset int, v uint16) { binary.LittleEndian.PutUint16(data[offset:], v) }
	put32 := func(offset int, v uint32) { binary.LittleEndian.PutUint32(data[offset:], v) }
	copy(data, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})
	put16(30, 7)
	put16(32, 0)
	put32(48, 4)
	put32(56, 4096)
	put32(60, 6)
	put32(68, 0xfffffffe)
	for i := 0; i < 109; i++ {
		put32(76+4*i, 0xffffffff)
	}
	put32(76, 3)
	offset := func(index int) int { return sector * (1 + index) }
	sat := offset(3)
	for i := 0; i < sector/4; i++ {
		put32(sat+4*i, 0xffffffff)
	}
	put32(sat+16, 5)
	put32(sat+20, 0xfffffffe)
	put32(sat+24, 6)
	entry := func(at int, name string, kind byte, first uint32, length uint32) {
		for i := range name {
			put16(at+2*i, uint16(name[i]))
		}
		put16(at+64, uint16(2*(len(name)+1)))
		data[at+66] = kind
		put32(at+68, 0xffffffff)
		put32(at+72, 0xffffffff)
		put32(at+76, 0xffffffff)
		put32(at+116, first)
		put32(at+120, length)
	}
	root := offset(4)
	entry(root, "Root Entry", 5, 6, 0)
	put32(root+76, 1)
	entry(offset(5), "\x05SummaryInformation", 2, uint32(size-2), 1)
	for i := 0; i < sector/4; i++ {
		put32(offset(6)+4*i, uint32(size-2))
	}
	return data
}
