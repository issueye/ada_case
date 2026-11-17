package search

import (
	"context"
	"os"
	"testing"
	"time"

	"lightcode/internal/workspace"
)

// TestSearchLargeTree measures a search in a big folder: set
// LIGHTCODE_BENCH_DIR to a 30k-file tree and LIGHTCODE_BENCH=1 to run it.
func TestSearchLargeTree(t *testing.T) {
	if os.Getenv("LIGHTCODE_BENCH") == "" {
		t.Skip("set LIGHTCODE_BENCH=1 and LIGHTCODE_BENCH_DIR to run")
	}
	dir := os.Getenv("LIGHTCODE_BENCH_DIR")
	if dir == "" {
		t.Skip("set LIGHTCODE_BENCH_DIR")
	}
	w, err := workspace.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var first time.Duration
	matches := 0
	res, err := RunStream(context.Background(), dir, "func F42(", Options{Files: w.Files()}, func(Match) {
		matches++
		if first == 0 {
			first = time.Since(start)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first match %v, %d matches, %d files read, total %v", first, matches, res.Files, res.Elapsed)
	if first > 800*time.Millisecond {
		t.Errorf("first match took %v, over the 800 ms line", first)
	}
}
