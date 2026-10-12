package ui

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func testWorkbench(t *testing.T) *Workbench {
	t.Helper()
	a := test.NewApp()
	a.Preferences().SetString("language", "en")
	a.Preferences().SetString("theme", "light")
	w := New(a)
	w.Window.Show()
	t.Cleanup(func() {
		w.Window.SetCloseIntercept(nil)
		w.Window.Close()
		a.Quit()
	})
	return w
}

func walkUI(object fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	if object == nil {
		return
	}
	visit(object)
	switch object := object.(type) {
	case *fyne.Container:
		for _, child := range object.Objects {
			walkUI(child, visit)
		}
	case *container.Scroll:
		walkUI(object.Content, visit)
	case *container.ThemeOverride:
		walkUI(object.Content, visit)
	case *container.Split:
		walkUI(object.Leading, visit)
		walkUI(object.Trailing, visit)
	case *widget.Form:
		for _, item := range object.Items {
			walkUI(item.Widget, visit)
		}
	}
}

func findUI[T fyne.CanvasObject](t *testing.T, root fyne.CanvasObject, match func(T) bool) T {
	t.Helper()
	var found T
	foundOne := false
	walkUI(root, func(object fyne.CanvasObject) {
		if value, ok := object.(T); ok && !foundOne && match(value) {
			found, foundOne = value, true
		}
	})
	if !foundOne {
		t.Fatalf("requested UI object %T not found", found)
	}
	return found
}

func uiTable(t *testing.T, root fyne.CanvasObject) *widget.Table {
	t.Helper()
	return findUI(t, root, func(*widget.Table) bool { return true })
}

func uiButton(t *testing.T, root fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	return findUI(t, root, func(b *widget.Button) bool { return b.Text == text })
}

func uiEntry(t *testing.T, root fyne.CanvasObject, placeholder string) *widget.Entry {
	t.Helper()
	return findUI(t, root, func(e *widget.Entry) bool { return e.PlaceHolder == placeholder })
}

func uiLabelContains(root fyne.CanvasObject, substring string) bool {
	found := false
	walkUI(root, func(object fyne.CanvasObject) {
		if label, ok := object.(*widget.Label); ok && strings.Contains(label.Text, substring) {
			found = true
		}
	})
	return found
}

// Use the table's public cell-rendering callback to inspect the same strings
// that Fyne renders, including rows outside the visible/virtualized viewport.
func uiCell(table *widget.Table, row, column int) string {
	label := widget.NewLabel("")
	table.UpdateCell(widget.TableCellID{Row: row, Col: column}, label)
	return label.Text
}

func exampleMap(t *testing.T, w *Workbench) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "Example", "DCP_PAGE.bin")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.OpenPath(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		current, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(data, current) {
			t.Error("the reference BIN was modified during the UI test")
		}
	})
	return data
}

