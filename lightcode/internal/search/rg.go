package search

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"lightcode/internal/proc"
	"lightcode/internal/workspace"
)

// ripgrep is the command to use when one is on PATH: the app ships a Go
// searcher, and a machine with ripgrep gets it instead, for its speed on
// large folders.
func ripgrep() string {
	path, err := exec.LookPath("rg")
	if err != nil {
		return ""
	}
	return path
}

// runRipgrep searches with ripgrep, reporting whether it ran at all: a
// machine without it falls back to the Go walk.
func runRipgrep(ctx context.Context, root, query string, opts Options, onMatch func(Match)) (Result, bool, error) {
	rg := ripgrep()
	if rg == "" {
		return Result{}, false, nil
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 500
	}
	maxBytes := opts.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	args := []string{"--vimgrep", "--no-heading", "--color=never", "--no-messages",
		"--max-filesize", itoaBytes(maxBytes),
	}
	if opts.CaseSensitive {
		args = append(args, "--case-sensitive")
	} else {
		args = append(args, "--ignore-case")
	}
	if !opts.Regex {
		args = append(args, "--fixed-strings")
	}
	for _, dir := range []string{".git", ".hg", ".svn", "node_modules", "vendor",
		".venv", "venv", "dist", "build", "target", ".idea", "__pycache__", ".next"} {
		if workspace.ShouldSkipDir(dir) {
			args = append(args, "--glob", "!"+dir+"/**")
		}
	}
	args = append(args, "-e", query, ".")
	cmd := exec.CommandContext(ctx, rg, args...)
	proc.Hide(cmd)
	cmd.Dir = root
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, false, nil
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{}, false, nil
	}
	var res Result
	found := 0
	var paths = map[string]bool{}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		m, ok := parseVimgrep(line)
		if !ok {
			continue
		}
		if onMatch != nil {
			onMatch(m)
		}
		found++
		paths[m.Path] = true
		if found >= maxResults {
			res.Truncated = true
			break
		}
	}
	res.Files = len(paths)
	res.Elapsed = time.Since(start)
	err = cmd.Wait()
	if ctx.Err() != nil {
		return res, true, ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			// 1 is "no matches".
			return res, true, nil
		}
		return res, true, err
	}
	return res, true, nil
}

// parseVimgrep reads ripgrep's path:line:col:text line.
func parseVimgrep(line string) (Match, bool) {
	parts := strings.SplitN(line, ":", 4)
	if len(parts) != 4 {
		return Match{}, false
	}
	path := filepath.ToSlash(strings.TrimPrefix(parts[0], "./"))
	lineNo, err := strconv.Atoi(parts[1])
	if err != nil {
		return Match{}, false
	}
	col, err := strconv.Atoi(parts[2])
	if err != nil || col < 1 {
		return Match{}, false
	}
	text := parts[3]
	byteCol := col - 1
	if byteCol > len(text) {
		byteCol = len(text)
	}
	runeCol := utf8.RuneCountInString(text[:byteCol]) + 1
	return Match{Path: path, Line: lineNo, Col: runeCol, Text: text}, true
}

func itoaBytes(n int64) string {
	switch {
	case n >= 1<<20 && n%(1<<20) == 0:
		return strconv.FormatInt(n/(1<<20), 10) + "M"
	case n >= 1<<10 && n%(1<<10) == 0:
		return strconv.FormatInt(n/(1<<10), 10) + "K"
	default:
		return strconv.FormatInt(n, 10)
	}
}
