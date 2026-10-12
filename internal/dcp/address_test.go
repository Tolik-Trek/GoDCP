package dcp

import (
	"testing"
)

func TestOffsetDocumentedExamples(t *testing.T) {
	tests := []struct {
		port    uint16
		context Context
		want    int
	}{
		{0x204E, Context{}, 0x0056},
		{0x204E, Context{Read: true}, 0x0256},
		{0xFFFD, Context{}, 0x01FD},
		{0xFFFD, Context{Read: true}, 0x03FD},
		{0xBFFD, Context{}, 0x017D},
		{0x7786, Context{Read: true, DOSOff: true}, 0x069E},
		{0x00E5, Context{DOSOff: true}, 0x046D},
		{0xFFFF, Context{CNF: 3, Read: true, DOSOff: true, PN5: true}, 0x3FFF},
	}
	for _, tc := range tests {
		if got := Offset(tc.port, tc.context); got != tc.want {
			t.Errorf("Offset(%04X, %+v) = %04X, want %04X", tc.port, tc.context, got, tc.want)
		}
	}
	if Canonical(0x7786) != 0x6086 {
		t.Error("canonical address differs from spreadsheet example")
	}
}

func TestEveryOffsetRoundTrip(t *testing.T) {
	seen := make(map[struct {
		port    uint16
		context Context
	}]bool, Size)
	for offset := 0; offset < Size; offset++ {
		p, c := Decode(offset)
		if got := Offset(p, c); got != offset {
			t.Fatalf("offset %04X decoded to %04X %+v, encoded as %04X", offset, p, c, got)
		}
		if p != Canonical(p) {
			t.Fatalf("Decode(%04X) returned non-canonical %04X", offset, p)
		}
		key := struct {
			port    uint16
			context Context
		}{p, c}
		if seen[key] {
			t.Fatalf("duplicate decoded selection for %04X", offset)
		}
		seen[key] = true
	}
}

func TestAllPortsAgainstBitWiring(t *testing.T) {
	// This independent bit-by-bit oracle follows the wiring table rather than
	// sharing shifts/masks with Offset or relying only on inverse round trips.
	portBits := []uint{0, 1, 2, 7, 13, 5, 6, 14, 15}
	for external := 0; external <= 0xFFFF; external++ {
		compact := 0
		for target, source := range portBits {
			compact |= ((external >> source) & 1) << target
		}
		for contextBits := 0; contextBits < 32; contextBits++ {
			context := Context{CNF: contextBits >> 3, Read: contextBits&1 != 0, DOSOff: contextBits&2 != 0, PN5: contextBits&4 != 0}
			want := compact | contextBits<<9
			if got := Offset(uint16(external), context); got != want {
				t.Fatalf("port %04X context %+v = %04X, want %04X", external, context, got, want)
			}
		}
	}
}

func TestAllAliasGroups(t *testing.T) {
	seen := make([]bool, 65536)
	for compact := 0; compact < 512; compact++ {
		canonical, _ := Decode(compact)
		aliases := Aliases(canonical)
		if len(aliases) != 128 || aliases[0] != canonical {
			t.Fatalf("unexpected alias group for %04X", canonical)
		}
		for i, p := range aliases {
			if seen[p] || Offset(p, Context{}) != compact {
				t.Fatalf("invalid or duplicate alias %04X for %04X", p, canonical)
			}
			if i > 0 && aliases[i-1] >= p {
				t.Fatalf("aliases are not sorted for %04X", canonical)
			}
			seen[p] = true
		}
	}
	for p, found := range seen {
		if !found {
			t.Fatalf("external port %04X was not assigned to an alias group", p)
		}
	}
}

func TestNumberParsingAndBIOSFormat(t *testing.T) {
	for _, input := range []string{"204E", "#204e", "0x204E", "204Eh", " 204eH ", "%0010'0000'0100'1110"} {
		got, err := ParseNumber(input, 16)
		if err != nil || got != 0x204E {
			t.Errorf("ParseNumber(%q) = %04X, %v", input, got, err)
		}
	}
	if got, _ := ParseNumber("10", 8); got != 0x10 {
		t.Fatal("unprefixed values must default to hex")
	}
	for _, input := range []string{"", "#", "%", "GG", "-1", "+1", "100", "%1111'11111", "%102", "20 4E"} {
		if _, err := ParseNumber(input, 8); err == nil {
			t.Errorf("accepted invalid 8-bit input %q", input)
		}
	}
	for _, bits := range []int{-1, 0, 17} {
		if _, err := ParseNumber("0", bits); err == nil {
			t.Errorf("accepted invalid bit width %d", bits)
		}
	}
	if got := Binary14(0x56); got != "%00'000'0'0101'0110" {
		t.Errorf("wrong HL BIOS format: %s", got)
	}
	if got := Binary14(0x3FF); got != "%00'001'1'1111'1111" {
		t.Errorf("wrong DE BIOS format: %s", got)
	}
	if got := Binary8(0xC3); got != "%1100'0011" {
		t.Errorf("wrong B BIOS format: %s", got)
	}
	for value := 0; value < Size; value++ {
		got, err := ParseNumber(Binary14(uint16(value)), 14)
		if err != nil || got != uint16(value) {
			t.Fatalf("binary formatting failed to round-trip %04X", value)
		}
	}
}
