package symbols

import "testing"

func TestGoSymbols(t *testing.T) {
	path := "x.go"
	text := "package x\n\nconst Max = 10\n\nvar Count int\n\ntype User struct{}\n\nfunc (u User) Name() string { return \"\" }\n\nfunc main() {}\n"
	syms := File(path, text)
	var names []string
	for _, s := range syms {
		names = append(names, s.Kind+":"+s.Name)
	}
	want := []string{"常量:Max", "变量:Count", "类型:User", "方法:User.Name", "函数:main"}
	if len(syms) != len(want) {
		t.Fatalf("symbols = %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("symbol %d = %q, want %q", i, names[i], want[i])
		}
	}
	if syms[0].Line != 3 || syms[4].Line != 11 {
		t.Errorf("lines = %d, %d", syms[0].Line, syms[4].Line)
	}
}

func TestMarkdownSymbols(t *testing.T) {
	text := "# Title\n\n## Section\n\ntext\n\n### Deeper\n"
	syms := File("x.md", text)
	if len(syms) != 3 {
		t.Fatalf("symbols = %+v", syms)
	}
	if syms[0].Name != "Title" || syms[0].Depth != 0 ||
		syms[1].Name != "Section" || syms[1].Depth != 1 ||
		syms[2].Name != "Deeper" || syms[2].Depth != 2 {
		t.Errorf("symbols = %+v", syms)
	}
}

func TestGenericSymbols(t *testing.T) {
	text := "import os\nclass A:\n    def run(self):\n        pass\n"
	syms := File("x.py", text)
	if len(syms) != 2 {
		t.Fatalf("symbols = %+v", syms)
	}
	if syms[0].Name != "A" || syms[1].Name != "run" {
		t.Errorf("symbols = %+v", syms)
	}
}

func TestNoSymbols(t *testing.T) {
	if syms := File("x.txt", "plain text\n"); len(syms) != 0 {
		t.Errorf("symbols = %+v", syms)
	}
}
