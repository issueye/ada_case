// Package search finds text in the workspace's files: a literal or a
// regular expression, case-insensitively by default, walking the folder
// with the explorer's ignores. It reads files on several goroutines and
// reports each match as it comes, so the first result shows before the
// walk ends. It is pure Go, so a machine without ripgrep searches the same.
package search

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"lightcode/internal/workspace"
)

// Options narrow a search.
type Options struct {
	// Regex reads Query as a Go regular expression instead of text.
	Regex bool
	// CaseSensitive matches case.
	CaseSensitive bool
	// MaxResults stops the search after this many matches; 500 when 0.
	MaxResults int
	// MaxFileBytes skips files larger than this; 2 MB when 0.
	MaxFileBytes int64
	// Files are paths relative to the root to search, as the workspace
	// index lists them; nil walks the folder instead.
	Files []string
	// NoRipgrep keeps the search on the Go walk even where ripgrep is
	// installed, for tests of that path.
	NoRipgrep bool
}

// Match is one line that matched.
type Match struct {
	// Path is relative to the workspace root, with slashes.
	Path string
	// Line is 1-based; Col is the 1-based rune of the first match.
	Line int
	Col  int
	// Text is the line as it is.
	Text string
}

// Result is what one search found.
type Result struct {
	Matches []Match
	// Files is how many files were read, Truncated that the match limit
	// stopped the search.
	Files     int
	Truncated bool
	Elapsed   time.Duration
}

// Run searches root and returns every match once the walk is done.
func Run(ctx context.Context, root, query string, opts Options) (Result, error) {
	var matches []Match
	res, err := RunStream(ctx, root, query, opts, func(m Match) { matches = append(matches, m) })
	if err == nil {
		sortMatches(matches)
	}
	res.Matches = matches
	return res, err
}

// RunStream searches root and calls onMatch for each match as it finds
// one; onMatch runs on one of the reader goroutines, never at the same
// time as another call. Cancel ctx to stop; the result then holds what was
// found so far.
func RunStream(ctx context.Context, root, query string, opts Options, onMatch func(Match)) (Result, error) {
	start := time.Now()
	var res Result
	if query == "" {
		res.Elapsed = time.Since(start)
		return res, nil
	}
	pattern := query
	if !opts.Regex {
		pattern = regexp.QuoteMeta(query)
	}
	if !opts.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		res.Elapsed = time.Since(start)
		return res, err
	}
	if !opts.NoRipgrep {
		if res, ran, err := runRipgrep(ctx, root, query, opts, onMatch); ran {
			return res, err
		}
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 500
	}
	maxBytes := opts.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	files := opts.Files
	if files == nil {
		files = walkFiles(root)
	}
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	var mu sync.Mutex
	found := 0
	var truncated bool
	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	scan := func(rel string) {
		if scanCtx.Err() != nil {
			return
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil || info.Size() > maxBytes {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil || isBinary(data) {
			return
		}
		mu.Lock()
		res.Files++
		mu.Unlock()
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if i%64 == 63 {
				select {
				case <-scanCtx.Done():
					return
				default:
				}
			}
			loc := re.FindStringIndex(line)
			if loc == nil {
				continue
			}
			col := utf8.RuneCountInString(line[:loc[0]]) + 1
			mu.Lock()
			if found >= maxResults {
				truncated = true
				mu.Unlock()
				cancel()
				return
			}
			found++
			if onMatch != nil {
				onMatch(Match{Path: rel, Line: i + 1, Col: col, Text: line})
			}
			hitLimit := found >= maxResults
			if hitLimit {
				truncated = true
			}
			mu.Unlock()
			if hitLimit {
				cancel()
				return
			}
		}
	}

	var wg sync.WaitGroup
	ch := make(chan string)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range ch {
				scan(rel)
			}
		}()
	}
feed:
	for _, rel := range files {
		select {
		case ch <- rel:
		case <-scanCtx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()

	res.Truncated = truncated
	res.Elapsed = time.Since(start)
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	return res, nil
}

func sortMatches(matches []Match) {
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].Line < matches[j].Line
	})
}

// walkFiles lists the folder's files for a search without an index.
func walkFiles(root string) []string {
	var out []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && workspace.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out
}

func isBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
