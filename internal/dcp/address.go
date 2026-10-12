// Package dcp implements the Sprinter port map's binary addressing and document
// model. It does not infer a BIOS or FPGA version from the loaded bytes.
package dcp

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// Size is the complete page: four maps of 4096 byte assignments.
	Size = 16384
	// AddressMask selects the nine CPU address bits decoded by the port map.
	AddressMask uint16 = 0xE0E7
)

// Context contains every non-address input used to select a map byte.
// Read=false means OUT; DOSOff=false means DOS is enabled (/DOS=0).
type Context struct {
	CNF    int
	Read   bool
	DOSOff bool
	PN5    bool
}

// Offset returns the byte selected by an external port and its context.
// CNF must be in [0,3]; an invalid context is a programming error.
func Offset(port uint16, c Context) int {
	if c.CNF < 0 || c.CNF > 3 {
		panic("dcp: CNF must be between 0 and 3")
	}
	o := int(port&7 | (port>>4)&8 | (port>>9)&0x10 | port&0x60 | (port>>7)&0x180)
	o |= c.CNF << 12
	if c.Read {
		o |= 1 << 9
	}
	if c.DOSOff {
		o |= 1 << 10
	}
	if c.PN5 {
		o |= 1 << 11
	}
	return o
}

// Decode returns the smallest external address and the context selecting offset.
// Offsets outside the document are programming errors.
func Decode(offset int) (uint16, Context) {
	if offset < 0 || offset >= Size {
		panic("dcp: offset is outside the port map")
	}
	o := uint16(offset)
	p := o&7 | (o&8)<<4 | (o&0x10)<<9 | o&0x60 | (o&0x180)<<7
	return p, Context{
		CNF:    offset >> 12,
		Read:   offset&(1<<9) != 0,
		DOSOff: offset&(1<<10) != 0,
		PN5:    offset&(1<<11) != 0,
	}
}

// Canonical returns the smallest external address in an alias group.
func Canonical(port uint16) uint16 { return port & AddressMask }

// Aliases returns all 128 external addresses of the group, in ascending order.
func Aliases(port uint16) []uint16 {
	base := Canonical(port)
	ports := make([]uint16, 128)
	for i := range ports {
		ports[i] = base | uint16(i&3)<<3 | uint16(i>>2)<<8
	}
	return ports
}

// ParseNumber accepts hexadecimal by default, including #FF, 0xFF and FFh.
// Binary input starts with %, and digit groups may use apostrophes or underscores.
func ParseNumber(text string, bits int) (uint16, error) {
	if bits < 1 || bits > 16 {
		return 0, fmt.Errorf("bit width must be between 1 and 16")
	}
	s := strings.TrimSpace(text)
	base := 16
	switch {
	case strings.HasPrefix(s, "%"):
		base, s = 2, s[1:]
	case strings.HasPrefix(s, "#"):
		s = s[1:]
	case strings.HasPrefix(strings.ToLower(s), "0x"):
		s = s[2:]
	case strings.HasSuffix(strings.ToLower(s), "h"):
		s = s[:len(s)-1]
	}
	s = strings.NewReplacer("'", "", "_", "").Replace(s)
	if s == "" || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("enter a hexadecimal or %%binary value")
	}
	v, err := strconv.ParseUint(s, base, bits)
	if err != nil {
		return 0, fmt.Errorf("invalid %d-bit value %q", bits, text)
	}
	return uint16(v), nil
}

// Binary14 formats a DCP address/mask like the BIOS initialization table:
// CNF / PN5,DOSOff,Read / P15 / P14,P6,P5,P13 / P7,P2,P1,P0.
func Binary14(v uint16) string {
	s := fmt.Sprintf("%014b", v&0x3FFF)
	return "%" + s[:2] + "'" + s[2:5] + "'" + s[5:6] + "'" + s[6:10] + "'" + s[10:]
}

// Binary8 formats one internal port code as two groups of four bits.
func Binary8(v byte) string {
	s := fmt.Sprintf("%08b", v)
	return "%" + s[:4] + "'" + s[4:]
}
