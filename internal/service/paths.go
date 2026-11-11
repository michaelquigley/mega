package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/df/dl"
)

const (
	// PathsLimit is the most matches a path search returns.
	PathsLimit = 20
	// pathsFresh is how long a checkout's listing is reused.
	pathsFresh = 5 * time.Second
	// pathsListed bounds a listing, from git or from the walk.
	pathsListed = 50000
	// pathsTimeout bounds running git for a listing.
	pathsTimeout = 10 * time.Second
)

// pathListing is a checkout's paths, relative to it with forward slashes,
// directories ending in a slash.
type pathListing struct {
	at    time.Time
	paths []string
}

// pathCache holds each checkout's listing briefly, and lists a checkout once
// however many searches arrive while it is being listed.
type pathCache struct {
	mu      sync.Mutex
	entries map[string]*pathEntry
}

type pathEntry struct {
	mu      sync.Mutex
	listing *pathListing
}

func (c *pathCache) listing(checkout string, now time.Time) []string {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[string]*pathEntry{}
	}
	for key, entry := range c.entries {
		if key != checkout && entry.mu.TryLock() {
			if entry.listing != nil && now.Sub(entry.listing.at) > pathsFresh {
				delete(c.entries, key)
			}
			entry.mu.Unlock()
		}
	}
	entry, ok := c.entries[checkout]
	if !ok {
		entry = &pathEntry{}
		c.entries[checkout] = entry
	}
	c.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.listing == nil || now.Sub(entry.listing.at) > pathsFresh {
		entry.listing = &pathListing{at: now, paths: listPaths(checkout)}
	}
	return entry.listing.paths
}

// Paths searches a project's checkout for paths matching q, best first, at
// most PathsLimit. it lists names only: it reads no file's contents, changes
// nothing, and leaves out a symbolic link that resolves outside the checkout.
func (s *Service) Paths(projectID, q string) ([]string, error) {
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	listing := s.paths.listing(project.Checkout, time.Now())
	return matchPaths(project.Checkout, listing, q, PathsLimit), nil
}

// listPaths lists a checkout: what git tracks plus untracked files it does not
// ignore, or, where git cannot list it, a bounded walk. directories the files
// imply are added.
func listPaths(checkout string) []string {
	files, err := gitFiles(checkout)
	if err != nil {
		dl.Debugf("listing '%s' with git: %v; walking it instead", checkout, err)
		files = walkFiles(checkout)
	}
	seen := make(map[string]struct{}, len(files))
	out := make([]string, 0, len(files))
	add := func(p string) {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	for _, f := range files {
		for i := 0; i < len(f); i++ {
			if f[i] == '/' && i < len(f)-1 {
				add(f[:i+1])
			}
		}
		add(f)
	}
	return out
}

// gitFiles runs git ls-files in the checkout. git reports paths relative to
// the directory it runs in, below it only, and never descends a symbolic link.
func gitFiles(checkout string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pathsTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = checkout
	// a listing takes no optional locks, so it never refreshes the index.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var files []string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	scanner.Split(splitNUL)
	for scanner.Scan() {
		if len(files) == pathsListed {
			cancel()
			break
		}
		if name := scanner.Text(); name != "" {
			files = append(files, name)
		}
	}
	if err := cmd.Wait(); err != nil && len(files) < pathsListed {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return files, nil
}

func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// walkFiles lists a checkout git cannot: files and empty directories, skipping
// directories whose names start with a dot, stopping at pathsListed. the walk
// does not follow symbolic links.
func walkFiles(checkout string) []string {
	var files []string
	_ = filepath.WalkDir(checkout, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && path != checkout {
				return fs.SkipDir
			}
			return nil
		}
		if path == checkout {
			return nil
		}
		if len(files) == pathsListed {
			return fs.SkipAll
		}
		rel, relErr := filepath.Rel(checkout, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			files = append(files, rel+"/")
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files
}

// a path's place in the results: basename matches before directory matches,
// then shorter before longer, then by name.
type pathMatch struct {
	path     string
	basename bool
}

// matchPaths picks the paths matching q, best first, at most limit. q is split
// into fragments at slashes and spaces; each fragment must appear, ignoring
// case, after the one before it. a match is a basename match when its last
// fragment can fall within the path's last segment.
func matchPaths(checkout string, paths []string, q string, limit int) []string {
	fragments := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return r == '/' || r == ' ' })
	var matches []pathMatch
	for _, p := range paths {
		if ok, basename := matchPath(strings.ToLower(p), fragments); ok {
			matches = append(matches, pathMatch{path: p, basename: basename})
		}
	}
	slices.SortFunc(matches, func(a, b pathMatch) int {
		if a.basename != b.basename {
			if a.basename {
				return -1
			}
			return 1
		}
		if len(a.path) != len(b.path) {
			return len(a.path) - len(b.path)
		}
		return strings.Compare(a.path, b.path)
	})
	root := resolvedRoot(checkout)
	out := make([]string, 0, min(limit, len(matches)))
	for _, m := range matches {
		if len(out) == limit {
			break
		}
		if !insideCheckout(checkout, root, m.path) {
			continue
		}
		out = append(out, m.path)
	}
	return out
}

func matchPath(lower string, fragments []string) (bool, bool) {
	if len(fragments) == 0 {
		return true, true
	}
	at := 0
	for _, f := range fragments[:len(fragments)-1] {
		i := strings.Index(lower[at:], f)
		if i < 0 {
			return false, false
		}
		at += i + len(f)
	}
	last := fragments[len(fragments)-1]
	base := strings.LastIndex(strings.TrimSuffix(lower, "/"), "/") + 1
	if i := strings.Index(lower[max(at, base):], last); i >= 0 {
		return true, true
	}
	return strings.Contains(lower[at:], last), false
}

func resolvedRoot(checkout string) string {
	root, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return filepath.Clean(checkout)
	}
	return root
}

// insideCheckout is false for a path that is a symbolic link resolving outside
// the checkout, or resolving nowhere. git and the walk descend no link, so
// only a path's last segment can be one.
func insideCheckout(checkout, root, rel string) bool {
	full := filepath.Join(checkout, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
	info, err := os.Lstat(full)
	if err != nil {
		// gone since the listing, or deleted but still tracked: offered by name.
		return true
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return true
	}
	target, err := filepath.EvalSymlinks(full)
	if err != nil {
		return false
	}
	within, err := filepath.Rel(root, target)
	return err == nil && within != ".." && !strings.HasPrefix(within, ".."+string(filepath.Separator))
}
