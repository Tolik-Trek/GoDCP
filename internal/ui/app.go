// Package ui contains the desktop shell. All map calculations live outside it.
package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"godcp/internal/catalog"
	"godcp/internal/dcp"
)

type Workbench struct {
	App                          fyne.App
	Window                       fyne.Window
	doc                          *dcp.Document
	language                     string
	tabs                         *container.AppTabs
	status, file                 *widget.Label
	undo, redo, save             *widget.Button
	refreshers                   []func()
	focusEditor                  func(int)
	selected                     int
	externalQuery, internalQuery string
	editorFilter                 filterState
	assignedFilter               filterState
	internalFilter               filterState
	hexMode                      bool
	generatorState               *generatorState
	editorDraft                  string
	editorDraftOffset            int
	editorDraftSet               bool
}

func New(a fyne.App) *Workbench {
	w := &Workbench{App: a, Window: a.NewWindow("GoDCP"), language: a.Preferences().StringWithFallback("language", "ru"), externalQuery: "204E", internalQuery: "C3"}
	w.applyTheme(a.Preferences().StringWithFallback("theme", "system"))
	w.Window.Resize(fyne.NewSize(1180, 760))
	w.render()
	w.installShortcuts()
	w.Window.SetCloseIntercept(func() { w.confirmDiscard(func() { w.Window.SetCloseIntercept(nil); w.Window.Close() }) })
	w.Window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if len(uris) > 0 {
			w.confirmDiscard(func() {
				if err := w.OpenPath(uris[0].Path()); err != nil {
					w.ShowError(err)
				}
			})
		}
	})
	return w
}

func (w *Workbench) t(ru, en string) string {
	if w.language == "en" {
		return en
	}
	return ru
}

func (w *Workbench) render() {
	index := 0
	if w.tabs != nil {
		index = w.tabs.SelectedIndex()
	}
	w.refreshers = nil
	w.file = widget.NewLabel("")
	w.file.Truncation = fyne.TextTruncateEllipsis
	w.status = widget.NewLabel("")
	w.undo = widget.NewButtonWithIcon(w.t("Отменить", "Undo"), theme.ContentUndoIcon(), w.undoEdit)
	w.redo = widget.NewButtonWithIcon(w.t("Повторить", "Redo"), theme.ContentRedoIcon(), w.redoEdit)
	w.save = widget.NewButtonWithIcon(w.t("Сохранить", "Save"), theme.DocumentSaveIcon(), func() { w.saveDocument(false) })
	tools := container.NewHBox(
		widget.NewButtonWithIcon(w.t("Открыть", "Open"), theme.FolderOpenIcon(), w.openDialog),
		w.save, widget.NewButton(w.t("Сохранить как…", "Save as…"), func() { w.saveDocument(true) }),
		w.undo, w.redo,
	)
	top := container.NewVBox(container.NewBorder(nil, nil, tools, widget.NewButtonWithIcon("", theme.SettingsIcon(), w.settings), w.file), widget.NewSeparator())
	w.tabs = container.NewAppTabs(
		container.NewTabItem(w.t("Редактор карты", "Map editor"), w.editor()),
		container.NewTabItem(w.t("Назначенные порты", "Assigned ports"), w.assigned()),
		container.NewTabItem(w.t("По внутреннему порту", "By internal port"), w.internalLookup()),
		container.NewTabItem(w.t("По внешнему порту", "By external port"), w.externalLookup()),
		container.NewTabItem(w.t("Генератор BIOS", "BIOS generator"), w.generator()),
	)
	w.tabs.SelectIndex(index)
	w.Window.SetContent(container.NewBorder(top, container.NewVBox(widget.NewSeparator(), w.status), nil, nil, w.tabs))
	w.refresh()
}

