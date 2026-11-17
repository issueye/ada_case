package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func makeTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, text string) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package main\n// Needle\nfunc main() {}\n")
	write("b.md", "# needle\n")
	write(".git/HEAD", "needle")
	write("bin.dat", "nee\x00dle")
	write("sub/c.go", "package sub\n// needle here\n")
	return dir
}

func TestSearchCaseInsensitive(t *testing.T) {
	res, err := Run(context.Background(), makeTree(t), "needle", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 3 {
		t.Fatalf("matches = %d, want 3: %+v", len(res.Matches), res.Matches)
	}
	if res.Matches[0].Path != "a.go" || res.Matches[0].Line != 2 || res.Matches[0].Col != 4 {
		t.Errorf("first match = %+v", res.Matches[0])
	}
	if res.Files != 3 {
		t.Errorf("files searched = %d, want 3", res.Files)
	}
	for _, m := range res.Matches {
		if m.Path == ".git/HEAD" || m.Path == "bin.dat" {
			t.Errorf("searched an ignored or binary file: %s", m.Path)
		}
	}
}

func TestSearchCaseSensitive(t *testing.T) {
	res, err := Run(context.Background(), makeTree(t), "Needle", Options{CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 || res.Matches[0].Path != "a.go" {
		t.Errorf("matches = %+v", res.Matches)
	}
}

func TestSearchRegex(t *testing.T) {
	res, err := Run(context.Background(), makeTree(t), `n[eE]+dle`, Options{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 3 {
		t.Errorf("matches = %d", len(res.Matches))
	}
}

func TestSearchMaxResults(t *testing.T) {
	res, err := Run(context.Background(), makeTree(t), "needle", Options{MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 1 || !res.Truncated {
		t.Errorf("matches %d truncated %v", len(res.Matches), res.Truncated)
	}
}

func TestSearchCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, makeTree(t), "needle", Options{})
	if err == nil {
		t.Fatalf("a canceled search returned no error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v", err)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	res, err := Run(context.Background(), makeTree(t), "", Options{})
	if err != nil || len(res.Matches) != 0 {
		t.Errorf("empty query: %+v, %v", res, err)
	}
}
