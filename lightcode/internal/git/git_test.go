package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// repo makes a repository with one committed file and returns it.
func repo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-b", "main")
	gitCmd(t, dir, "config", "user.email", "test@example.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "a.txt")
	gitCmd(t, dir, "commit", "-m", "first")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOpen(t *testing.T) {
	r := repo(t)
	if r.Root == "" {
		t.Fatal("no root")
	}
	if _, err := Open(t.TempDir()); err != ErrNotRepo {
		t.Errorf("Open without a repo = %v, want ErrNotRepo", err)
	}
}

func TestStatus(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(r.Root, "a.txt"), []byte("one\nTWO\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Root, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" {
		t.Errorf("branch = %q", st.Branch)
	}
	if len(st.Changes) != 1 || st.Changes[0].Path != "a.txt" || st.Changes[0].Y != 'M' {
		t.Errorf("changes = %+v", st.Changes)
	}
	if len(st.Untracked) != 1 || st.Untracked[0].Path != "new.txt" {
		t.Errorf("untracked = %+v", st.Untracked)
	}
	if err := r.StageFile(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	st, err = r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 1 || st.Staged[0].X != 'M' {
		t.Errorf("staged = %+v", st.Staged)
	}
	if len(st.Changes) != 0 {
		t.Errorf("changes after add = %+v", st.Changes)
	}
	if err := r.UnstageFile(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if st, _ = r.Status(ctx); len(st.Staged) != 0 {
		t.Errorf("unstage left staged = %+v", st.Staged)
	}
}

func TestParseStatus(t *testing.T) {
	text := strings.Join([]string{
		"# branch.oid 1234",
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +2 -1",
		"1 M. N... 100644 100644 100644 aaa bbb a.txt",
		"? new.txt",
		"2 R. N... 100644 100644 100644 aaa bbb R100 new.txt",
		"old.txt",
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc conflict.txt",
		"",
	}, "\x00")
	st := parseStatus(text)
	if st.Branch != "main" || st.Upstream != "origin/main" || st.Ahead != 2 || st.Behind != 1 {
		t.Errorf("branch = %+v", st)
	}
	if len(st.Staged) != 2 || st.Staged[0].Path != "a.txt" || st.Staged[0].X != 'M' {
		t.Errorf("staged = %+v", st.Staged)
	}
	if len(st.Untracked) != 1 || st.Untracked[0].Path != "new.txt" {
		t.Errorf("untracked = %+v", st.Untracked)
	}
	if len(st.Staged) != 2 || st.Staged[1].Path != "new.txt" || st.Staged[1].Orig != "old.txt" {
		t.Errorf("rename = %+v", st.Staged)
	}
	if len(st.Conflicts) != 1 || st.Conflicts[0].Path != "conflict.txt" {
		t.Errorf("conflicts = %+v", st.Conflicts)
	}
	if st.Clean() || st.Count() != 4 {
		t.Errorf("clean/count = %v/%d", st.Clean(), st.Count())
	}
}

func TestStageHunk(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	var base strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&base, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(r.Root, "a.txt"), []byte(base.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, r.Root, "add", "a.txt")
	gitCmd(t, r.Root, "commit", "-m", "base")
	var sb strings.Builder
	for i := 1; i <= 30; i++ {
		if i == 3 || i == 25 {
			sb.WriteString("changed line\n")
		} else {
			fmt.Fprintf(&sb, "line %d\n", i)
		}
	}
	if err := os.WriteFile(filepath.Join(r.Root, "a.txt"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	fp, patch, err := r.HunkDiff(ctx, "a.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.Hunks) != 2 {
		t.Fatalf("hunks = %d, want 2; patch:\n%s", len(fp.Hunks), patch)
	}
	if err := r.StageHunk(ctx, "a.txt", 1); err != nil {
		t.Fatal(err)
	}
	indexText, err := r.IndexText(ctx, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(indexText, "\n")
	if lines[24] != "changed line" {
		t.Errorf("line 25 of the index = %q, want the staged change", lines[24])
	}
	if lines[2] == "changed line" {
		t.Errorf("the other hunk leaked into the index")
	}
	cached, err := r.Diff(ctx, "a.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	hunks := 0
	for _, line := range strings.Split(cached, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			hunks++
		}
	}
	if hunks != 1 {
		t.Errorf("cached diff has %d hunks, want 1:\n%s", hunks, cached)
	}
	st, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Staged) != 1 || len(st.Changes) != 1 {
		t.Errorf("status after staging = staged %+v changes %+v", st.Staged, st.Changes)
	}
}

func TestCommitAndLog(t *testing.T) {
	r := repo(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(r.Root, "a.txt"), []byte("one\ntwo\nTHREE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.StageFile(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	logs, err := r.Log(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Subject != "second" || logs[1].Subject != "first" {
		t.Errorf("log = %+v", logs)
	}
	if st, _ := r.Status(ctx); !st.Clean() {
		t.Errorf("clean after commit: %+v", st)
	}
}
