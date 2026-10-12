package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"godcp/internal/dcp"
)

type grid struct {
	table    *widget.Table
	headers  []string
	rows     [][]string
	selected int
}

func newGrid(headers []string, widths []float32, onSelect func(int)) *grid {
	g := &grid{headers: headers, selected: -1}
	g.table = widget.NewTable(func() (int, int) { return len(g.rows) + 1, len(g.headers) }, func() fyne.CanvasObject { return mono("") }, func(id widget.TableCellID, obj fyne.CanvasObject) {
		l := obj.(*widget.Label)
		l.TextStyle = fyne.TextStyle{Monospace: true, Bold: id.Row == 0}
		l.Truncation = fyne.TextTruncateEllipsis
		if id.Row == 0 {
			l.SetText(g.headers[id.Col])
		} else if id.Row <= len(g.rows) {
			l.SetText(g.rows[id.Row-1][id.Col])
		}
	})
	for i, width := range widths {
		g.table.SetColumnWidth(i, width)
	}
	g.table.StickyRowCount = 1
	g.table.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Row <= len(g.rows) {
			g.selected = id.Row - 1
			if onSelect != nil {
				onSelect(g.selected)
			}
		}
	}
	return g
}
func (g *grid) setRows(rows [][]string) {
	g.rows = rows
	g.selected = -1
	g.table.UnselectAll()
	g.table.Refresh()
}

func (w *Workbench) filters(f *filterState, changed func()) fyne.CanvasObject {
	all := w.t("Все", "All")
	selector := func(options []string, current *int) *widget.Select {
		s := widget.NewSelect(options, nil)
		s.SetSelectedIndex(*current)
		s.OnChanged = func(string) { *current = s.SelectedIndex(); changed() }
		return s
	}
	cnf := selector([]string{all, "0", "1", "2", "3"}, &f.cnf)
	dir := selector([]string{all, "OUT", "IN"}, &f.direction)
	dos := selector([]string{all, "ON", "OFF"}, &f.dos)
	pn5 := selector([]string{all, "0", "1"}, &f.pn5)
	q := widget.NewEntry()
	q.SetPlaceHolder(w.t("Код, имя, адрес или смещение…", "Code, name, address or offset…"))
	q.SetText(f.query)
	q.OnChanged = func(s string) { f.query = s; changed() }
	field := widget.NewSelect([]string{
		w.t("Все поля", "All fields"), w.t("Адрес", "Address"),
		w.t("Смещение", "Offset"), w.t("Код", "Code"), w.t("Имя", "Name"),
	}, nil)
	field.SetSelectedIndex(int(f.field))
	q.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" || f.field == searchAll || f.field == searchName {
			return nil
		}
		bits := 16
		if f.field == searchOffset {
			bits = 14
		} else if f.field == searchCode {
			bits = 8
		}
		if _, err := dcp.ParseNumber(s, bits); err != nil {
			return fmt.Errorf(w.t("Введите HEX-число 0–%X или двоичное с %%", "Enter a HEX number 0–%X or binary with %%"), (1<<bits)-1)
		}
		return nil
	}
	hint := widget.NewLabel("")
	hint.Wrapping = fyne.TextWrapWord
	updateHint := func() {
		switch f.field {
		case searchAddress:
			hint.SetText(w.t("Точный адрес HEX 0000–FFFF; находит группу любого из её 128 алиасов.", "Exact HEX address 0000–FFFF; finds the group of any of its 128 aliases."))
		case searchOffset:
			hint.SetText(w.t("Точное смещение HEX 0000–3FFF в загруженном BIN.", "Exact HEX offset 0000–3FFF in the loaded BIN."))
		case searchCode:
			hint.SetText(w.t("Точный внутренний код HEX 00–FF.", "Exact internal HEX code 00–FF."))
		case searchName:
			hint.SetText(w.t("Часть имени внутреннего порта, без учёта регистра.", "Part of an internal port name, case insensitive."))
		default:
			hint.SetText(w.t("Число: точное совпадение в любом поле, адрес включает алиасы. Имя: часть строки. Для поиска по столбцу выберите поле.", "Number: exact match in any field, addresses include aliases. Name: substring. Choose a field to search one column."))
		}
	}
	field.OnChanged = func(string) {
		f.field = searchField(field.SelectedIndex())
		updateHint()
		_ = q.Validate()
		changed()
	}
	updateHint()
	_ = q.Validate()
	clear := widget.NewButton(w.t("Очистить", "Clear"), func() { q.SetText("") })
	selectors := container.NewHBox(widget.NewLabel("CNF"), cnf, widget.NewLabel("I/O"), dir, widget.NewLabel("DOS"), dos, widget.NewLabel("PN5"), pn5)
	search := container.NewBorder(nil, nil, container.NewHBox(widget.NewLabel(w.t("Поиск", "Search")), field), clear, q)
	return container.NewVBox(selectors, search, hint)
}