func TestWorkbenchLoadedBytesEditUndoRedoAndLanguage(t *testing.T) {
	w := testWorkbench(t)
	if w.doc != nil || len(w.tabs.Items) != 5 || !w.save.Disabled() || !w.undo.Disabled() || !w.redo.Disabled() {
		t.Fatal("initial workbench must expose five tabs with no open document or available edits")
	}
	data := exampleMap(t, w)
	if w.doc.Dirty() || !bytes.Equal(w.doc.Bytes(), data) {
		t.Fatal("opening the example changed its data or marked it dirty")
	}
	assigned := uiTable(t, w.tabs.Items[1].Content)
	rows, _ := assigned.Length()
	nonzero := 0
	for _, value := range data {
		if value != 0 {
			nonzero++
		}
	}
	if rows != nonzero+1 {
		t.Fatalf("assigned table has %d rows; want %d assignments and a header", rows, nonzero)
	}

	w.tabs.SelectIndex(3)
	external := uiTable(t, w.tabs.Items[3].Content)
	rows, _ = external.Length()
	if rows != 33 {
		t.Fatalf("external lookup has %d rows; want 32 contexts and a header", rows)
	}
	// Independently known compact address for 204Eh is 0056h. Every context
	// occupies the next 0200h block. Validate values against the loaded file,
	// never against display names or assumed BIOS defaults.
	for row := 1; row <= 32; row++ {
		offset := 0x56 + (row-1)*0x200
		if got := uiCell(external, row, 0); got != fmt.Sprintf("%04X", offset) {
			t.Fatalf("external row %d offset = %s; want %04X", row, got, offset)
		}
		if got := uiCell(external, row, 6); got != fmt.Sprintf("%02X", data[offset]) {
			t.Fatalf("external row %d code = %s; raw BIN has %02X", row, got, data[offset])
		}
	}

	internalContent := w.tabs.Items[2].Content
	internalQuery := uiEntry(t, internalContent, "HEX code or exact name")
	internalQuery.SetText("18") // Reserved in the reference, present in this BIN.
	internal := uiTable(t, internalContent)
	wantCount := bytes.Count(data, []byte{0x18})
	rows, _ = internal.Length()
	if rows != wantCount+1 {
		t.Fatalf("internal lookup ignored raw 18h assignments: rows=%d want=%d", rows, wantCount+1)
	}

	// Exercise actual editor controls, then toolbar Undo/Redo. Searches must
	// reflect unsaved edits without reopening or saving the map.
	w.tabs.SelectIndex(0)
	editor := w.tabs.Items[0].Content
	uiEntry(t, editor, "0000–3FFF").SetText("0056")
	test.Tap(uiButton(t, editor, "Go"))
	uiEntry(t, editor, "00–FF or name").SetText("18")
	test.Tap(uiButton(t, editor, "Replace byte"))
	if w.doc.Value(0x56) != 0x18 || !w.doc.Dirty() || w.undo.Disabled() || w.save.Disabled() {
		t.Fatal("editor replacement did not produce an undoable unsaved change")
	}
	if got := uiCell(external, 1, 6); got != "18" {
		t.Fatalf("external lookup is stale after editing: %s", got)
	}
	rows, _ = internal.Length()
	if rows != wantCount+2 {
		t.Fatalf("internal lookup did not gain the edited assignment: rows=%d", rows)
	}
	test.Tap(w.undo)
	if w.doc.Value(0x56) != data[0x56] || w.doc.Dirty() || w.redo.Disabled() || uiCell(external, 1, 6) != "C3" {
		t.Fatal("Undo did not restore the loaded byte and live lookup")
	}
	test.Tap(w.redo)
	if w.doc.Value(0x56) != 0x18 || !w.doc.Dirty() || uiCell(external, 1, 6) != "18" {
		t.Fatal("Redo did not restore the edited byte and live lookup")
	}

	before := w.doc.Bytes()
	document := w.doc
	w.tabs.SelectIndex(3)
	w.language = "ru"
	w.render()
	if w.doc != document || !bytes.Equal(w.doc.Bytes(), before) || !w.doc.Dirty() {
		t.Fatal("changing language lost the document or its unsaved change")
	}
	if w.tabs.SelectedIndex() != 3 || w.tabs.Items[0].Text != "Редактор карты" || w.internalQuery != "18" {
		t.Fatal("changing language lost active tab/query or failed to localize tab titles")
	}
	if got := uiCell(uiTable(t, w.tabs.Items[3].Content), 1, 6); got != "18" {
		t.Fatalf("localized lookup no longer shows current data: %s", got)
	}
}

