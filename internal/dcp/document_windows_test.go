//go:build windows

package dcp

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWindowsReplacementFailureKeepsOriginal(t *testing.T) {
	d := testDocument(t)
	path := filepath.Join(t.TempDir(), "locked.bin")
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	d.Set(5, 0xA5)
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// Disallow delete sharing, as an external application holding the map might.
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	if err := d.Save(path); err == nil {
		t.Fatal("replacing an externally locked destination should fail")
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, make([]byte, Size)) {
		t.Fatal("failed replacement changed existing bytes", err)
	}
	if !d.Dirty() || d.Value(5) != 0xA5 || d.Path != path || !d.CanUndo() {
		t.Fatal("failed replacement changed document state")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("failed replacement leaked its temporary file", err)
	}
}

func TestWindowsReadOnlyDestinationKeepsOriginal(t *testing.T) {
	d := testDocument(t)
	path := filepath.Join(t.TempDir(), "readonly.bin")
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	d.Set(25, 0x7F)
	if err := d.Save(path); err == nil {
		t.Fatal("replacing a read-only destination should fail")
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, make([]byte, Size)) {
		t.Fatal("failed replacement changed existing bytes", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("failed replacement leaked a read-only temporary file", err)
	}
}
