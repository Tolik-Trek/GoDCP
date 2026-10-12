package catalog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestKnownAndUnknownCodes(t *testing.T) {
	for code, want := range map[byte]string{
		0x52: "AY_FFFD_READ",
		0x90: "AY_FFFD_WRITE",
		0x91: "AY_BFFD",
		0xC3: "ALL_MODE",
		0x00: "",
		0x18: "",
		0x8C: "",
	} {
		if got := Name(code); got != want {
			t.Errorf("Name(%02X) = %q, want %q", code, got, want)
		}
	}
	if !strings.Contains(Description(0xC3), "#204E") {
		t.Error("ALL_MODE must retain the source address comment")
	}
	if !strings.Contains(Description(0x90), "??????") {
		t.Error("source uncertainty must be preserved")
	}
	if Description(0x18) != "" {
		t.Error("reserved source comment is not a named port definition")
	}
}

func TestCatalogMatchesSourceDefinitions(t *testing.T) {
	source, err := os.ReadFile("../../Example/внутренние_порты.txt")
	if err != nil {
		t.Fatal(err)
	}
	definition := regexp.MustCompile(`^\s*([.A-Za-z_][.A-Za-z0-9_]*):?\s+EQU\s+#([A-Fa-f0-9]{2})(?:\s*(?:;(.*))?)?$`)
	label := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):\s*(?:;.*)?$`)
	scope, count := "", 0
	for i, raw := range strings.Split(string(source), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if m := label.FindStringSubmatch(line); m != nil {
			scope = m[1]
		}
		m := definition.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		code, err := strconv.ParseUint(m[2], 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		name := m[1]
		if strings.HasPrefix(name, ".") {
			name = scope + name
		}
		want := entry{byte(code), name, strings.TrimSpace(m[3]), i + 1}
		if got := byCode[code]; got != want {
			t.Errorf("source line %d: got %#v, want %#v", i+1, got, want)
		}
		count++
	}
	if count != len(Codes()) {
		t.Errorf("%d named definitions in source, %d catalog entries", count, len(Codes()))
	}
	var provenance struct {
		Source string `json:"source"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(reference, &provenance); err != nil {
		t.Fatal(err)
	}
	if provenance.Source != "Example/внутренние_порты.txt" {
		t.Fatal("missing reference provenance")
	}
	if got := fmt.Sprintf("%X", sha256.Sum256(source)); got != provenance.SHA256 {
		t.Errorf("source hash %s does not match catalog provenance %s", got, provenance.SHA256)
	}
}

func TestCodesSortedAndIndependent(t *testing.T) {
	first := Codes()
	if len(first) == 0 {
		t.Fatal("empty catalog")
	}
	for i := 1; i < len(first); i++ {
		if first[i] <= first[i-1] {
			t.Fatal("codes must be unique and sorted")
		}
	}
	want := first[0]
	first[0] = 0
	if Codes()[0] != want {
		t.Fatal("caller mutated catalog")
	}
}
