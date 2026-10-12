package dcp

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func testDocument(t *testing.T) *Document {
	t.Helper()
	d, err := NewDocument(make([]byte, Size), "")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDocumentCopiesInputAndOutput(t *testing.T) {
	input := make([]byte, Size)
	input[32] = 0xFF
	d, err := NewDocument(input, "example.bin")
	if err != nil {
		t.Fatal(err)
	}
	input[32] = 0
	output := d.Bytes()
	output[32] = 1
	if d.Value(32) != 0xFF || d.Dirty() || d.CanUndo() {
		t.Fatal("document contents escaped ownership or were modified")
	}
	for _, size := range []int{0, Size - 1, Size + 1} {
		if _, err := NewDocument(make([]byte, size), ""); err == nil {
			t.Errorf("accepted invalid map size %d", size)
		}
	}
}

func TestUndoRedoAndChangedCount(t *testing.T) {
	d := testDocument(t)
	if d.Undo() || d.Redo() {
		t.Fatal("empty document has undo/redo actions")
	}
	for _, offset := range []int{-1, Size} {
		if err := d.Set(offset, 1); err == nil {
			t.Errorf("accepted invalid offset %d", offset)
		}
	}
	d.Set(5, 0xFF)
	d.Set(5, 0xAA)
	d.Set(6, 0x18) // Unknown codes are preserved just like named codes.
	if d.ChangedCount() != 2 || !d.Dirty() {
		t.Fatal("change count must count distinct changed bytes")
	}
	d.Set(5, 0xAA) // Same-byte edits do not consume undo history.
	if !d.Undo() || d.Value(6) != 0 || d.ChangedCount() != 1 {
		t.Fatal("undo did not restore previous byte")
	}
	if !d.Undo() || d.Value(5) != 0xFF {
		t.Fatal("undo did not restore the previous value at the same offset")
	}
	if !d.Undo() || d.Dirty() || d.ChangedCount() != 0 {
		t.Fatal("full undo did not restore the original bytes")
	}
	d.Redo()
	d.Set(7, 0x1A)
	if d.CanRedo() || d.Value(7) != 0x1A || d.ChangedCount() != 2 {
		t.Fatal("edit after undo did not replace the redo branch")
	}
	d.Set(5, 0)
	d.Set(7, 0)
	if d.Dirty() {
		t.Fatal("explicitly restoring original bytes must clear dirty status")
	}
}

func TestExampleProbesAndExactRoundTrip(t *testing.T) {
	path := filepath.Join("..", "..", "Example", "DCP_PAGE.bin")
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	probes := map[int]byte{0x0056: 0xC3, 0x0256: 0, 0x01FD: 0x90, 0x03FD: 0x52, 0x017D: 0x91, 0x046D: 0xC0, 0x146D: 0xC0, 0x246D: 0xC1, 0x346D: 0xC0}
	for offset, want := range probes {
		if got := d.Value(offset); got != want {
			t.Errorf("saved example at %04X = %02X, want %02X", offset, got, want)
		}
	}
	nonzero := 0
	for _, b := range input {
		if b != 0 {
			nonzero++
		}
	}
	if nonzero != 3904 {
		t.Errorf("saved example contains %d assigned bytes, want 3904", nonzero)
	}
	destination := filepath.Join(t.TempDir(), "roundtrip.bin")
	if err := d.Save(destination); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(input, saved) {
		t.Fatal("unedited save did not preserve every source byte", err)
	}
	if d.Path != destination || d.Dirty() {
		t.Fatal("save did not update document path and baseline")
	}
}

func TestSaveReplacesExistingAndRetainsUndo(t *testing.T) {
	d := testDocument(t)
	path := filepath.Join(t.TempDir(), "map.bin")
	if err := os.WriteFile(path, []byte("older file"), 0600); err != nil {
		t.Fatal(err)
	}
	d.Set(0, 0xA5)
	d.Set(Size-1, 0xFF)
	if err := d.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != Size || data[0] != 0xA5 || data[Size-1] != 0xFF {
		t.Fatalf("incorrect saved map: length=%d error=%v", len(data), err)
	}
	if d.Dirty() || !d.CanUndo() {
		t.Fatal("save must set a new baseline and retain history")
	}
	if !d.Undo() || !d.Dirty() || d.ChangedCount() != 1 {
		t.Fatal("undo across save should differ from saved baseline")
	}
	if !d.Redo() || d.Dirty() {
		t.Fatal("redo to saved state should clear dirty status")
	}
	d.Set(123, 0x19)
	if err := d.Save(""); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil || reloaded.Value(123) != 0x19 {
		t.Fatal("saving to current path failed", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("save leaked a temporary file", err)
	}
}

func TestFailedSavePreservesDocumentAndDestination(t *testing.T) {
	d := testDocument(t)
	originalPath := filepath.Join(t.TempDir(), "original.bin")
	if err := d.Save(originalPath); err != nil {
		t.Fatal(err)
	}
	d.Set(20, 0x80)
	badTarget := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(badTarget, 0700); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(badTarget); err == nil {
		t.Fatal("save to a directory should fail")
	}
	if err := d.Save(filepath.Join(badTarget, "missing", "map.bin")); err == nil {
		t.Fatal("save to nonexistent parent should fail")
	}
	if d.Path != originalPath || !d.Dirty() || d.ChangedCount() != 1 || !d.CanUndo() {
		t.Fatal("failed save changed document state")
	}
	data, err := os.ReadFile(originalPath)
	if err != nil || !bytes.Equal(data, make([]byte, Size)) {
		t.Fatal("failed save changed the original file", err)
	}
	if err := os.WriteFile(filepath.Join(badTarget, "bad.bin"), []byte{1, 2, 3}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(badTarget, "bad.bin")); err == nil {
		t.Fatal("Load accepted truncated input")
	}
}
