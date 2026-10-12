package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"godcp/internal/bios"
	"godcp/internal/catalog"
	"godcp/internal/dcp"
)

type generatorState struct {
	port, code                  string
	cnf                         []string
	direction, dos, pn5, format int
}

func (w *Workbench) generator() fyne.CanvasObject {
	if w.generatorState == nil {
		w.generatorState = &generatorState{port: "204E", code: "C3", cnf: []string{"0", "1", "2", "3"}, dos: 2, pn5: 2}
	}
	state := w.generatorState
	port := widget.NewEntry()
	port.TextStyle.Monospace = true
	port.SetText(state.port)
	port.SetPlaceHolder("0000 … FFFF")
	code := widget.NewEntry()
	code.TextStyle.Monospace = true
	code.SetText(state.code)
	code.SetPlaceHolder("00 … FF")
	cnf := widget.NewCheckGroup([]string{"0", "1", "2", "3"}, nil)
	cnf.Horizontal = true
	cnf.SetSelected(append([]string(nil), state.cnf...))
	direction := widget.NewSelect([]string{"OUT", "IN", w.t("Оба", "Both")}, nil)
	direction.SetSelectedIndex(state.direction)
	dos := widget.NewSelect([]string{"ON (/DOS = 0)", "OFF (/DOS = 1)", w.t("Любое", "Any")}, nil)
	dos.SetSelectedIndex(state.dos)
	pn5 := widget.NewSelect([]string{"0", "1", w.t("Любое", "Any")}, nil)
	pn5.SetSelectedIndex(state.pn5)
	format := widget.NewSelect([]string{"HL / DE / B", "DW / DW / DB"}, nil)
	format.SetSelectedIndex(state.format)

	var names []string
	codesByName := make(map[string]byte)
	var namesByCode [256]string
	for _, value := range append([]byte{0}, catalog.Codes()...) {
		name := fmt.Sprintf("%02X — %s", value, w.codeName(value))
		names = append(names, name)
		codesByName[name] = value
		namesByCode[value] = name
	}
	namedCode := widget.NewSelect(names, nil)
	namedCode.PlaceHolder = w.t("Числовой код / справочник", "Numeric code / reference")
	if value, err := dcp.ParseNumber(state.code, 8); err == nil && namesByCode[value] != "" {
		namedCode.SetSelected(namesByCode[value])
	}

	output := widget.NewMultiLineEntry()
	output.TextStyle.Monospace = true
	output.SetMinRowsVisible(7)
	// A disabled Fyne entry still supports mouse selection and the Copy /
	// Select all context menu, while preventing edits to calculated values.
	output.Disable()
	validation := widget.NewLabel("")
	validation.Wrapping = fyne.TextWrapWord
	summary := widget.NewLabel("")
	summary.Wrapping = fyne.TextWrapWord
	aliasSummary := widget.NewLabel("")
	aliasSummary.Wrapping = fyne.TextWrapWord
	preview := widget.NewLabel("")
	preview.Wrapping = fyne.TextWrapWord
	var records []bios.Record

	copyValues := func(value func(bios.Record) string) {
		if len(records) == 0 {
			return
		}
		parts := make([]string, len(records))
		for i, record := range records {
			parts[i] = value(record)
		}
		w.App.Clipboard().SetContent(strings.Join(parts, "\n"))
	}
	copyAddress := widget.NewButton(w.t("Адрес (HL)", "Address (HL)"), func() {
		copyValues(func(r bios.Record) string { return dcp.Binary14(r.Address) })
	})
	copyMask := widget.NewButton(w.t("Маска (DE)", "Mask (DE)"), func() {
		copyValues(func(r bios.Record) string { return dcp.Binary14(r.Mask) })
	})
	copyCode := widget.NewButton(w.t("Код (B)", "Code (B)"), func() {
		copyValues(func(r bios.Record) string { return dcp.Binary8(r.Code) })
	})
	copyAll := widget.NewButton(w.t("Копировать всё", "Copy all"), func() {
		if len(records) != 0 {
			w.App.Clipboard().SetContent(output.Text)
		}
	})
	copyButtons := []*widget.Button{copyAddress, copyMask, copyCode, copyAll}

	updatePreview := func() {
		if len(records) == 0 {
			preview.SetText("")
			return
		}
		if w.doc == nil {
			preview.SetText(w.t("Откройте BIN для сравнения с текущими назначениями.", "Open a BIN to compare with its current assignments."))
			return
		}
		matched, different := 0, 0
		for _, record := range records {
			for _, offset := range record.Offsets() {
				if w.doc.Value(offset) == record.Code {
					matched++
				} else {
					different++
				}
			}
		}
		preview.SetText(fmt.Sprintf(w.t("Текущий BIN: %d записей совпадают, %d отличаются.\nГенератор рассчитывает значения; карта не изменяется.", "Current BIN: %d entries match, %d differ.\nThe generator calculates values; the map is unchanged."), matched, different))
	}
	// Document edits refresh only the comparison; input and calculated output
	// remain stable while another tab edits the map.
	w.refreshers = append(w.refreshers, updatePreview)

	clear := func(message string) {
		records = nil
		output.SetText("")
		summary.SetText("")
		aliasSummary.SetText("")
		validation.SetText(message)
		for _, button := range copyButtons {
			button.Disable()
		}
		updatePreview()
	}
	boolValues := func(index int) []bool {
		if index == 2 {
			return []bool{false, true}
		}
		return []bool{index == 1}
	}
	var syncingName bool
	calculate := func() {
		state.port = port.Text
		state.code = code.Text
		state.cnf = append([]string(nil), cnf.Selected...)
		state.direction = direction.SelectedIndex()
		state.dos = dos.SelectedIndex()
		state.pn5 = pn5.SelectedIndex()
		state.format = format.SelectedIndex()
		portValue, err := dcp.ParseNumber(port.Text, 16)
		if err != nil {
			clear(w.t("Введите внешний порт: HEX 0000…FFFF или %двоичное число.", "Enter an external port: HEX 0000…FFFF or a %binary number."))
			return
		}
		codeValue, err := dcp.ParseNumber(code.Text, 8)
		if err != nil {
			clear(w.t("Введите внутренний код: HEX 00…FF или %двоичное число.", "Enter an internal code: HEX 00…FF or a %binary number."))
			return
		}
		if len(cnf.Selected) == 0 {
			clear(w.t("Выберите хотя бы одну карту CNF.", "Select at least one CNF map."))
			return
		}
		maps := make([]int, len(cnf.Selected))
		for i, selected := range cnf.Selected {
			maps[i] = int(selected[0] - '0')
		}
		rule := bios.Rule{
			Port: portValue, Code: byte(codeValue), CNF: maps,
			Directions: boolValues(direction.SelectedIndex()),
			DOSOff:     boolValues(dos.SelectedIndex()),
			PN5:        boolValues(pn5.SelectedIndex()),
		}
		records, err = bios.Generate(rule)
		if err != nil {
			clear(err.Error())
			return
		}
		validation.SetText("")
		for _, button := range copyButtons {
			button.Enable()
		}
		if format.SelectedIndex() == 1 {
			output.SetText(bios.Table(records))
		} else {
			output.SetText(bios.Values(records))
		}
		cells := 0
		for _, record := range records {
			cells += len(record.Offsets())
		}
		summary.SetText(fmt.Sprintf(w.t("Правил: %d  •  Охват: %d записей карты", "Rules: %d  •  Covers %d map entries"), len(records), cells))
		aliasSummary.SetText(fmt.Sprintf(w.t("Группа внешних адресов: %04X / E0E7h — 128 алиасов.\nНесмежные карты CNF разделяются на точные правила.", "External address group: %04X / E0E7h — 128 aliases.\nNonadjacent CNF maps are split into exact rules."), dcp.Canonical(portValue)))
		updatePreview()
	}
	port.OnChanged = func(string) { calculate() }
	code.OnChanged = func(text string) {
		if !syncingName {
			syncingName = true
			value, err := dcp.ParseNumber(text, 8)
			if err == nil && namesByCode[value] != "" {
				namedCode.SetSelected(namesByCode[value])
			} else {
				namedCode.ClearSelected()
			}
			syncingName = false
		}
		calculate()
	}
	namedCode.OnChanged = func(name string) {
		if syncingName {
			return
		}
		if value, ok := codesByName[name]; ok {
			syncingName = true
			code.SetText(fmt.Sprintf("%02X", value))
			syncingName = false
		}
	}
	cnf.OnChanged = func([]string) { calculate() }
	direction.OnChanged = func(string) { calculate() }
	dos.OnChanged = func(string) { calculate() }
	pn5.OnChanged = func(string) { calculate() }
	format.OnChanged = func(string) { calculate() }

	inputs := widget.NewForm(
		widget.NewFormItem(w.t("Внешний порт", "External port"), port),
		widget.NewFormItem(w.t("Внутренний код", "Internal code"), code),
		widget.NewFormItem(w.t("Имя кода", "Code name"), namedCode),
		widget.NewFormItem("CNF", cnf),
		widget.NewFormItem(w.t("Направление", "Direction"), direction),
		widget.NewFormItem("DOS", dos),
		widget.NewFormItem("PN5", pn5),
	)
	legend := mono(w.t(
		"13…12: CNF   11: PN5   10: /DOS\n9: IN=1 / OUT=0\n8…0: P15 P14 P6 P5 P13 P7 P2 P1 P0\nDE: 1 — фиксировать, 0 — оба значения",
		"13…12: CNF   11: PN5   10: /DOS\n9: IN=1 / OUT=0\n8…0: P15 P14 P6 P5 P13 P7 P2 P1 P0\nDE: 1 = fixed, 0 = both values"))
	legend.Wrapping = fyne.TextWrapWord
	left := container.NewVScroll(container.NewVBox(inputs, validation, widget.NewSeparator(), summary, aliasSummary, widget.NewSeparator(), preview, widget.NewSeparator(), legend))
	right := container.NewBorder(
		container.NewBorder(nil, nil, widget.NewLabel(w.t("Представление", "Format")), nil, format),
		container.NewVBox(widget.NewLabel(w.t("Копировать значения регистров:", "Copy register values:")), container.NewGridWithColumns(3, copyAddress, copyMask, copyCode), copyAll),
		nil, nil, w.readOnly(output),
	)
	split := container.NewHSplit(left, right)
	split.Offset = 0.50
	calculate()
	return container.NewPadded(split)
}
