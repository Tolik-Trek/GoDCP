package ui

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestSearchFieldsMatchValuesRatherThanNumericSubstrings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		field  searchField
		query  string
		offset int
		value  byte
		label  string
		want   bool
	}{
		{"all offset", searchAll, "0056", 0x56, 0xC3, "ALL_MODE", true},
		{"all alias", searchAll, "204E", 0x56, 0xC3, "ALL_MODE", true},
		{"all code", searchAll, "C3", 0x56, 0xC3, "ALL_MODE", true},
		{"all name", searchAll, " mode ", 0x56, 0xC3, "ALL_MODE", true},
		{"all hex looking name", searchAll, "AC", 0x56, 0xC3, "SERIAL_AC_CTRL", true},
		{"all unrelated offset substring", searchAll, "C3", 0x0C34, 0x18, "Unknown", false},
		{"all partial address", searchAll, "204", 0x56, 0xC3, "ALL_MODE", false},
		{"all whitespace", searchAll, " \t ", 0x56, 0xC3, "ALL_MODE", true},
		{"offset exact", searchOffset, "0056", 0x56, 0xC3, "ALL_MODE", true},
		{"offset no code match", searchOffset, "C3", 0x56, 0xC3, "ALL_MODE", false},
		{"offset no address match", searchOffset, "204E", 0x56, 0xC3, "ALL_MODE", false},
		{"offset no substring", searchOffset, "56", 0x156, 0xC3, "ALL_MODE", false},
		{"offset maximum", searchOffset, "3FFF", 0x3FFF, 0, "", true},
		{"offset overflow", searchOffset, "4000", 0, 0, "4000", false},
		{"address canonical", searchAddress, "2046", 0x56, 0xC3, "ALL_MODE", true},
		{"address alias", searchAddress, "204E", 0x56, 0xC3, "ALL_MODE", true},
		{"address no offset match", searchAddress, "0056", 0x56, 0xC3, "ALL_MODE", false},
		{"address no code match", searchAddress, "C3", 0x56, 0xC3, "ALL_MODE", false},
		{"address maximum", searchAddress, "FFFF", 0x1FF, 0, "", true},
		{"address overflow", searchAddress, "10000", 0, 0, "10000", false},
		{"code exact", searchCode, "C3", 0x56, 0xC3, "ALL_MODE", true},
		{"code no offset match", searchCode, "0056", 0x56, 0xC3, "ALL_MODE", false},
		{"code no name match", searchCode, "MODE", 0x56, 0xC3, "ALL_MODE", false},
		{"code maximum", searchCode, "FF", 0, 0xFF, "", true},
		{"code overflow", searchCode, "100", 0x100, 0, "100", false},
		{"name case and whitespace", searchName, " mode ", 0x56, 0xC3, "ALL_MODE", true},
		{"name hex looking", searchName, "ac", 0x56, 0xC3, "SERIAL_AC_CTRL", true},
		{"name no code match", searchName, "C3", 0x56, 0xC3, "ALL_MODE", false},
		{"name unicode case", searchName, "НЕИЗВЕСТ", 0, 0x18, "Неизвестный", true},
		{"invalid numeric address", searchAddress, "not a port", 0x56, 0xC3, "not a port", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := filterState{field: tc.field, query: tc.query}
			if got := f.matches(tc.offset, tc.value, tc.label); got != tc.want {
				t.Fatalf("field %d query %q: got %v, want %v", tc.field, tc.query, got, tc.want)
			}
		})
	}
}

func TestSearchScopesRemainCombinedWithContextAndAssignmentFilters(t *testing.T) {
	f := filterState{field: searchAddress, query: "204E", cnf: 2, direction: 2, dos: 2, pn5: 2, onlyAssigned: true}
	for context := 0; context < 32; context++ {
		offset := 0x56 + context*0x200
		want := offset == 0x1E56 // CNF1 / IN / DOS OFF / PN5=1.
		if got := f.matches(offset, 0xC3, "ALL_MODE"); got != want {
			t.Errorf("offset %04X: match=%v, want %v", offset, got, want)
		}
	}
	if f.matches(0x1E56, 0, "Unassigned") {
		t.Fatal("a numeric match bypassed Assigned only")
	}
	for _, field := range []searchField{searchAll, searchAddress, searchOffset, searchCode, searchName} {
		f.field, f.query = field, " \t "
		if !f.matches(0x1E56, 0xC3, "ALL_MODE") || f.matches(0x0056, 0xC3, "ALL_MODE") {
			t.Fatalf("empty query in field %d did not preserve context filters", field)
		}
	}
}

func searchScope(t *testing.T, w *Workbench, tab int) *widget.Select {
	t.Helper()
	return findUI(t, w.tabs.Items[tab].Content, func(s *widget.Select) bool {
		return len(s.Options) == 5 && s.Options[0] == w.t("Все поля", "All fields")
	})
}

func assertSearchOffsets(t *testing.T, table *widget.Table, want []int) {
	t.Helper()
	rows, _ := table.Length()
	if rows != len(want)+1 {
		t.Fatalf("table contains %d results, want %d", rows-1, len(want))
	}
	for i, offset := range want {
		if got := uiCell(table, i+1, 0); got != fmt.Sprintf("%04X", offset) {
			t.Fatalf("result %d has offset %s, want %04X", i, got, offset)
		}
	}
}

