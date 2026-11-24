package service

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// OutsidePath is a path the @ search found under a directory the operator
// named beyond the checkout: Shown, as the list shows it, with ~ for the home
// directory and a slash after a directory, and Full, the absolute path.
type OutsidePath struct {
	Shown string
	Full  string
}

// OutsideFound is what a search of the named directories found: the paths,
// at most PathsLimit, and Stopped, each directory, as the list shows it,
// whose listing stopped at its bound, so a path missing from the list may
// only be unseen.
type OutsideFound struct {
	Paths   []OutsidePath
	Stopped []string
}

// outsideSearch lists the directories the operator named for the @ search
// (config path_search.directories), each briefly held as a checkout's
// listing is.
type outsideSearch struct {
	dirs []string
	// bound is how many entries one directory's listing holds, each named
	// directory its own; zero is pathsListed.
	bound int

	mu      sync.Mutex
	at      time.Time
	listing *outsideListing
}

// outsideListing is the named directories' entries, shallowest first in
// each, and the directories whose walk stopped at its bound.
type outsideListing struct {
	entries []outsideEntry
	stopped []string
}

// outsideEntry is one entry of a named directory: its absolute path under
// the directory as named, a slash after a directory, how many levels below
// the named directory it lies (its own entries are 1), and when it last
// changed.
type outsideEntry struct {
	full     string
	depth    int
	modified time.Time
}

func (o *outsideSearch) limit() int {
	if o.bound > 0 {
		return o.bound
	}
	return pathsListed
}

// searches reports whether an @ word starting with q searches the named
// directories rather than the checkout: it starts with ~/ or /, and at least
// one directory is named. with none, the search is the checkout's, as it
// always was.
func (o *outsideSearch) searches(q string) bool {
	return len(o.dirs) > 0 && (q == "~" || strings.HasPrefix(q, "~/") || strings.HasPrefix(q, "/"))
}

// find searches the named directories for q. where what is typed up to its
// last slash names a directory inside a named one, that directory is looked
// in directly (look); otherwise q filters the listing of every named
// directory (filter). nothing outside a named directory is listed either
// way.
func (o *outsideSearch) find(q string, now time.Time, limit int) OutsideFound {
	home, _ := os.UserHomeDir()
	if found, ok := o.look(q, home, limit); ok {
		return found
	}
	return o.filter(q, home, now, limit)
}

// look lists the directory q names up to its last slash, when that
// directory, resolved through every link, is still inside a named
// directory: its own entries, those whose names match what follows the
// slash, files newest first and then directories. each is shown under the
// named directory as configured. q naming anything else, or leaving every
// named directory through a link or "..", is not looked in, and ok is false.
func (o *outsideSearch) look(q, home string, limit int) (OutsideFound, bool) {
	cut := strings.LastIndex(q, "/")
	if cut < 0 {
		return OutsideFound{}, false
	}
	typed, rest := q[:cut+1], q[cut+1:]
	if strings.HasPrefix(typed, "~/") {
		if home == "" {
			return OutsideFound{}, false
		}
		typed = home + typed[1:]
	}
	if !filepath.IsAbs(typed) {
		return OutsideFound{}, false
	}
	resolved, err := filepath.EvalSymlinks(typed)
	if err != nil {
		return OutsideFound{}, false
	}
	named, root, below, ok := o.inside(resolved)
	if !ok {
		return OutsideFound{}, false
	}
	dir := filepath.Join(named, below)
	entries, stopped, err := readDir(resolved, o.limit())
	if err != nil {
		return OutsideFound{}, false
	}
	fragments := strings.Fields(strings.ToLower(rest))
	var matched []outsideEntry
	for _, d := range entries {
		name := d.Name()
		if d.IsDir() && strings.HasPrefix(name, ".") {
			continue
		}
		if ok, _ := matchPath(strings.ToLower(name), fragments); !ok {
			continue
		}
		if !offered(resolved, root, name) {
			continue
		}
		info, err := d.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(dir, name)
		if d.IsDir() {
			full += "/"
		}
		matched = append(matched, outsideEntry{full: full, depth: 1, modified: info.ModTime()})
	}
	found := OutsideFound{Paths: ranked(matched, home, limit)}
	if stopped {
		found.Stopped = []string{shownPath(home, strings.TrimSuffix(dir, "/")+"/")}
	}
	return found, true
}

// inside finds the named directory a resolved path lies in: the directory as
// configured, its resolved root, and the path below it. ok is false for a
// path outside every named directory.
func (o *outsideSearch) inside(resolved string) (named, root, below string, ok bool) {
	for _, dir := range o.dirs {
		dir = filepath.Clean(dir)
		root := resolvedRoot(dir)
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if rel == "." {
			rel = ""
		}
		return dir, root, rel, true
	}
	return "", "", "", false
}