func (w *Workbench) row(offset int) []string {
	p, c := dcp.Decode(offset)
	v := w.doc.Value(offset)
	return []string{fmt.Sprintf("%04X", offset), fmt.Sprintf("%04X", p), fmt.Sprint(c.CNF), w.direction(c.Read), w.dos(c.DOSOff), fmt.Sprint(bit(c.PN5)), fmt.Sprintf("%02X", v), w.codeName(v)}
}
func (w *Workbench) headers() []string {
	return []string{w.t("Смещение", "Offset"), w.t("Адрес*", "Port*"), "CNF", "I/O", "DOS", "PN5", w.t("Код", "Code"), w.t("Внутренний порт", "Internal port")}
}

var rowWidths = []float32{85, 75, 44, 50, 50, 44, 48, 290}

func (w *Workbench) aliasesDialog(port uint16) {
	aliases := dcp.Aliases(port)
	var b strings.Builder
	fmt.Fprintf(&b, "(port & #E0E7) == #%04X\n\n", dcp.Canonical(port))
	for i, p := range aliases {
		fmt.Fprintf(&b, "%04X", p)
		if i%8 == 7 {
			b.WriteByte('\n')
		} else {
			b.WriteString("  ")
		}
	}
	text := widget.NewMultiLineEntry()
	text.TextStyle.Monospace = true
	text.SetText(b.String())
	content := container.NewBorder(nil, widget.NewButton(w.t("Копировать все адреса", "Copy all addresses"), func() { w.Window.Clipboard().SetContent(b.String()) }), nil, nil, text)
	d := dialog.NewCustom(w.t("128 адресных алиасов", "128 address aliases"), w.t("Закрыть", "Close"), content, w.Window)
	d.Resize(fyne.NewSize(630, 510))
	d.Show()
}

func (w *Workbench) assigned() fyne.CanvasObject {
	var offsets []int
	var g *grid
	count := widget.NewLabel("")
	detail := mono(w.t("* Адрес — минимальный представитель группы из 128 алиасов.", "* Port is the lowest representative of 128 aliases."))
	open := widget.NewButton(w.t("В редактор", "Edit selected"), func() {
		if g.selected >= 0 && g.selected < len(offsets) {
			w.navigate(offsets[g.selected])
		}
	})
	aliases := widget.NewButton(w.t("Показать алиасы", "Show aliases"), func() {
		if g.selected >= 0 && g.selected < len(offsets) {
			p, _ := dcp.Decode(offsets[g.selected])
			w.aliasesDialog(p)
		}
	})
	open.Disable()
	aliases.Disable()
	g = newGrid(w.headers(), rowWidths, func(i int) {
		if i < len(offsets) {
			open.Enable()
			aliases.Enable()
		}
	})
	refresh := func() {
		offsets = nil
		var rows [][]string
		open.Disable()
		aliases.Disable()
		groups := map[uint16]bool{}
		if w.doc != nil {
			for i := 0; i < dcp.Size; i++ {
				v := w.doc.Value(i)
				if v == 0 || !w.assignedFilter.matches(i, v, w.codeName(v)) {
					continue
				}
				offsets = append(offsets, i)
				rows = append(rows, w.row(i))
				p, _ := dcp.Decode(i)
				groups[p] = true
			}
		}
		g.setRows(rows)
		count.SetText(fmt.Sprintf(w.t("Назначений: %d  •  Адресных групп: %d  •  Алиасов на группу: 128", "Assignments: %d  •  Address groups: %d  •  Aliases per group: 128"), len(rows), len(groups)))
	}
	w.refreshers = append(w.refreshers, refresh)
	return container.NewBorder(container.NewVBox(w.filters(&w.assignedFilter, refresh), count), container.NewVBox(detail, container.NewHBox(open, aliases)), nil, nil, g.table)
}

