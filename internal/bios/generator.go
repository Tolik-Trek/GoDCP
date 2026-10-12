// Package bios calculates the register values accepted by Sprinter DCP_CONFIG.
// Its output describes rules; it never changes the loaded map.
package bios

import (
	"fmt"
	"sort"
	"strings"

	"godcp/internal/dcp"
)

// Rule selects one external port and the contexts to which the assignment
// applies. Directions uses false for OUT and true for IN. DOSOff describes
// the /DOS signal: false means DOS enabled, true means DOS disabled.
// Each selection must contain at least one value. Duplicate values are ignored.
type Rule struct {
	Port       uint16
	Code       byte
	CNF        []int
	Directions []bool
	DOSOff     []bool
	PN5        []bool
}

// Record contains the values for HL, DE, and B respectively. The low 14 bits
// address the 16 KiB DCP page. A set Mask bit fixes the corresponding address
// bit; a clear bit lets the BIOS visit both values.
type Record struct {
	Address uint16
	Mask    uint16
	Code    byte
}

// Generate returns deterministic, disjoint records covering exactly the
// selected contexts. Nonadjacent CNF selections may require multiple records.
// Address bits ignored by the hardware do not affect the result.
func Generate(rule Rule) ([]Record, error) {
	if len(rule.CNF) == 0 {
		return nil, fmt.Errorf("select at least one CNF map")
	}
	var selected uint8
	for _, cnf := range rule.CNF {
		if cnf < 0 || cnf > 3 {
			return nil, fmt.Errorf("CNF map %d is outside 0..3", cnf)
		}
		selected |= 1 << cnf
	}

	address := uint16(dcp.Offset(rule.Port, dcp.Context{}))
	mask := uint16(0x01FF) // all nine decoded external-address bits are fixed
	selections := []struct {
		name   string
		bit    uint
		values []bool
	}{
		{"direction", 9, rule.Directions},
		{"DOS state", 10, rule.DOSOff},
		{"PN5 state", 11, rule.PN5},
	}
	for _, selection := range selections {
		if len(selection.values) == 0 {
			return nil, fmt.Errorf("select at least one %s", selection.name)
		}
		var present [2]bool
		for _, value := range selection.values {
			if value {
				present[1] = true
			} else {
				present[0] = true
			}
		}
		if present[0] && present[1] {
			continue
		}
		mask |= 1 << selection.bit
		if present[1] {
			address |= 1 << selection.bit
		}
	}

	var records []Record
	for selected != 0 {
		// There are only two CNF bits. Select the largest cube contained in
		// the remaining set, then remove it so records cannot overlap.
		var bestAddress, bestMask, bestMembers uint8
		bestCount := 0
		for cubeMask := uint8(0); cubeMask < 4; cubeMask++ {
			for cubeAddress := uint8(0); cubeAddress < 4; cubeAddress++ {
				if cubeAddress & ^cubeMask != 0 {
					continue
				}
				var members uint8
				count := 0
				for cnf := uint8(0); cnf < 4; cnf++ {
					if cnf&cubeMask == cubeAddress {
						members |= 1 << cnf
						count++
					}
				}
				if members & ^selected == 0 && count > bestCount {
					bestAddress, bestMask, bestMembers, bestCount = cubeAddress, cubeMask, members, count
				}
			}
		}
		records = append(records, Record{
			Address: address | uint16(bestAddress)<<12,
			Mask:    mask | uint16(bestMask)<<12,
			Code:    rule.Code,
		})
		selected &^= bestMembers
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Address < records[j].Address })
	return records, nil
}

// Offsets lists the page offsets visited by DCP_CONFIG, in ascending order.
// The top two register bits are ignored, as the BIOS replaces them with the
// page's slot address. Address bits cleared in Mask are likewise ignored.
func (r Record) Offsets() []int {
	mask := r.Mask & 0x3FFF
	address := r.Address & mask
	offsets := make([]int, 0)
	for offset := 0; offset < 0x4000; offset++ {
		if uint16(offset)&mask == address {
			offsets = append(offsets, offset)
		}
	}
	return offsets
}

// Text displays the three register values using the grouping in the BIOS's
// initial port table. It deliberately does not generate a BIOS call sequence.
func (r Record) Text() string {
	return fmt.Sprintf("HL = %s\nDE = %s\nB  = %s", dcp.Binary14(r.Address), dcp.Binary14(r.Mask), dcp.Binary8(r.Code))
}

// Values formats copyable register values, with a blank line between records.
func Values(records []Record) string {
	parts := make([]string, len(records))
	for i, record := range records {
		parts[i] = record.Text()
	}
	return strings.Join(parts, "\n\n")
}

// Table formats the same values as DW/DW/DB initialization table records.
func Table(records []Record) string {
	parts := make([]string, len(records))
	for i, record := range records {
		parts[i] = fmt.Sprintf("DW %s\nDW %s\nDB %s", dcp.Binary14(record.Address), dcp.Binary14(record.Mask), dcp.Binary8(record.Code))
	}
	return strings.Join(parts, "\n\n")
}
