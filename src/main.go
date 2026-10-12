package main

import (
	"flag"
	"fmt"
	"os"

	"fyne.io/fyne/v2/app"
	"godcp/internal/ui"
)

const version = "0.1.0"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("GoDCP", version)
		return
	}
	a := app.NewWithID("org.godcp.editor")
	w := ui.New(a)
	if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "Usage: GoDCP [map.bin]")
		os.Exit(2)
	}
	if flag.NArg() == 1 {
		if err := w.OpenPath(flag.Arg(0)); err != nil {
			w.ShowError(err)
		}
	}
	w.Window.ShowAndRun()
}
