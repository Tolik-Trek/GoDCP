package ui

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/dialog"
)

func (w *Workbench) saveTo(path string) {
	perform := func() {
		if err := w.doc.Save(path); err != nil {
			w.ShowError(err)
			return
		}
		w.App.Preferences().SetString("lastDirectory", filepath.Dir(path))
		w.refresh()
	}
	_, err := os.Stat(path)
	if err == nil {
		d := dialog.NewConfirm(w.t("Заменить файл?", "Replace file?"), path, func(ok bool) {
			if ok {
				perform()
			}
		}, w.Window)
		d.SetConfirmText(w.t("Заменить", "Replace"))
		d.SetDismissText(w.t("Отмена", "Cancel"))
		d.Show()
		return
	}
	if !os.IsNotExist(err) {
		w.ShowError(err)
		return
	}
	perform()
}