// filter matches q against the listing of every named directory, fragment
// by fragment as the checkout's search matches: a query starting with ~
// against each path shown with ~, any other against the absolute path. the
// query only filters; it is never read as a path here.
func (o *outsideSearch) filter(q, home string, now time.Time, limit int) OutsideFound {
	listing := o.entries(now)
	fragments := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return r == '/' || r == ' ' })
	tilde := strings.HasPrefix(q, "~")
	var matched []outsideEntry
	for _, e := range listing.entries {
		against := e.full
		if tilde {
			against = shownPath(home, e.full)
		}
		if ok, _ := matchPath(strings.ToLower(against), fragments); ok {
			matched = append(matched, e)
		}
	}
	return OutsideFound{Paths: ranked(matched, home, limit), Stopped: listing.stopped}
}

// ranked orders found entries and keeps the first limit: files before
// directories, since adding a file makes its directory as new as the file
// and the file just made is nearly always the one wanted; within each, the
// shallower first, so a directory's own files come before anything deeper;
// then newest first, then by path.
func ranked(entries []outsideEntry, home string, limit int) []OutsidePath {
	slices.SortStableFunc(entries, func(a, b outsideEntry) int {
		if ad, bd := strings.HasSuffix(a.full, "/"), strings.HasSuffix(b.full, "/"); ad != bd {
			if ad {
				return 1
			}
			return -1
		}
		if a.depth != b.depth {
			return a.depth - b.depth
		}
		if c := b.modified.Compare(a.modified); c != 0 {
			return c
		}
		return strings.Compare(a.full, b.full)
	})
	out := make([]OutsidePath, 0, min(limit, len(entries)))
	for _, e := range entries[:min(limit, len(entries))] {
		out = append(out, OutsidePath{Shown: shownPath(home, e.full), Full: e.full})
	}
	return out
}

// entries is the named directories' listing, read again once it is older
// than pathsFresh.
func (o *outsideSearch) entries(now time.Time) *outsideListing {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.listing == nil || now.Sub(o.at) > pathsFresh {
		o.listing, o.at = listOutside(o.dirs, o.limit()), now
	}
	return o.listing
}

// listOutside lists each named directory, each with its own bound: the
// directory itself, then its entries level by level, so its own files are
// always listed before anything deeper and a deep tree cannot crowd them
// out. directories whose names start with a dot are skipped, no symbolic
// link is followed, and a link that resolves outside the directory it is
// in, or nowhere, is left out. names and times only; it reads no file's
// contents. a directory whose listing reached its bound is named in stopped.
func listOutside(dirs []string, bound int) *outsideListing {
	home, _ := os.UserHomeDir()
	out := &outsideListing{}
	for _, dir := range dirs {
		dir = filepath.Clean(dir)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		root := resolvedRoot(dir)
		out.entries = append(out.entries, outsideEntry{full: dir + "/", modified: info.ModTime()})
		listed := 0
		stopped := false
		level := []string{""}
		for depth := 1; len(level) > 0 && !stopped; depth++ {
			var next []string
			for _, below := range level {
				entries, short, err := readDir(filepath.Join(dir, below), bound-listed)
				if err != nil {
					continue
				}
				for _, d := range entries {
					rel := filepath.ToSlash(filepath.Join(below, d.Name()))
					if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
						continue
					}
					if !offered(dir, root, rel) {
						continue
					}
					info, err := d.Info()
					if err != nil {
						continue
					}
					full := filepath.Join(dir, rel)
					if d.IsDir() {
						full += "/"
						next = append(next, rel)
					}
					out.entries = append(out.entries, outsideEntry{full: full, depth: depth, modified: info.ModTime()})
					listed++
				}
				if short || listed >= bound {
					stopped = true
					break
				}
			}
			level = next
		}
		if stopped {
			out.stopped = append(out.stopped, shownPath(home, dir+"/"))
		}
	}
	return out
}

// readDir reads at most n entries of a directory, in the order the system
// gives them, and says whether it had more.
func readDir(dir string, n int) ([]os.DirEntry, bool, error) {
	if n <= 0 {
		return nil, true, nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	entries, err := f.ReadDir(n)
	if err != nil && err != io.EOF {
		return nil, false, err
	}
	if len(entries) < n {
		return entries, false, nil
	}
	more, err := f.ReadDir(1)
	return entries, err == nil && len(more) > 0, nil
}

// shownPath is an absolute path as the list shows it: with ~ for the home
// directory when it is under it.
func shownPath(home, full string) string {
	if home != "" && full == home {
		return "~"
	}
	if home != "" && (full == home+"/" || strings.HasPrefix(full, home+"/")) {
		return "~" + strings.TrimPrefix(full, home)
	}
	return full
}

// OutsidePaths searches the directories the operator named beyond the
// checkout, for an @ word starting with ~/ or /, at most PathsLimit.
// searched is false for any other query, or with no directory named, and the
// checkout's search (Paths) answers it. the directories are the operator's
// own, so they are the same in every project; the project must still exist.
func (s *Service) OutsidePaths(projectID, q string) (found OutsideFound, searched bool, err error) {
	if err := s.lockOpen(); err != nil {
		return OutsideFound{}, false, err
	}
	_, err = s.projectLocked(projectID)
	s.mu.Unlock()
	if err != nil {
		return OutsideFound{}, false, err
	}
	if !s.outside.searches(q) {
		return OutsideFound{}, false, nil
	}
	return s.outside.find(q, time.Now(), PathsLimit), true, nil
}
