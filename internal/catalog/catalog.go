// Package catalog supplies optional display names for internal port codes.
// Names describe the bundled reference text; they do not validate a loaded map.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
)

// ports.json contains active, one-byte EQU definitions from
// Example/внутренние_порты.txt. Local assembly names are qualified by their
// enclosing label. Comments, including uncertainty in the source, are retained.
// Commented-out definitions and multi-byte loader constants are not entries.
//
//go:embed ports.json
var reference []byte

type entry struct {
	Code        byte   `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Line        int    `json:"line"`
}

var (
	byCode [256]entry
	codes  []byte
)

func init() {
	var data struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(reference, &data); err != nil {
		panic(fmt.Sprintf("catalog: invalid embedded reference: %v", err))
	}
	for _, item := range data.Entries {
		if item.Code == 0 || item.Name == "" || byCode[item.Code].Name != "" {
			panic(fmt.Sprintf("catalog: invalid or repeated entry %02X", item.Code))
		}
		byCode[item.Code] = item
		codes = append(codes, item.Code)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
}

// Name returns the symbol from the bundled reference, or an empty string if
// the code has no named definition. Code zero is handled by the UI as unmapped.
func Name(code byte) string { return byCode[code].Name }

// Description returns the inline source comment, or an empty string when the
// source has no comment or no named definition for this code.
func Description(code byte) string { return byCode[code].Description }

// Codes returns a sorted copy of the named codes. All 256 byte values remain
// valid document data regardless of whether they are present in this list.
func Codes() []byte { return append([]byte(nil), codes...) }
