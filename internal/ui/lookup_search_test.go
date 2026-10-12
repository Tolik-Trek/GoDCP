package ui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func assertLookupOffsets(t *testing.T, table *widget.Table, want []int) {
	t.Helper()
	rows, _ := table.Length()
	if rows != len(want)+1 {
		t.Fatalf("table has %d data rows; want %d", rows-1, len(want))
	}
	for row, offset := range want {
		if got := uiCell(table, row+1, 0); got != fmt.Sprintf("%04X", offset) {
			t.Fatalf("row %d has offset %s; want %04X", row+1, got, offset)
		}
	}
}

func TestInternalLookupInputFormatsAndRecovery(t *testing.T) {
	w := testWorkbench(t)
	data := exampleMap(t, w)
	w.tabs.SelectIndex(2)
	content := w.tabs.Items[2].Content
	query := uiEntry(t, content, "HEX code or exact name")
	table := uiTable(t, content)
	for _, tc := range []struct {
		input string
		code  byte
	}{
		{"C3", 0xC3}, {"#c3", 0xC3}, {"0XC3", 0xC3}, {"c3H", 0xC3},
		{"%1100'0011", 0xC3}, {"  all_mode  ", 0xC3},
		{"18", 0x18}, // A valid code absent from the name catalog.
		{"00", 0},    // Unassigned records remain searchable in this tab.
		{"FF", 0xFF},
	} {
		t.Run(tc.input, func(t *testing.T) {
			query.SetText(tc.input)
			var want []int
			for offset, value := range data {
				if value == tc.code {
					want = append(want, offset)
				}
			}
			assertLookupOffsets(t, table, want)
		})
	}
	for _, invalid := range []string{"", " ", "100", "-1", "GG", "%100000000", "ALL_MOD"} {
		t.Run("invalid_"+invalid, func(t *testing.T) {
			query.SetText("C3")
			table.Select(widget.TableCellID{Row: 1, Col: 0})
			if uiButton(t, content, "Edit selected").Disabled() {
				t.Fatal("valid selected lookup row cannot be opened")
			}
			query.SetText(invalid)
			assertLookupOffsets(t, table, nil)
			if !uiButton(t, content, "Edit selected").Disabled() || !uiButton(t, content, "Selected port aliases").Disabled() {
				t.Fatal("invalid input retained actions for a stale selection")
			}
			query.SetText("ALL_MODE")
			rows, _ := table.Length()
			if rows <= 1 {
				t.Fatal("valid catalog name did not recover lookup after invalid input")
			}
		})
	}
}

func TestExternalLookupInputFormatsAndRecovery(t *testing.T) {
	w := testWorkbench(t)
	data := exampleMap(t, w)
	w.tabs.SelectIndex(3)
	content := w.tabs.Items[3].Content
	query := uiEntry(t, content, "204E")
	table := uiTable(t, content)
	for _, tc := range []struct {
		input string
		port  uint16
		base  int
	}{
		{"204E", 0x204E, 0x56}, {"  #204e  ", 0x204E, 0x56},
		{"0X204E", 0x204E, 0x56}, {"204eh", 0x204E, 0x56},
		{"%0010'0000'0100'1110", 0x204E, 0x56},
		{"2046", 0x2046, 0x56}, {"3F5E", 0x3F5E, 0x56},
		{"0000", 0, 0}, {"FFFF", 0xFFFF, 0x1FF},
	} {
		t.Run(tc.input, func(t *testing.T) {
			query.SetText(tc.input)
			want := make([]int, 32)
			for context := range want {
				want[context] = tc.base + context*0x200
			}
			assertLookupOffsets(t, table, want)
			for row, offset := range want {
				if got := uiCell(table, row+1, 1); got != fmt.Sprintf("%04X", tc.port) {
					t.Fatalf("row %d displays address %s; want entered address %04X", row+1, got, tc.port)
				}
				if got := uiCell(table, row+1, 6); got != fmt.Sprintf("%02X", data[offset]) {
					t.Fatalf("row %d has value %s; raw BIN at %04X has %02X", row+1, got, offset, data[offset])
				}
			}
		})
	}
	for _, invalid := range []string{"", " ", "10000", "-1", "GG", "%10000000000000000"} {
		t.Run("invalid_"+invalid, func(t *testing.T) {
			query.SetText("204E")
			table.Select(widget.TableCellID{Row: 1, Col: 0})
			query.SetText(invalid)
			assertLookupOffsets(t, table, nil)
			if !uiButton(t, content, "Edit selected").Disabled() || !uiButton(t, content, "Address aliases").Disabled() {
				t.Fatal("invalid address retained actions for a stale selection")
			}
			query.SetText("204E")
			rows, _ := table.Length()
			if rows != 33 || uiButton(t, content, "Address aliases").Disabled() {
				t.Fatal("valid address did not recover all contexts and alias action")
			}
		})
	}
}

func modeSelect(t *testing.T, content fyne.CanvasObject, options string) *widget.Select {
	t.Helper()
	return findUI(t, content, func(s *widget.Select) bool { return strings.Join(s.Options, "|") == options })
}

func TestModeFiltersAcrossSearchTabs(t *testing.T) {
	w := testWorkbench(t)
	data := exampleMap(t, w)
	for _, tab := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("tab_%d", tab), func(t *testing.T) {
			w.tabs.SelectIndex(tab)
			content := w.tabs.Items[tab].Content
			table := uiTable(t, content)
			cnf := modeSelect(t, content, "All|0|1|2|3")
			direction := modeSelect(t, content, "All|OUT|IN")
			dos := modeSelect(t, content, "All|ON|OFF")
			pn5 := modeSelect(t, content, "All|0|1")
			for _, selections := range [][4]int{
				{0, 0, 0, 0}, {4, 0, 0, 0}, {0, 1, 0, 0}, {0, 2, 0, 0},
				{0, 0, 1, 0}, {0, 0, 2, 0}, {0, 0, 0, 1}, {0, 0, 0, 2},
				{4, 1, 2, 2}, {4, 2, 2, 2}, {0, 0, 0, 0},
			} {
				cnf.SetSelectedIndex(selections[0])
				direction.SetSelectedIndex(selections[1])
				dos.SetSelectedIndex(selections[2])
				pn5.SetSelectedIndex(selections[3])
				var want []int
				for offset, value := range data {
					if tab == 1 && value == 0 || tab == 2 && value != 0xC3 {
						continue
					}
					// Context selection uses the documented BIN address bits;
					// do not reuse the production filter or Decode implementation.
					if selections[0] > 0 && offset>>12 != selections[0]-1 ||
						selections[1] > 0 && offset>>9&1 != selections[1]-1 ||
						selections[2] > 0 && offset>>10&1 != selections[2]-1 ||
						selections[3] > 0 && offset>>11&1 != selections[3]-1 {
						continue
					}
					want = append(want, offset)
				}
				t.Run(fmt.Sprint(selections), func(t *testing.T) { assertLookupOffsets(t, table, want) })
			}
		})
	}
}
