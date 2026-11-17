package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testTree(t *testing.T) string {
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
	write("a.txt", "a")
	write("sub/b.go", "package sub")
	write("sub/deep/c.go", "package deep")
	write(".git/HEAD", "ref: refs/heads/main")
	return dir
}

func TestOpenSortsFoldersFirst(t *testing.T) {
	w, err := Open(testTree(t))
	if err != nil {
		t.Fatal(err)
	}
	top := w.Top()
	if len(top) != 2 {
		t.Fatalf("top has %d entries: %v", len(top), names(top))
	}
	if !top[0].IsDir || top[0].Name != "sub" || top[1].Name != "a.txt" {
		t.Errorf("top = %v", names(top))
	}
	if top[0].Loaded() {
		t.Errorf("a folder loaded before it opened")
	}
	sub := top[0].Children()
	if len(sub) != 2 || sub[0].Name != "deep" || sub[1].Name != "b.go" {
		t.Errorf("sub = %v", names(sub))
	}
	if !top[0].Loaded() {
		t.Errorf("folder not marked loaded after Children")
	}
}

func TestFilesSkipsGit(t *testing.T) {
	w, err := Open(testTree(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "sub/b.go", "sub/deep/c.go"}
	if got := w.Files(); !reflect.DeepEqual(got, want) {
		t.Errorf("Files = %v, want %v", got, want)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, name string
		ok        bool
	}{
		{"", "a.go", true},
		{"ag", "a.go", true},
		{"AG", "a.go", true},
		{"so", "src/main.go", true},
		{"foo bar", "foo/bar.go", true},
		{"xyz", "a.go", false},
	}
	for _, c := range cases {
		if _, ok := Match(c.pat, c.name); ok != c.ok {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pat, c.name, ok, c.ok)
		}
	}
	// A match at a word start scores higher than one in the middle.
	early, _ := Match("ma", "main.go")
	late, _ := Match("ma", "xmain.go")
	if early <= late {
		t.Errorf("scores %d <= %d", early, late)
	}
}

func names(es []*Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}
