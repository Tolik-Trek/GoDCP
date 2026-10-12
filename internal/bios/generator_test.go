package bios

import (
	"reflect"
	"strings"
	"testing"
)

func sampleRule() Rule {
	return Rule{
		Port: 0x204E, Code: 0xC3,
		CNF: []int{0, 1, 2, 3}, Directions: []bool{false},
		DOSOff: []bool{false, true}, PN5: []bool{false, true},
	}
}

func TestAllContextsOUT(t *testing.T) {
	records, err := Generate(sampleRule())
	if err != nil {
		t.Fatal(err)
	}
	want := []Record{{Address: 0x0056, Mask: 0x03FF, Code: 0xC3}}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v; want %#v", records, want)
	}
	if count := len(records[0].Offsets()); count != 16 {
		t.Fatalf("covered %d page entries; want 16", count)
	}
	wantValues := "HL = %00'000'0'0101'0110\nDE = %00'001'1'1111'1111\nB  = %1100'0011"
	if got := Values(records); got != wantValues {
		t.Errorf("Values = %q; want %q", got, wantValues)
	}
	wantTable := "DW %00'000'0'0101'0110\nDW %00'001'1'1111'1111\nDB %1100'0011"
	if got := Table(records); got != wantTable {
		t.Errorf("Table = %q; want %q", got, wantTable)
	}
}

func TestOppositeCNFMapsStaySeparate(t *testing.T) {
	rule := sampleRule()
	rule.CNF = []int{0, 3}
	records, err := Generate(rule)
	if err != nil {
		t.Fatal(err)
	}
	want := []Record{
		{Address: 0x0056, Mask: 0x33FF, Code: 0xC3},
		{Address: 0x3056, Mask: 0x33FF, Code: 0xC3},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v; want %#v", records, want)
	}
	if !strings.Contains(Values(records), "\n\nHL = ") || !strings.Contains(Table(records), "\n\nDW ") {
		t.Error("multiple records should be separated by a blank line")
	}
}

func boolSelection(bits int) []bool {
	var values []bool
	if bits&1 != 0 {
		values = append(values, false)
	}
	if bits&2 != 0 {
		values = append(values, true)
	}
	return values
}

// Exercise every nonempty combination of context selections. The expected
// membership is checked independently for every byte in the 16 KiB page;
// this catches both accidental scope expansion and gaps in split rules.
func TestEverySelectionHasExactDisjointCoverage(t *testing.T) {
	for maps := 1; maps < 16; maps++ {
		var cnf []int
		for n := 0; n < 4; n++ {
			if maps&(1<<n) != 0 {
				cnf = append(cnf, n)
			}
		}
		for directions := 1; directions < 4; directions++ {
			for dos := 1; dos < 4; dos++ {
				for pn5 := 1; pn5 < 4; pn5++ {
					rule := Rule{
						Port: 0x204E, Code: 0xC3, CNF: cnf,
						Directions: boolSelection(directions), DOSOff: boolSelection(dos), PN5: boolSelection(pn5),
					}
					records, err := Generate(rule)
					if err != nil {
						t.Fatal(err)
					}
					var visits [0x4000]int
					for _, record := range records {
						if record.Code != 0xC3 || record.Address & ^record.Mask != 0 || record.Mask > 0x3FFF {
							t.Fatalf("noncanonical record: %#v", record)
						}
						for _, offset := range record.Offsets() {
							visits[offset]++
						}
					}
					for offset, visited := range visits {
						want := 0
						if offset&0x1FF == 0x56 &&
							maps&(1<<(offset>>12)) != 0 &&
							directions&(1<<((offset>>9)&1)) != 0 &&
							dos&(1<<((offset>>10)&1)) != 0 &&
							pn5&(1<<((offset>>11)&1)) != 0 {
							want = 1
						}
						if visited != want {
							t.Fatalf("maps=%04b dir=%02b dos=%02b pn5=%02b offset=%04X visits=%d; want %d", maps, directions, dos, pn5, offset, visited, want)
						}
					}
				}
			}
		}
	}
}

func TestDuplicateSelectionsAreNormalized(t *testing.T) {
	rule := sampleRule()
	want, err := Generate(rule)
	if err != nil {
		t.Fatal(err)
	}
	rule.CNF = []int{3, 1, 0, 3, 2, 0}
	rule.Directions = []bool{false, false}
	rule.DOSOff = []bool{true, false, true}
	rule.PN5 = []bool{true, true, false}
	got, err := Generate(rule)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records = %#v; want %#v", got, want)
	}
}

func TestInvalidSelections(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Rule)
	}{
		{"no maps", func(r *Rule) { r.CNF = nil }},
		{"negative map", func(r *Rule) { r.CNF = []int{-1} }},
		{"map out of range", func(r *Rule) { r.CNF = []int{4} }},
		{"no directions", func(r *Rule) { r.Directions = nil }},
		{"no DOS", func(r *Rule) { r.DOSOff = nil }},
		{"no PN5", func(r *Rule) { r.PN5 = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := sampleRule()
			tc.edit(&rule)
			if records, err := Generate(rule); err == nil || len(records) != 0 {
				t.Fatalf("Generate = %#v, %v; want no records and an error", records, err)
			}
		})
	}
}

func TestExternalAddressExamplesAndAliases(t *testing.T) {
	for _, tc := range []struct {
		port, address uint16
	}{
		{0x0000, 0x0000}, {0xFFFF, 0x01FF}, {0x204E, 0x0056},
		{0x7786, 0x009E}, {0x0050, 0x0040}, {0x0058, 0x0040},
	} {
		rule := sampleRule()
		rule.Port = tc.port
		records, err := Generate(rule)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 || records[0].Address != tc.address {
			t.Fatalf("port %04X generated %#v; want address %04X", tc.port, records, tc.address)
		}
	}
}

func TestOffsetsMatchBIOSMasking(t *testing.T) {
	// DCP_CONFIG masks HL with DE, then forces the high two bits to select
	// the RAM slot. Those slot bits do not form part of a page offset.
	r := Record{Address: 0xF056, Mask: 0xC3FF}
	offsets := r.Offsets()
	if len(offsets) != 16 || offsets[0] != 0x56 || offsets[len(offsets)-1] != 0x3C56 {
		t.Fatalf("unexpected offsets: %04X", offsets)
	}
	if Values(nil) != "" || Table(nil) != "" {
		t.Error("empty lists should format as empty text")
	}
}