func TestGeneratorCopyInvalidInputAndDocumentIsolation(t *testing.T) {
	w := testWorkbench(t)
	w.tabs.SelectIndex(4)
	content := w.tabs.Items[4].Content
	output := findUI(t, content, func(e *widget.Entry) bool { return e.MultiLine })
	wantDefault := "HL = %00'000'0'0101'0110\nDE = %00'001'1'1111'1111\nB  = %1100'0011"
	if output.Text != wantDefault || w.doc != nil {
		t.Fatalf("generator should work without a file: output=%q", output.Text)
	}
	for _, tc := range []struct{ button, value string }{
		{"Address (HL)", "%00'000'0'0101'0110"},
		{"Mask (DE)", "%00'001'1'1111'1111"},
		{"Code (B)", "%1100'0011"},
		{"Copy all", wantDefault},
	} {
		test.Tap(uiButton(t, content, tc.button))
		if got := w.App.Clipboard().Content(); got != tc.value {
			t.Fatalf("%s copied %q; want %q", tc.button, got, tc.value)
		}
	}
	port := uiEntry(t, content, "0000 … FFFF")
	port.SetText("10000")
	if output.Text != "" {
		t.Fatal("invalid input left stale generator text available")
	}
	w.App.Clipboard().SetContent("unchanged clipboard")
	for _, caption := range []string{"Address (HL)", "Mask (DE)", "Code (B)", "Copy all"} {
		button := uiButton(t, content, caption)
		if !button.Disabled() {
			t.Fatalf("%s is still enabled for invalid input", caption)
		}
		test.Tap(button)
	}
	if got := w.App.Clipboard().Content(); got != "unchanged clipboard" {
		t.Fatalf("invalid input could still copy an old rule: %q", got)
	}
	port.SetText("204E")
	if output.Text != wantDefault || uiButton(t, content, "Copy all").Disabled() {
		t.Fatal("valid input did not restore calculated output and copying")
	}

	data := exampleMap(t, w)
	cnf := findUI(t, content, func(*widget.CheckGroup) bool { return true })
	cnf.SetSelected([]string{"0", "3"})
	wantSplit := "HL = %00'000'0'0101'0110\nDE = %11'001'1'1111'1111\nB  = %1100'0011\n\nHL = %11'000'0'0101'0110\nDE = %11'001'1'1111'1111\nB  = %1100'0011"
	if output.Text != wantSplit || !uiLabelContains(content, "Covers 8 map entries") {
		t.Fatalf("CNF0 + CNF3 did not produce two exact rules: %q", output.Text)
	}
	test.Tap(uiButton(t, content, "Address (HL)"))
	if got := w.App.Clipboard().Content(); got != "%00'000'0'0101'0110\n%11'000'0'0101'0110" {
		t.Fatalf("multiple addresses copied incorrectly: %q", got)
	}
	format := findUI(t, content, func(s *widget.Select) bool {
		return len(s.Options) == 2 && s.Options[0] == "HL / DE / B"
	})
	format.SetSelectedIndex(1)
	wantTable := "DW %00'000'0'0101'0110\nDW %11'001'1'1111'1111\nDB %1100'0011\n\nDW %11'000'0'0101'0110\nDW %11'001'1'1111'1111\nDB %1100'0011"
	if output.Text != wantTable {
		t.Fatalf("initialization-table format = %q", output.Text)
	}
	cnf.SetSelected(nil)
	if output.Text != "" || !uiButton(t, content, "Copy all").Disabled() {
		t.Fatal("empty CNF selection did not clear and disable the output")
	}
	cnf.SetSelected([]string{"0", "3"})
	if !bytes.Equal(w.doc.Bytes(), data) || w.doc.Dirty() {
		t.Fatal("generator controls changed the loaded BIN")
	}

	// An editor change refreshes only the comparison, preserving this rule.
	matched := 0
	for _, base := range []int{0, 0x3000} {
		for _, context := range []int{0, 0x400, 0x800, 0xC00} {
			if data[base+context+0x56] == 0xC3 {
				matched++
			}
		}
	}
	if !uiLabelContains(content, fmt.Sprintf("Current BIN: %d entries match, %d differ.", matched, 8-matched)) {
		t.Fatal("generator preview does not reflect the loaded BIN")
	}
	w.setByte(0x56, 0x18)
	if output.Text != wantTable || !uiLabelContains(content, fmt.Sprintf("Current BIN: %d entries match, %d differ.", matched-1, 9-matched)) {
		t.Fatal("live document edit reset the rule or failed to update its comparison")
	}

	before := w.doc.Bytes()
	w.language = "ru"
	w.render()
	localized := w.tabs.Items[4].Content
	localizedOutput := findUI(t, localized, func(e *widget.Entry) bool { return e.MultiLine })
	if localizedOutput.Text != wantTable || !bytes.Equal(w.doc.Bytes(), before) {
		t.Fatal("changing language reset generator selections or changed document data")
	}
	localizedCNF := findUI(t, localized, func(*widget.CheckGroup) bool { return true })
	if strings.Join(localizedCNF.Selected, ",") != "0,3" {
		t.Fatalf("localized CNF selection = %v", localizedCNF.Selected)
	}
}