func (w *Workbench) refresh() {
	w.undo.Disable()
	w.redo.Disable()
	w.save.Disable()
	if w.doc == nil {
		w.file.SetText(w.t("Откройте карту BIN (16 КБ)", "Open a 16 KiB BIN map"))
		w.status.SetText(w.t("Генератор BIOS доступен без файла. Ctrl+O — открыть карту.", "BIOS generator works without a file. Ctrl+O — open map."))
		w.Window.SetTitle("GoDCP")
	} else {
		if w.doc.CanUndo() {
			w.undo.Enable()
		}
		if w.doc.CanRedo() {
			w.redo.Enable()
		}
		if w.doc.Dirty() {
			w.save.Enable()
		}
		mark := ""
		if w.doc.Dirty() {
			mark = " *"
		}
		w.file.SetText(w.doc.Path + mark)
		w.Window.SetTitle(filepath.Base(w.doc.Path) + mark + " — GoDCP")
		w.status.SetText(fmt.Sprintf(w.t("16 384 байта  •  Изменено: %d  •  Все вкладки используют текущие данные", "16,384 bytes  •  Changed: %d  •  All tabs use current data"), w.doc.ChangedCount()))
	}
	for _, refresh := range w.refreshers {
		refresh()
	}
}

func (w *Workbench) OpenPath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	path = absolute
	d, err := dcp.Load(path)
	if err != nil {
		return err
	}
	w.doc = d
	w.editorDraftSet = false
	w.selected = 0
	w.refresh()
	if w.focusEditor != nil {
		w.focusEditor(0)
	}
	w.App.Preferences().SetString("lastDirectory", filepath.Dir(path))
	return nil
}

func (w *Workbench) ShowError(err error) { dialog.ShowError(err, w.Window) }
func (w *Workbench) confirmDiscard(action func()) {
	if w.doc == nil || !w.doc.Dirty() {
		action()
		return
	}
	d := dialog.NewConfirm(w.t("Несохранённые изменения", "Unsaved changes"), w.t("Продолжить и отбросить изменения карты?", "Continue and discard the map changes?"), func(ok bool) {
		if ok {
			action()
		}
	}, w.Window)
	d.SetConfirmText(w.t("Отбросить", "Discard"))
	d.SetDismissText(w.t("Назад", "Back"))
	d.Show()
}

func (w *Workbench) openDialog() {
	w.confirmDiscard(func() {
		d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil {
				w.ShowError(err)
				return
			}
			if r == nil {
				return
			}
			path := r.URI().Path()
			_ = r.Close()
			if err := w.OpenPath(path); err != nil {
				w.ShowError(err)
			}
		}, w.Window)
		d.SetFilter(storage.NewExtensionFileFilter([]string{".bin"}))
		w.setDialogDirectory(d)
		d.Resize(fyne.NewSize(850, 550))
		d.Show()
	})
}

func (w *Workbench) setDialogDirectory(d *dialog.FileDialog) {
	path := w.App.Preferences().String("lastDirectory")
	if path != "" {
		if dir, err := storage.ListerForURI(storage.NewFileURI(path)); err == nil {
			d.SetLocation(dir)
		}
	}
}

func (w *Workbench) saveDocument(as bool) {
	if w.doc == nil {
		return
	}
	if !as && w.doc.Path != "" {
		if err := w.doc.Save(w.doc.Path); err != nil {
			w.ShowError(err)
			return
		}
		w.refresh()
		return
	}
	// A picker must not create/truncate the destination: Document.Save owns the atomic write.
	name := widget.NewEntry()
	name.SetText(filepath.Base(w.doc.Path))
	if name.Text == "." || name.Text == "" {
		name.SetText("DCP_PAGE.bin")
	}
	d := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			w.ShowError(err)
			return
		}
		if uri == nil {
			return
		}
		form := dialog.NewForm(w.t("Сохранить карту", "Save map"), w.t("Сохранить", "Save"), w.t("Отмена", "Cancel"), []*widget.FormItem{widget.NewFormItem(w.t("Имя файла", "File name"), name)}, func(ok bool) {
			if !ok {
				return
			}
			n := strings.TrimSpace(name.Text)
			if n == "" || n == "." || n == ".." || strings.ContainsAny(n, "/\\") {
				w.ShowError(fmt.Errorf("%s", w.t("Введите имя файла без пути", "Enter a file name without a path")))
				return
			}
			path := filepath.Join(uri.Path(), n)
			w.saveTo(path)
		}, w.Window)
		form.Resize(fyne.NewSize(460, 160))
		form.Show()
	}, w.Window)
	w.setDialogDirectory(d)
	d.Show()
}

