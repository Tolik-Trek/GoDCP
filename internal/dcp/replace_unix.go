//go:build !windows

package dcp

import "os"

func replaceFile(source, target string) error { return os.Rename(source, target) }
