package ui

import (
	"strings"

	"godcp/internal/dcp"
)

type searchField int

const (
	searchAll searchField = iota
	searchAddress
	searchOffset
	searchCode
	searchName
)

type filterState struct {
	cnf, direction, dos, pn5 int
	query                    string
	field                    searchField
	onlyAssigned             bool
}

func (f filterState) matches(offset int, value byte, name string) bool {
	port, c := dcp.Decode(offset)
	if f.cnf > 0 && c.CNF != f.cnf-1 {
		return false
	}
	if f.direction > 0 && bit(c.Read) != f.direction-1 {
		return false
	}
	if f.dos > 0 && bit(c.DOSOff) != f.dos-1 {
		return false
	}
	if f.pn5 > 0 && bit(c.PN5) != f.pn5-1 {
		return false
	}
	if f.onlyAssigned && value == 0 {
		return false
	}
	query := strings.TrimSpace(f.query)
	if query == "" {
		return true
	}
	if f.field == searchName {
		return strings.Contains(strings.ToLower(name), strings.ToLower(query))
	}
	bits := 16
	if f.field == searchOffset {
		bits = 14
	} else if f.field == searchCode {
		bits = 8
	}
	number, err := dcp.ParseNumber(query, bits)
	switch f.field {
	case searchAddress:
		return err == nil && dcp.Canonical(number) == port
	case searchOffset:
		return err == nil && int(number) == offset
	case searchCode:
		return err == nil && number == uint16(value)
	case searchAll:
		// Do not search formatted numeric columns as text: "0056" must not
		// match arbitrary digits in another number. An explicit field also
		// separates a file offset from an address with the same numeric value.
		return (err == nil && (dcp.Canonical(number) == port || int(number) == offset || number == uint16(value))) ||
			strings.Contains(strings.ToLower(name), strings.ToLower(query))
	default:
		return false
	}
}