func (w *Workbench) undoEdit() {
	if w.doc != nil && w.doc.Undo() {
		w.editorDraftSet = false
		w.refresh()
	}
}
func (w *Workbench) redoEdit() {
	if w.doc != nil && w.doc.Redo() {
		w.editorDraftSet = false
		w.refresh()
	}
}
func (w *Workbench) setByte(offset int, value byte) {
	if w.doc != nil {
		if err := w.doc.Set(offset, value); err != nil {
			w.ShowError(err)
			return
		}
		w.editorDraftSet = false
		w.refresh()
	}
}
func (w *Workbench) navigate(offset int) {
	w.selected = offset
	w.tabs.SelectIndex(0)
	if w.focusEditor != nil {
		w.focusEditor(offset)
	}
}

func (w *Workbench) installShortcuts() {
	modifier := fyne.KeyModifierControl
	if runtime.GOOS == "darwin" {
		modifier = fyne.KeyModifierSuper
	}
	for _, s := range []struct {
		key    fyne.KeyName
		mod    fyne.KeyModifier
		action func()
	}{
		{fyne.KeyO, modifier, w.openDialog}, {fyne.KeyS, modifier, func() { w.saveDocument(false) }},
		{fyne.KeyS, modifier | fyne.KeyModifierShift, func() { w.saveDocument(true) }},
		{fyne.KeyZ, modifier, w.undoEdit}, {fyne.KeyZ, modifier | fyne.KeyModifierShift, w.redoEdit}, {fyne.KeyY, modifier, w.redoEdit},
	} {
		action := s.action
		w.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: s.key, Modifier: s.mod}, func(fyne.Shortcut) { action() })
	}
}

func (w *Workbench) codeName(v byte) string {
	if v == 0 {
		return w.t("Нет назначения", "Unassigned")
	}
	if n := catalog.Name(v); n != "" {
		return n
	}
	return w.t("Неизвестный код", "Unknown code")
}
func (w *Workbench) direction(read bool) string {
	if read {
		return "IN"
	}
	return "OUT"
}
func (w *Workbench) dos(off bool) string {
	if off {
		return "OFF"
	}
	return "ON"
}
func mono(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle.Monospace = true
	return l
}
func bit(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (w *Workbench) settings() {
	lang := widget.NewSelect([]string{"Русский", "English"}, nil)
	if w.language == "en" {
		lang.SetSelected("English")
	} else {
		lang.SetSelected("Русский")
	}
	skin := widget.NewSelect([]string{w.t("Системная", "System"), w.t("Светлая", "Light"), w.t("Тёмная", "Dark")}, nil)
	current := w.App.Preferences().StringWithFallback("theme", "system")
	i := 0
	if current == "light" {
		i = 1
	}
	if current == "dark" {
		i = 2
	}
	skin.SetSelectedIndex(i)
	d := dialog.NewForm(w.t("Настройки", "Settings"), w.t("Применить", "Apply"), w.t("Отмена", "Cancel"), []*widget.FormItem{widget.NewFormItem(w.t("Язык", "Language"), lang), widget.NewFormItem(w.t("Тема", "Theme"), skin)}, func(ok bool) {
		if !ok {
			return
		}
		w.language = "ru"
		if lang.SelectedIndex() == 1 {
			w.language = "en"
		}
		w.App.Preferences().SetString("language", w.language)
		w.applyTheme([]string{"system", "light", "dark"}[skin.SelectedIndex()])
		w.render()
	}, w.Window)
	d.Show()
}

type compactTheme struct{ mode string }

func (t compactTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if t.mode == "light" {
		v = theme.VariantLight
	}
	if t.mode == "dark" {
		v = theme.VariantDark
	}
	return theme.DefaultTheme().Color(n, v)
}
func (t compactTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (t compactTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (t compactTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNamePadding {
		return 3
	}
	if n == theme.SizeNameText {
		return 13
	}
	return theme.DefaultTheme().Size(n)
}
func (w *Workbench) applyTheme(mode string) {
	w.App.Preferences().SetString("theme", mode)
	w.App.Settings().SetTheme(compactTheme{mode: mode})
}

// Calculated values are read-only, but remain as legible as ordinary text.
type readOnlyTheme struct{ fyne.Theme }

func (t readOnlyTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameDisabled {
		name = theme.ColorNameForeground
	}
	return t.Theme.Color(name, variant)
}
func (w *Workbench) readOnly(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewThemeOverride(obj, readOnlyTheme{w.App.Settings().Theme()})
}
