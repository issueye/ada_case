// Package git is the app's Git: the repository, its status, diffs, the
// index's text, and staging one hunk at a time through hash-object and
// update-index rather than a patch (the algorithm of docs/mayoi-reference.md
// §2.6).
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/proc"
)

// ErrNotRepo is returned by Open for a folder outside a repository.
var ErrNotRepo = errors.New("not a git repository")

// Repo is one repository.
type Repo struct {
	// Root is its working tree's top, absolute.
	Root string
}

// Open finds the repository root of dir.
func Open(dir string) (*Repo, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNotRepo
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return nil, ErrNotRepo
	}
	return &Repo{Root: filepath.Clean(root)}, nil
}

// run runs git in dir and returns its output.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	proc.Hide(cmd)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return string(out), errors.New(msg)
	}
	return string(out), nil
}

// Run runs git in the repository, with ctx.
func (r *Repo) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.Root}, args...)...)
	proc.Hide(cmd)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return string(out), errors.New(msg)
	}
	return string(out), nil
}

// Entry is one file git reports.
type Entry struct {
	// Path is relative to the root, with slashes; Orig is a rename's
	// name before.
	Path, Orig string
	// X is the index's status and Y the worktree's: 'M', 'A', 'D', 'R',
	// 'C', 'T' or '.'. Untracked entries have X '?'.
	X, Y byte
}

// Status is what git status says.
type Status struct {
	Branch, Upstream string
	Ahead, Behind    int
	// Staged are the changes in the index, Changes the ones in the
	// working tree not staged, Untracked the new files, Conflicts the
	// unmerged ones.
	Staged, Changes, Untracked, Conflicts []Entry
}

// Clean reports a repository with nothing to commit.
func (s *Status) Clean() bool {
	return len(s.Staged)+len(s.Changes)+len(s.Untracked)+len(s.Conflicts) == 0
}

// Count is how many files git reports.
func (s *Status) Count() int {
	return len(s.Staged) + len(s.Changes) + len(s.Untracked) + len(s.Conflicts)
}

// Status reads the working tree's status: porcelain v2, with renames and
// the branch's state.
func (r *Repo) Status(ctx context.Context) (*Status, error) {
	out, err := r.Run(ctx, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	st := parseStatus(out)
	return &st, nil
}

// parseStatus reads porcelain v2 with -z.
func parseStatus(out string) Status {
	var st Status
	recs := strings.Split(out, "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if rec == "" {
			continue
		}
		if strings.HasPrefix(rec, "# ") {
			parseStatusHeader(rec, &st)
			continue
		}
		switch rec[0] {
		case '1':
			fields := strings.SplitN(rec, " ", 9)
			if len(fields) < 9 {
				continue
			}
			addEntry(&st, Entry{Path: fields[8], X: fields[1][0], Y: fields[1][1]})
		case '2':
			fields := strings.SplitN(rec, " ", 10)
			if len(fields) < 10 {
				continue
			}
			e := Entry{Path: fields[9], X: fields[1][0], Y: fields[1][1]}
			if i+1 < len(recs) {
				e.Orig = recs[i+1]
				i++
			}
			addEntry(&st, e)
		case 'u':
			fields := strings.SplitN(rec, " ", 11)
			if len(fields) < 11 {
				continue
			}
			e := Entry{Path: fields[10], X: fields[1][0], Y: fields[1][1]}
			st.Conflicts = append(st.Conflicts, e)
		case '?':
			fields := strings.SplitN(rec, " ", 2)
			if len(fields) < 2 {
				continue
			}
			st.Untracked = append(st.Untracked, Entry{Path: fields[1], X: '?', Y: '?'})
		}
	}
	return st
}

func parseStatusHeader(rec string, st *Status) {
	fields := strings.Fields(rec)
	if len(fields) < 3 {
		return
	}
	switch fields[1] {
	case "branch.head":
		st.Branch = fields[2]
		if st.Branch == "(detached)" {
			st.Branch = "detached"
		}
	case "branch.upstream":
		st.Upstream = fields[2]
	case "branch.ab":
		for _, f := range fields[2:] {
			if strings.HasPrefix(f, "+") {
				st.Ahead, _ = strconv.Atoi(f[1:])
			}
			if strings.HasPrefix(f, "-") {
				st.Behind, _ = strconv.Atoi(f[1:])
			}
		}
	}
}

func addEntry(st *Status, e Entry) {
	if e.X != '.' {
		st.Staged = append(st.Staged, e)
	}
	if e.Y != '.' {
		st.Changes = append(st.Changes, e)
	}
}

// Diff returns the unified diff of one file, of the working tree against
// the index, or of the index against HEAD when cached.
func (r *Repo) Diff(ctx context.Context, path string, cached bool) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--unified=3",
		"--src-prefix=a/", "--dst-prefix=b/"}
	if cached {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	return r.Run(ctx, args...)
}

// IndexText is the file's text in the index, "" for one not there.
func (r *Repo) IndexText(ctx context.Context, path string) (string, error) {
	out, err := r.Run(ctx, "show", ":"+filepath.ToSlash(path))
	if err != nil {
		// New files are not in the index yet.
		return "", nil
	}
	return out, nil
}