func (w *Workbench) internalLookup() fyne.CanvasObject {
	q := widget.NewEntry()
	q.SetText(w.internalQuery)
	q.SetPlaceHolder(w.t("Код HEX или точное имя", "HEX code or exact name"))
	var offsets []int
	var g *grid
	count := widget.NewLabel("")
	open := widget.NewButton(w.t("В редактор", "Edit selected"), func() {
		if g.selected >= 0 && g.selected < len(offsets) {
			w.navigate(offsets[g.selected])
		}
	})
	aliases := widget.NewButton(w.t("Алиасы выбранного адреса", "Selected port aliases"), func() {
		if g.selected >= 0 && g.selected < len(offsets) {
			p, _ := dcp.Decode(offsets[g.selected])
			w.aliasesDialog(p)
		}
	})
	g = newGrid(w.headers(), rowWidths, func(i int) {
		if i < len(offsets) {
			open.Enable()
			aliases.Enable()
		}
	})
	refresh := func() {
		offsets = nil
		var rows [][]string
		open.Disable()
		aliases.Disable()
		code, err := parseCode(q.Text)
		if err != nil {
			g.setRows(nil)
			count.SetText(w.t("Введите код 00–FF или имя из справочника", "Enter a code 00–FF or a catalog name"))
			return
		}
		groups := map[uint16]bool{}
		if w.doc != nil {
			for i := 0; i < dcp.Size; i++ {
				v := w.doc.Value(i)
				if v != code || !w.internalFilter.matches(i, v, w.codeName(v)) {
					continue
				}
				offsets = append(offsets, i)
				rows = append(rows, w.row(i))
				p, _ := dcp.Decode(i)
				groups[p] = true
			}
		}
		g.setRows(rows)
		count.SetText(fmt.Sprintf("%02X — %s   |   %s: %d   |   %s: %d", code, w.codeName(code), w.t("Записей", "Records"), len(rows), w.t("Адресных групп", "Address groups"), len(groups)))
	}
	q.OnChanged = func(s string) { w.internalQuery = s; refresh() }
	w.refreshers = append(w.refreshers, refresh)
	return container.NewBorder(container.NewVBox(container.NewBorder(nil, nil, widget.NewLabel(w.t("Внутренний порт", "Internal port")), nil, q), w.filters(&w.internalFilter, refresh), count), container.NewHBox(open, aliases), nil, nil, g.table)
}

func (w *Workbench) externalLookup() fyne.CanvasObject {
	q := widget.NewEntry()
	q.SetText(w.externalQuery)
	q.SetPlaceHolder("204E")
	info := widget.NewLabel("")
	info.Wrapping = fyne.TextWrapWord
	var offsets []int
	var g *grid
	open := widget.NewButton(w.t("В редактор", "Edit selected"), func() {
		if g.selected >= 0 && g.selected < len(offsets) {
			w.navigate(offsets[g.selected])
		}
	})
	aliases := widget.NewButton(w.t("Алиасы адреса", "Address aliases"), func() {
		if p, err := dcp.ParseNumber(q.Text, 16); err == nil {
			w.aliasesDialog(p)
		}
	})
	g = newGrid(w.headers(), rowWidths, func(i int) {
		if i < len(offsets) {
			open.Enable()
		}
	})
	refresh := func() {
		offsets = nil
		var rows [][]string
		open.Disable()
		aliases.Disable()
		p, err := dcp.ParseNumber(q.Text, 16)
		if err != nil {
			g.setRows(nil)
			info.SetText(w.t("Введите внешний адрес 0000–FFFF", "Enter an external address 0000–FFFF"))
			return
		}
		aliases.Enable()
		if w.doc != nil {
			for cnf := 0; cnf < 4; cnf++ {
				for pn5 := 0; pn5 < 2; pn5++ {
					for dos := 0; dos < 2; dos++ {
						for rd := 0; rd < 2; rd++ {
							c := dcp.Context{CNF: cnf, PN5: pn5 == 1, DOSOff: dos == 1, Read: rd == 1}
							o := dcp.Offset(p, c)
							offsets = append(offsets, o)
							row := w.row(o)
							row[1] = fmt.Sprintf("%04X", p)
							rows = append(rows, row)
						}
					}
				}
			}
		}
		g.setRows(rows)
		info.SetText(fmt.Sprintf(w.t("Адрес %04X  •  Группа %04X / маска E0E7  •  Все 32 сочетания режимов; 00 = нет назначения", "Port %04X  •  Group %04X / mask E0E7  •  All 32 contexts; 00 = unassigned"), p, dcp.Canonical(p)))
	}
	q.OnChanged = func(s string) { w.externalQuery = s; refresh() }
	w.refreshers = append(w.refreshers, refresh)
	return container.NewBorder(container.NewVBox(container.NewBorder(nil, nil, widget.NewLabel(w.t("Внешний порт HEX", "External port HEX")), nil, q), info), container.NewHBox(open, aliases), nil, nil, g.table)
}