func TestEditorSelectionAcrossHEXDraftAndAliasFilter(t *testing.T) {
	w := testWorkbench(t)
	data := exampleMap(t, w)
	editor := w.tabs.Items[0].Content
	table := findUI(t, editor, func(table *widget.Table) bool {
		_, columns := table.Length()
		return columns == 8
	})
	hex := findUI(t, editor, func(table *widget.Table) bool {
		_, columns := table.Length()
		return columns == 17
	})
	mode := findUI(t, editor, func(*widget.RadioGroup) bool { return true })
	const a, b = 0x56, 0x57
	table.Select(widget.TableCellID{Row: b + 1, Col: 0})
	mode.SetSelected("HEX")
	hex.Select(widget.TableCellID{Row: a/16 + 1, Col: a%16 + 1})
	mode.SetSelected("Table")
	// B used to be the table's selected row before the HEX navigation to A.
	// Selecting B again must update the editor target, even though the table
	// could otherwise still believe that B was already selected.
	table.Select(widget.TableCellID{Row: b + 1, Col: 0})
	uiEntry(t, editor, "00–FF or name").SetText("18")
	test.Tap(uiButton(t, editor, "Replace byte"))
	if w.selected != b || w.doc.Value(b) != 0x18 || w.doc.Value(a) != data[a] {
		t.Fatalf("Table → HEX → Table selection edited wrong offset: selected=%04X A=%02X B=%02X", w.selected, w.doc.Value(a), w.doc.Value(b))
	}

	query := uiEntry(t, editor, "Code, name, address or offset…")
	for _, address := range []string{"0x204E", "#204E", "204Eh"} {
		query.SetText(address)
		rows, _ := table.Length()
		found := make(map[string]bool)
		for row := 1; row < rows; row++ {
			found[uiCell(table, row, 0)] = true
		}
		for context := 0; context < 32; context++ {
			offset := fmt.Sprintf("%04X", 0x56+context*0x200)
			if !found[offset] {
				t.Fatalf("alias filter %q omitted address-group offset %s", address, offset)
			}
		}
	}
	query.SetText("")
	w.focusEditor(b)
	uiEntry(t, editor, "00–FF or name").SetText("FE")
	before := w.doc.Bytes()
	w.language = "ru"
	w.render()
	localized := w.tabs.Items[0].Content
	if got := uiEntry(t, localized, "00–FF или имя").Text; got != "FE" {
		t.Fatalf("language switch lost an unapplied editor draft: %q", got)
	}
	if w.selected != b || !bytes.Equal(w.doc.Bytes(), before) {
		t.Fatal("language switch applied a draft or changed the selected offset")
	}
}

func TestEditorModeSwitchReflowsFilter(t *testing.T) {
	w := testWorkbench(t)
	exampleMap(t, w)
	mode := findUI(t, w.tabs.Items[0].Content, func(*widget.RadioGroup) bool { return true })
	mode.SetSelected("HEX")
	w.language = "ru"
	w.render()
	editor := w.tabs.Items[0].Content
	mode = findUI(t, editor, func(*widget.RadioGroup) bool { return true })
	mode.SetSelected("Таблица")
	query := uiEntry(t, editor, "Код, имя, адрес или смещение…")
	if size := query.Size(); size.Width <= 0 || size.Height <= 0 {
		t.Fatalf("filter geometry after HEX/language/Table switch is invalid: %v", size)
	}
}

func TestWorkbenchRenderPreview(t *testing.T) {
	if os.Getenv("GODCP_SCREENSHOT") == "" {
		t.Skip("set GODCP_SCREENSHOT=1 to capture headless previews")
	}
	w := testWorkbench(t)
	exampleMap(t, w)
	outputDir := filepath.Join("..", "..", "Build")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"ru", "en"} {
		w.language = language
		w.render()
		w.focusEditor(0x56)
		for _, preview := range []struct {
			tab  int
			name string
			hex  bool
		}{
			{0, "GoDCP-preview", false}, {1, "GoDCP-assigned", false},
			{2, "GoDCP-internal", false}, {3, "GoDCP-external", false},
			{4, "GoDCP-generator", false}, {0, "GoDCP-hex", true},
		} {
			w.tabs.SelectIndex(preview.tab)
			if preview.tab == 0 {
				mode := findUI(t, w.tabs.Items[0].Content, func(*widget.RadioGroup) bool { return true })
				if preview.hex {
					mode.SetSelected("HEX")
				} else {
					mode.SetSelected(w.t("Таблица", "Table"))
				}
				w.focusEditor(0x56)
			}
			image := w.Window.Canvas().Capture()
			if image == nil || image.Bounds().Empty() {
				t.Fatal("headless canvas capture returned an empty image")
			}
			name := preview.name
			if language == "en" {
				name += "-en"
			}
			path := filepath.Join(outputDir, name+".png")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			encodeErr := png.Encode(file, image)
			closeErr := file.Close()
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			t.Logf("saved CPU-rendered preview: %s", path)
		}
	}
}
