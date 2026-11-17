package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// testGitWorkspace makes a repository with a committed README and a Go
// file, then changes two far apart lines of the Go file and adds an
// untracked one, and opens the app on it.
func testGitWorkspace(t *testing.T) (*App, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	base := strings.Builder{}
	base.WriteString("package main\n\n")
	for i := 1; i <= 18; i++ {
		fmt.Fprintf(&base, "// line %d\n", i)
	}
	base.WriteString("func main() {}\n")
	for name, text := range map[string]string{
		"README.md": "# Demo\n",
		"main.go":   base.String(),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "add", "README.md", "main.go")
	gitRun(t, dir, "commit", "-m", "first")
	changed := strings.Builder{}
	changed.WriteString("package main\n\n")
	for i := 1; i <= 18; i++ {
		if i == 2 || i == 17 {
			fmt.Fprintf(&changed, "// changed %d\n", i)
		} else {
			fmt.Fprintf(&changed, "// line %d\n", i)
		}
	}
	changed.WriteString("func main() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(changed.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIGHTCODE_STATE", filepath.Join(dir, "state.json"))
	return New(dir, ""), dir
}

func TestSCMPanel(t *testing.T) {
	a, _ := testGitWorkspace(t)
	if a.Repo == nil || a.Git == nil {
		t.Fatalf("repository = %v, status = %v", a.Repo, a.Git)
	}
	if a.Git.Branch != "main" {
		t.Errorf("branch = %q", a.Git.Branch)
	}
	if len(a.Git.Changes) != 1 || len(a.Git.Untracked) != 1 {
		t.Fatalf("status = changes %+v untracked %+v", a.Git.Changes, a.Git.Untracked)
	}
	a.ShowSidebar(2)
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"源代码管理", "更改", "未跟踪", "main.go", "new.txt", "提交"} {
		if !tt.HasText(want) {
			t.Errorf("SCM panel misses %q; texts %q", want, tt.Texts())
		}
	}
	if !tt.HasText("main") {
		t.Errorf("branch name missing")
	}
}

func TestDiffPageAndStageHunk(t *testing.T) {
	a, _ := testGitWorkspace(t)
	a.ShowSidebar(2)
	a.openDiff("main.go", false)
	if a.Diff == nil || len(a.Diff.Hunks) != 2 {
		t.Fatalf("diff = %+v, hunks = %d", a.Diff, hunkCount(a.Diff))
	}
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("块 1") || !tt.HasText("暂存全部") {
		t.Fatalf("diff page misses its bars; texts %q", tt.Texts())
	}
	a.stageHunk("main.go", 0)
	if a.Git == nil || len(a.Git.Staged) != 1 || len(a.Git.Changes) != 1 {
		t.Fatalf("after staging: staged %+v changes %+v", a.Git.Staged, a.Git.Changes)
	}
	// Reopening shows one hunk left.
	a.openDiff("main.go", false)
	if len(a.Diff.Hunks) != 1 {
		t.Errorf("hunks left = %d, want 1", len(a.Diff.Hunks))
	}
}

func hunkCount(d *diffTab) int {
	if d == nil {
		return 0
	}
	return len(d.Hunks)
}

func TestCommitThroughPanel(t *testing.T) {
	a, _ := testGitWorkspace(t)
	a.ShowSidebar(2)
	a.stageFile("main.go")
	a.GitMsg = "stage the change"
	a.commit()
	if a.GitErr != "" {
		t.Fatalf("commit error: %s", a.GitErr)
	}
	if a.Git == nil || len(a.Git.Staged) != 0 {
		t.Errorf("staged after commit = %+v", a.Git)
	}
	if len(a.GitLog) == 0 || a.GitLog[0].Subject != "stage the change" {
		t.Errorf("log = %+v", a.GitLog)
	}
	if a.GitMsg != "" {
		t.Errorf("the message was not cleared")
	}
}

func TestGitPagesCloseAndUnstage(t *testing.T) {
	a, _ := testGitWorkspace(t)
	a.openDiff("main.go", false)
	a.Diff = nil
	a.stageFile("main.go")
	a.openDiff("main.go", true)
	if a.Diff == nil || !a.Diff.Staged {
		t.Fatalf("staged diff page not opened: %+v", a.Diff)
	}
	a.unstageFile("main.go")
	if a.Git == nil || len(a.Git.Staged) != 0 {
		t.Errorf("unstage left staged = %+v", a.Git.Staged)
	}
}