// WorktreeText is the file's text on disk.
func (r *Repo) WorktreeText(path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(path)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FileMode is the file's mode in the index, 100644 for one not there.
func (r *Repo) FileMode(ctx context.Context, path string) string {
	out, err := r.Run(ctx, "ls-files", "-s", "--", path)
	if err != nil {
		return "100644"
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "100644"
	}
	return fields[0]
}

// HashObject writes text into the object database and returns its id.
func (r *Repo) HashObject(ctx context.Context, text string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", r.Root, "hash-object", "-w", "--stdin")
	proc.Hide(cmd)
	cmd.Stdin = strings.NewReader(text)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimSpace(string(out)), nil
}

// StageHunk stages the hunk of the file's working-tree diff at index h:
// the hunk is applied to the index's text, that text written as a blob,
// and the index pointed at it. No patch is applied, so no context can
// fuzz it.
func (r *Repo) StageHunk(ctx context.Context, path string, h int) error {
	patch, err := r.Diff(ctx, path, false)
	if err != nil {
		return err
	}
	files, err := editor.ParsePatch(patch)
	if err != nil {
		return err
	}
	fp, ok := findFile(files, path)
	if !ok {
		return fmt.Errorf("no diff for %s", path)
	}
	if h < 0 || h >= len(fp.Hunks) {
		return fmt.Errorf("hunk %d of %d", h, len(fp.Hunks))
	}
	indexText, err := r.IndexText(ctx, path)
	if err != nil {
		return err
	}
	one := editor.FilePatch{OldName: fp.OldName, NewName: fp.NewName, OldMode: fp.OldMode, NewMode: fp.NewMode,
		Hunks: []editor.PatchHunk{fp.Hunks[h]}}
	newText, err := one.Apply(indexText)
	if err != nil {
		return fmt.Errorf("apply hunk: %w", err)
	}
	hash, err := r.HashObject(ctx, newText)
	if err != nil {
		return err
	}
	mode := r.FileMode(ctx, path)
	if _, err := r.Run(ctx, "update-index", "--cacheinfo", mode+","+hash+","+filepath.ToSlash(path)); err != nil {
		return err
	}
	return nil
}

// findFile picks the patch of path from a diff.
func findFile(files []editor.FilePatch, path string) (editor.FilePatch, bool) {
	path = filepath.ToSlash(path)
	for _, f := range files {
		if f.NewName == path || f.OldName == path {
			return f, true
		}
	}
	return editor.FilePatch{}, false
}

// HunkDiff returns the diff of path with its hunks, for the diff page.
func (r *Repo) HunkDiff(ctx context.Context, path string, cached bool) (editor.FilePatch, string, error) {
	patch, err := r.Diff(ctx, path, cached)
	if err != nil {
		return editor.FilePatch{}, "", err
	}
	files, err := editor.ParsePatch(patch)
	if err != nil {
		return editor.FilePatch{}, "", err
	}
	fp, ok := findFile(files, path)
	if !ok {
		return editor.FilePatch{}, patch, nil
	}
	return fp, patch, nil
}

// HeadText is the file's text in HEAD, "" for one not there.
func (r *Repo) HeadText(ctx context.Context, path string) (string, error) {
	out, err := r.Run(ctx, "show", "HEAD:"+filepath.ToSlash(path))
	if err != nil {
		return "", nil
	}
	return out, nil
}

// StageFile stages the whole file.
func (r *Repo) StageFile(ctx context.Context, path string) error {
	_, err := r.Run(ctx, "add", "--", path)
	return err
}

// UnstageFile takes the file's changes out of the index.
func (r *Repo) UnstageFile(ctx context.Context, path string) error {
	_, err := r.Run(ctx, "restore", "--staged", "--", path)
	return err
}

// DiscardFile throws the working tree's changes of the file away.
func (r *Repo) DiscardFile(ctx context.Context, path string) error {
	_, err := r.Run(ctx, "restore", "--worktree", "--", path)
	return err
}

// Commit records the index with message.
func (r *Repo) Commit(ctx context.Context, message string) error {
	_, err := r.Run(ctx, "commit", "-m", message)
	return err
}

// LogEntry is one commit of git log.
type LogEntry struct {
	Hash    string
	Author  string
	When    string
	Subject string
}

// Log reads the last n commits.
func (r *Repo) Log(ctx context.Context, n int) ([]LogEntry, error) {
	if n <= 0 {
		n = 30
	}
	out, err := r.Run(ctx, "log", "--max-count="+strconv.Itoa(n),
		"--pretty=format:%h%x1f%an%x1f%ar%x1f%s%x1e")
	if err != nil {
		// A repository without commits yet.
		return nil, nil
	}
	var entries []LogEntry
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 4 {
			continue
		}
		entries = append(entries, LogEntry{Hash: parts[0], Author: parts[1], When: parts[2], Subject: parts[3]})
	}
	return entries, nil
}

// CurrentBranch is the branch's name, "" for a detached head.
func (r *Repo) CurrentBranch(ctx context.Context) string {
	out, err := r.Run(ctx, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