func TestScopedSearchThroughControlsOnEveryFilterableTab(t *testing.T) {
	w := testWorkbench(t)
	data := exampleMap(t, w)
	for _, tab := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("tab%d", tab), func(t *testing.T) {
			w.tabs.SelectIndex(tab)
			content := w.tabs.Items[tab].Content
			query := uiEntry(t, content, "Code, name, address or offset…")
			scope := searchScope(t, w, tab)
			table := uiTable(t, content)
			if tab == 2 {
				uiEntry(t, content, "HEX code or exact name").SetText("C3")
			}
			scope.SetSelected("Offset")
			for _, text := range []string{"0056", "56", "#0056", "0x0056", "0056h", "%00'000'0'0101'0110", " 0056 "} {
				query.SetText(text)
				assertSearchOffsets(t, table, []int{0x56})
				if err := query.Validate(); err != nil {
					t.Fatalf("valid offset %q rejected: %v", text, err)
				}
			}
			for _, text := range []string{"4000", "10000", "invalid", "-1"} {
				query.SetText(text)
				assertSearchOffsets(t, table, nil)
				if query.Validate() == nil {
					t.Fatalf("invalid offset %q has no validation error", text)
				}
			}
			var unfiltered []int
			for offset, value := range data {
				if (tab != 1 || value != 0) && (tab != 2 || value == 0xC3) {
					unfiltered = append(unfiltered, offset)
				}
			}
			test.Tap(uiButton(t, content, "Clear"))
			assertSearchOffsets(t, table, unfiltered)
			if query.Text != "" || query.Validate() != nil || scope.Selected != "Offset" {
				t.Fatal("Clear failed to reset invalid input while retaining the selected field")
			}

			var aliases []int
			for context := 0; context < 32; context++ {
				offset := 0x56 + context*0x200
				if (tab != 1 || data[offset] != 0) && (tab != 2 || data[offset] == 0xC3) {
					aliases = append(aliases, offset)
				}
			}
			scope.SetSelected("Address")
			for _, text := range []string{"2046", "204E", "#204E", "0x204E", "204eh", "%0010'0000'0100'1110", " 204e "} {
				query.SetText(text)
				assertSearchOffsets(t, table, aliases)
				for row := 1; row <= len(aliases); row++ {
					if got := uiCell(table, row, 1); got != "2046" {
						t.Fatalf("alias search included unrelated address %s", got)
					}
				}
			}
			query.SetText("10000")
			assertSearchOffsets(t, table, nil)

			var codeOffsets []int
			for offset, value := range data {
				if value == 0xC3 {
					codeOffsets = append(codeOffsets, offset)
				}
			}
			scope.SetSelected("Code")
			for _, text := range []string{"C3", "#C3", "0xc3", "C3h", "%1100'0011"} {
				query.SetText(text)
				assertSearchOffsets(t, table, codeOffsets)
				for row := 1; row <= len(codeOffsets); row++ {
					if got := uiCell(table, row, 6); got != "C3" {
						t.Fatalf("code search included code %s", got)
					}
				}
			}
			query.SetText("100")
			assertSearchOffsets(t, table, nil)
			scope.SetSelected("Name")
			query.SetText(" all_mode ")
			assertSearchOffsets(t, table, codeOffsets)
			for row := 1; row <= len(codeOffsets); row++ {
				if !strings.Contains(strings.ToLower(uiCell(table, row, 7)), "all_mode") {
					t.Fatal("name search included an unrelated name")
				}
			}
			query.SetText("C3")
			var namesContainingC3 []int
			for offset, value := range data {
				if (tab != 1 || value != 0) && (tab != 2 || value == 0xC3) && strings.Contains(strings.ToUpper(w.codeName(value)), "C3") {
					namesContainingC3 = append(namesContainingC3, offset)
				}
			}
			assertSearchOffsets(t, table, namesContainingC3)
			query.SetText("#C3")
			assertSearchOffsets(t, table, nil)

			// The same text must be re-evaluated when the selected field changes.
			query.SetText("0056")
			scope.SetSelected("Offset")
			assertSearchOffsets(t, table, []int{0x56})
			scope.SetSelected("Address")
			rows, _ := table.Length()
			for row := 1; row < rows; row++ {
				if got := uiCell(table, row, 1); got != "0046" {
					t.Fatalf("changing scope kept stale address results: %s", got)
				}
			}
			// Leave different state in every tab for the language rebuild below.
			scope.SetSelectedIndex(tab + 1)
		})
	}
	if w.doc.Dirty() {
		t.Fatal("searching changed the document")
	}

	w.language = "ru"
	w.render()
	for tab := 0; tab < 3; tab++ {
		query := uiEntry(t, w.tabs.Items[tab].Content, "Код, имя, адрес или смещение…")
		if query.Text != "0056" || searchScope(t, w, tab).SelectedIndex() != tab+1 {
			t.Fatalf("language rebuild lost query or scope on tab %d", tab)
		}
	}
	assertSearchOffsets(t, uiTable(t, w.tabs.Items[1].Content), []int{0x56})
}
