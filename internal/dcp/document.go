package dcp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type change struct {
	offset int
	before byte
	after  byte
}

// Document keeps loaded bytes, the last saved bytes, and reversible byte edits.
// It is intended to be owned by one GUI thread and is not concurrency safe.
type Document struct {
	Path     string
	data     [Size]byte
	saved    [Size]byte
	history  []change
	cursor   int
	modified int
}

// NewDocument copies data. A port map must contain exactly Size bytes.
func NewDocument(data []byte, path string) (*Document, error) {
	if len(data) != Size {
		return nil, fmt.Errorf("port map must contain exactly %d bytes; got %d", Size, len(data))
	}
	d := &Document{Path: path}
	copy(d.data[:], data)
	copy(d.saved[:], data)
	return d, nil
}

// Load reads and validates a complete BIN without altering it.
func Load(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, Size+1))
	if err != nil {
		return nil, err
	}
	return NewDocument(data, path)
}

// Bytes returns an independent copy of the current contents.
func (d *Document) Bytes() []byte { return append([]byte(nil), d.data[:]...) }

// Value returns a map byte. An invalid offset is a programming error.
func (d *Document) Value(offset int) byte { return d.data[offset] }

func (d *Document) apply(offset int, value byte) {
	if d.data[offset] != d.saved[offset] {
		d.modified--
	}
	d.data[offset] = value
	if d.data[offset] != d.saved[offset] {
		d.modified++
	}
}

// Set records a single byte replacement. Setting the same value is a no-op.
func (d *Document) Set(offset int, value byte) error {
	if offset < 0 || offset >= Size {
		return fmt.Errorf("offset must be between 0 and %04X", Size-1)
	}
	if d.data[offset] == value {
		return nil
	}
	d.history = append(d.history[:d.cursor], change{offset, d.data[offset], value})
	d.cursor++
	d.apply(offset, value)
	return nil
}

func (d *Document) CanUndo() bool     { return d.cursor > 0 }
func (d *Document) CanRedo() bool     { return d.cursor < len(d.history) }
func (d *Document) Dirty() bool       { return d.modified != 0 }
func (d *Document) ChangedCount() int { return d.modified }

// Undo reverts one edit and returns whether an edit was available.
func (d *Document) Undo() bool {
	if !d.CanUndo() {
		return false
	}
	d.cursor--
	c := d.history[d.cursor]
	d.apply(c.offset, c.before)
	return true
}

// Redo reapplies one reverted edit and returns whether it was available.
func (d *Document) Redo() bool {
	if !d.CanRedo() {
		return false
	}
	c := d.history[d.cursor]
	d.cursor++
	d.apply(c.offset, c.after)
	return true
}

// Save writes the complete BIN to a temporary sibling file, flushes it, and
// replaces the destination. A failure leaves the existing destination and the
// document's saved state intact. Empty path means the current document path.
// Undo history is retained, so undoing a saved edit correctly makes it dirty.
func (d *Document) Save(path string) error {
	if path == "" {
		path = d.Path
	}
	if path == "" {
		return fmt.Errorf("choose a destination file")
	}
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("destination is not a regular file: %s", path)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".godcp-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		// A read-only destination may cause the replacement to fail on Windows.
		// Restore permissions on our own temporary file so it can still be removed.
		_ = os.Chmod(temp, 0600)
		_ = os.Remove(temp)
	}()
	if _, err = f.Write(d.data[:]); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = replaceFile(temp, path); err != nil {
		return err
	}
	d.Path = path
	d.saved = d.data
	d.modified = 0
	return nil
}
