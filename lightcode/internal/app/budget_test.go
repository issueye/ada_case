package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestFrameBudget keeps the main window cheap to draw: it opens a
// 2,000-line file and times 20 frames of the view, as docs/dev-plan.md §6
// asks (idle 2 ms, scroll 5 ms, typing 4 ms; this bound leaves room for
// test machines).
func TestFrameBudget(t *testing.T) {
	a, dir := testWorkspace(t)
	big := strings.Repeat("func f() {}\n", 2000)
	if err := writeFileAt(dir, "big.go", big); err != nil {
		t.Fatal(err)
	}
	a.OpenFile(filepath.Join(dir, "big.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	start := time.Now()
	for range 20 {
		tt.Frame()
	}
	per := time.Since(start) / 20
	t.Logf("frame: %v", per)
	if per > 8*time.Millisecond {
		t.Errorf("frames take %v, over the 8 ms budget", per)
	}
}
