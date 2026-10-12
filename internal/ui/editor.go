package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"godcp/internal/catalog"
	"godcp/internal/dcp"
)

func parseCode(text string) (byte, error) {
	if n, err := dcp.ParseNumber(text, 8); err == nil {
		return byte(n), nil
	}
	for _, code := range catalog.Codes() {
		if strings.EqualFold(strings.TrimSpace(text), catalog.Name(code)) {
			return code, nil
		}
	}
	return 0, fmt.Errorf("invalid internal code")
}

func (w *Workbench) editor() fyne.CanvasObject {
	var offsets []int
	var table *grid
	var hex *widget.Table
	var editorContent *fyne.Container
	offsetInput := widget.NewEntry()
	offsetInput.SetPlaceHolder("0000–3FFF")
	valueInput := widget.NewEntry()
	valueInput.SetPlaceHolder(w.t("00–FF или имя", "00–FF or name"))
	details := widget.NewMultiLineEntry()
	details.TextStyle.Monospace = true
	details.SetMinRowsVisible(8)
	details.Disable()
	details.Wrapping = fyne.TextWrapWord
	help := widget.NewLabel(w.t("Замена одного байта меняет назначение для 128 алиасов в выбранном режиме.", "Replacing one byte changes the assignment for 128 aliases in this context."))
	help.Wrapping = fyne.TextWrapWord
	apply := widget.NewButton(w.t("Заменить байт", "Replace byte"), nil)
	apply.Importance = widget.HighImportance
	message := widget.NewLabel("")
	message.Wrapping = fyne.TextWrapWord
	syncing := false
	selectOffset := func(offset int) {
		if syncing {
			return
		}
		syncing = true
		defer func() { syncing = false }()
		if w.selected != offset {
			w.editorDraftSet = false
		}
		w.selected = offset
		offsetInput.SetText(fmt.Sprintf("%04X", offset))
		if w.doc == nil {
			valueInput.SetText("")
			details.SetText(w.t("Карта не открыта", "No map loaded"))
			apply.Disable()
			return
		}
		v := w.doc.Value(offset)
		if w.editorDraftSet && w.editorDraftOffset == offset {
			valueInput.SetText(w.editorDraft)
		} else {
			valueInput.SetText(fmt.Sprintf("%02X", v))
		}
		p, c := dcp.Decode(offset)
		details.SetText(fmt.Sprintf("%s: %04X\nCNF %d · %s · DOS %s · PN5 %d\n%s: %04X / E0E7\n%s: %02X · %s\n%s", w.t("Смещение", "Offset"), offset, c.CNF, w.direction(c.Read), w.dos(c.DOSOff), bit(c.PN5), w.t("Группа адресов", "Address group"), p, w.t("Внутренний код", "Internal code"), v, w.codeName(v), catalog.Description(v)))
		apply.Enable()
		message.SetText("")
		if hex != nil {
			hex.Select(widget.TableCellID{Row: offset/16 + 1, Col: offset%16 + 1})
		}
		if table != nil {
			table.table.UnselectAll()
			for i, o := range offsets {
				if o == offset {
					table.table.Select(widget.TableCellID{Row: i + 1, Col: 0})
					break
				}
			}
		}
	}
	valueInput.OnChanged = func(s string) {
		if !syncing {
			w.editorDraft = s
			w.editorDraftOffset = w.selected
			w.editorDraftSet = true
		}
	}
	table = newGrid(w.headers(), rowWidths, func(i int) {
		if i < len(offsets) {
			selectOffset(offsets[i])
		}
	})
	hex = widget.NewTable(func() (int, int) {
		if w.doc == nil {
			return 1, 17
		}
		return 1025, 17
	}, func() fyne.CanvasObject { return mono("") }, func(id widget.TableCellID, obj fyne.CanvasObject) {
		l := obj.(*widget.Label)
		l.TextStyle = fyne.TextStyle{Monospace: true, Bold: id.Row == 0 || id.Col == 0}
		if id.Row == 0 {
			if id.Col == 0 {
				l.SetText(w.t("Адрес", "Offset"))
			} else {
				l.SetText(fmt.Sprintf("%X", id.Col-1))
			}
			return
		}
		if id.Col == 0 {
			l.SetText(fmt.Sprintf("%04X", (id.Row-1)*16))
			return
		}
		if w.doc != nil {
			l.SetText(fmt.Sprintf("%02X", w.doc.Value((id.Row-1)*16+id.Col-1)))
		} else {
			l.SetText("")
		}
	})
	hex.SetColumnWidth(0, 68)
	hex.StickyRowCount = 1
	hex.StickyColumnCount = 1
	for i := 1; i < 17; i++ {
		hex.SetColumnWidth(i, 30)
	}
	hex.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Col > 0 && w.doc != nil {
			selectOffset((id.Row-1)*16 + id.Col - 1)
		}
	}
	only := widget.NewCheck(w.t("Только назначенные", "Assigned only"), nil)
	only.SetChecked(w.editorFilter.onlyAssigned)
	filterContainer := container.NewVBox()
	stack := container.NewStack(table.table, hex)
	mode := widget.NewRadioGroup([]string{w.t("Таблица", "Table"), "HEX"}, nil)
	mode.Horizontal = true
	if w.hexMode {
		mode.SetSelected("HEX")
	} else {
		mode.SetSelected(w.t("Таблица", "Table"))
	}
	setMode := func() {
		w.hexMode = mode.Selected == "HEX"
		if w.hexMode {
			table.table.Hide()
			hex.Show()
			filterContainer.Hide()
			only.Hide()
		} else {
			hex.Hide()
			table.table.Show()
			filterContainer.Show()
			only.Show()
		}
		selectOffset(w.selected)
		if editorContent != nil {
			editorContent.Refresh()
		}
	}
	mode.OnChanged = func(string) { setMode() }
	setMode()
	refresh := func() {
		offsets = nil
		var rows [][]string
		if w.doc != nil {
			for i := 0; i < dcp.Size; i++ {
				v := w.doc.Value(i)
				if !w.editorFilter.matches(i, v, w.codeName(v)) {
					continue
				}
				offsets = append(offsets, i)
				rows = append(rows, w.row(i))
			}
		}
		table.setRows(rows)
		hex.Refresh()
		selectOffset(w.selected)
	}
	filterContainer.Add(w.filters(&w.editorFilter, refresh))
	only.OnChanged = func(v bool) { w.editorFilter.onlyAssigned = v; refresh() }
	goTo := func() {
		o, err := dcp.ParseNumber(offsetInput.Text, 14)
		if err != nil {
			message.SetText(w.t("Смещение должно быть 0000–3FFF", "Offset must be 0000–3FFF"))
			return
		}
		selectOffset(int(o))
		if w.hexMode {
			hex.Select(widget.TableCellID{Row: int(o)/16 + 1, Col: int(o)%16 + 1})
			hex.ScrollTo(widget.TableCellID{Row: int(o)/16 + 1, Col: int(o)%16 + 1})
		} else {
			found := false
			for i, offset := range offsets {
				if offset == int(o) {
					table.table.Select(widget.TableCellID{Row: i + 1, Col: 0})
					table.table.ScrollTo(widget.TableCellID{Row: i + 1, Col: 0})
					found = true
					break
				}
			}
			if !found {
				message.SetText(w.t("Запись скрыта фильтрами; её значение доступно справа.", "Record is hidden by filters; its value is available in this panel."))
			}
		}
	}
	offsetInput.OnSubmitted = func(string) { goTo() }
	apply.OnTapped = func() {
		v, err := parseCode(valueInput.Text)
		if err != nil {
			message.SetText(w.t("Введите код 00–FF или имя из справочника", "Enter a code 00–FF or a catalog name"))
			return
		}
		w.setByte(w.selected, v)
	}
	valueInput.OnSubmitted = func(string) { apply.OnTapped() }
	aliasButton := widget.NewButton(w.t("128 алиасов", "128 aliases"), func() { p, _ := dcp.Decode(w.selected); w.aliasesDialog(p) })
	panel := container.NewVBox(widget.NewLabelWithStyle(w.t("Выбранная запись", "Selected record"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, widget.NewButton(w.t("Перейти", "Go"), goTo), offsetInput), w.readOnly(details), aliasButton, widget.NewSeparator(), widget.NewLabel(w.t("Новое значение HEX / имя", "New value HEX / name")), valueInput, apply, message, help)
	split := container.NewHSplit(stack, container.NewVScroll(panel))
	split.Offset = .73
	w.refreshers = append(w.refreshers, refresh)
	w.focusEditor = func(offset int) { selectOffset(offset); offsetInput.SetText(fmt.Sprintf("%04X", offset)); goTo() }
	editorContent = container.NewBorder(container.NewVBox(container.NewHBox(mode, only), filterContainer), nil, nil, nil, split)
	return editorContent
}
