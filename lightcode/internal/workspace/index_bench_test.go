package workspace

import (
	"os"
	"testing"
	"time"
)

// TestIndexLargeTree measures opening a big folder: set LIGHTCODE_BENCH_DIR
// to a 30k-file tree and LIGHTCODE_BENCH=1 to run it.
func TestIndexLargeTree(t *testing.T) {
	if os.Getenv("LIGHTCODE_BENCH") == "" {
		t.Skip("set LIGHTCODE_BENCH=1 and LIGHTCODE_BENCH_DIR to run")
	}
	dir := os.Getenv("LIGHTCODE_BENCH_DIR")
	if dir == "" {
		t.Skip("set LIGHTCODE_BENCH_DIR")
	}
	start := time.Now()
	w, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("indexed %d files in %v", w.FileCount(), time.Since(start))
	if time.Since(start) > 400*time.Millisecond {
		t.Errorf("indexing took %v, over the 400 ms line", time.Since(start))
	}
}
