package search

import (
	"context"
	"testing"
)

func TestParseVimgrep(t *testing.T) {
	cases := []struct {
		line string
		want Match
		ok   bool
	}{
		{"./d1/f1.go:3:6:func F42() {}", Match{Path: "d1/f1.go", Line: 3, Col: 6, Text: "func F42() {}"}, true},
		{"a b/c.go:12:4:\treturn x", Match{Path: "a b/c.go", Line: 12, Col: 4, Text: "\treturn x"}, true},
		{"no colons", Match{}, false},
		{"f.go:x:1:text", Match{}, false},
		{"f.go:1:0:text", Match{}, false},
	}
	for _, c := range cases {
		got, ok := parseVimgrep(c.line)
		if ok != c.ok {
			t.Errorf("%q: ok = %v, want %v", c.line, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("%q: got %+v, want %+v", c.line, got, c.want)
		}
	}
}

func TestParseVimgrepWideRunes(t *testing.T) {
	got, ok := parseVimgrep("f.go:1:6:func 中文() {}")
	if !ok {
		t.Fatal("not parsed")
	}
	// ripgrep counts bytes: byte 5 starts the wide character after "func ",
	// which is rune 6 in this app's 1-based columns.
	if got.Col != 6 {
		t.Errorf("col = %d, want 6", got.Col)
	}
}

// TestRipgrepMissing falls back to the Go walk on a machine without rg.
func TestRipgrepMissing(t *testing.T) {
	if ripgrep() != "" {
		t.Skip("ripgrep is installed; the fallback path is tested elsewhere")
	}
	res, ran, err := runRipgrep(context.Background(), t.TempDir(), "x", Options{}, nil)
	if err != nil || ran {
		t.Errorf("ran = %v, err = %v, want a clean fallback", ran, err)
	}
	_ = res
}

func TestItoaBytes(t *testing.T) {
	cases := map[int64]string{2048: "2K", 2 << 20: "2M", 500: "500"}
	for in, want := range cases {
		if got := itoaBytes(in); got != want {
			t.Errorf("itoaBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
