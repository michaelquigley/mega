package service

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// foldersListed bounds one folder's listing in the register dialog's picker.
const foldersListed = 2000

// Folders is one folder as the register dialog's picker shows it: the
// folders in it, by name, each marked a git checkout or already registered.
// Path is the folder as asked for, with ~ for the home directory; Full is
// its absolute path; Parent the folder above it, empty at the projects root,
// which the picker never goes above; Root the projects root, shown the same
// way. Stopped says the listing stopped at its bound. Note says why nothing
// is listed, for a path the picker does not look in: outside the root, not
// absolute, missing or unreadable. those are ordinary while a path is being
// typed, so they are answers, not errors.
type Folders struct {
	Path    string
	Full    string
	Parent  string
	Root    string
	Entries []Folder
	Stopped bool
	Note    string
}

// Folder is one folder in a listing: its name, whether it is a git checkout,
// and the name of the project that already registered it, if one has.
type Folder struct {
	Name       string
	Git        bool
	Registered string
}

// Folders lists the folders in path, for the register dialog's picker: an
// empty path is the projects root. names only, folders only, and only inside
// the projects root (config projects_root, else the home directory): a path
// that resolves, links and all, outside it is refused, a folder whose name
// starts with a dot is left out, and a link is listed only where it leads to
// a folder still inside the root. it reads no file's contents and changes
// nothing. a path typed in full is still registered wherever it is; only
// what the picker lists is confined.
func (s *Service) Folders(path string) (Folders, error) {
	if err := s.lockOpen(); err != nil {
		return Folders{}, err
	}
	s.mu.Unlock()
	home, _ := os.UserHomeDir()
	root := s.projectsRoot
	if root == "" {
		root = home
	}
	if root == "" {
		return Folders{}, newError(CodeUnavailable, "the projects root cannot be found: no home directory and no projects_root")
	}
	root = filepath.Clean(root)
	shownRoot := shownPath(home, filepath.Clean(root))
	unlisted := func(shown, note string) (Folders, error) {
		return Folders{Path: shown, Root: shownRoot, Entries: []Folder{}, Note: note}, nil
	}
	full := strings.TrimSpace(path)
	switch {
	case full == "":
		full = root
	case full == "~" || strings.HasPrefix(full, "~/"):
		full = filepath.Join(home, strings.TrimPrefix(full, "~"))
	}
	if !filepath.IsAbs(full) {
		return unlisted(path, fmt.Sprintf("'%s' is not an absolute path; start it with / or ~/", path))
	}
	full = filepath.Clean(full)
	realRoot := resolvedRoot(root)
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return unlisted(shownPath(home, full), fmt.Sprintf("'%s' does not exist", shownPath(home, full)))
		}
		return unlisted(shownPath(home, full), fmt.Sprintf("'%s' cannot be read: %v", shownPath(home, full), err))
	}
	if !within(realRoot, resolved) {
		return unlisted(shownPath(home, full), fmt.Sprintf("'%s' is outside the projects root, '%s', so nothing there is listed; a path typed in full is still registered", shownPath(home, full), shownRoot))
	}
	dir, err := os.Open(resolved)
	if err != nil {
		return unlisted(shownPath(home, full), fmt.Sprintf("'%s' cannot be listed: %v", shownPath(home, full), err))
	}
	defer dir.Close()
	s.mu.Lock()
	registered := map[string]string{}
	for _, project := range s.reg.Snapshot().Projects {
		registered[filepath.Clean(project.Checkout)] = project.Name
		if real, err := filepath.EvalSymlinks(project.Checkout); err == nil {
			registered[real] = project.Name
		}
	}
	s.mu.Unlock()
	out := Folders{Path: shownPath(home, full), Full: full, Root: shownPath(home, root), Entries: []Folder{}}
	// the way up stops at the root: a parent is offered only while it, too,
	// resolves inside it.
	if full != root && resolved != realRoot {
		if up, err := filepath.EvalSymlinks(filepath.Dir(full)); err == nil && within(realRoot, up) {
			out.Parent = shownPath(home, filepath.Dir(full))
		}
	}
	// read a chunk at a time, keeping folders, until the folders listed reach
	// their bound or the entries read reach pathsListed, files included.
	read := 0
	for !out.Stopped {
		chunk, err := dir.ReadDir(256)
		if len(chunk) == 0 || (err != nil && !errors.Is(err, io.EOF)) {
			break
		}
		read += len(chunk)
		for _, d := range chunk {
			if len(out.Entries) == foldersListed {
				out.Stopped = true
				break
			}
			if folder, ok := folderOf(d, resolved, full, realRoot, registered); ok {
				out.Entries = append(out.Entries, folder)
			}
		}
		if read >= pathsListed {
			out.Stopped = true
		}
	}
	slices.SortFunc(out.Entries, func(a, b Folder) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// folderOf is one directory entry as the picker lists it, or false for one
// it leaves out: not a folder, a dot-folder, or a link leading out of the
// root or nowhere. dir is the folder being listed, resolved; full is it as
// asked for.
func folderOf(d fs.DirEntry, dir, full, realRoot string, registered map[string]string) (Folder, bool) {
	name := d.Name()
	if strings.HasPrefix(name, ".") {
		return Folder{}, false
	}
	real := filepath.Join(dir, name)
	if d.Type()&fs.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(real)
		if err != nil || !within(realRoot, target) {
			return Folder{}, false
		}
		real = target
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return Folder{}, false
	}
	folder := Folder{Name: name, Registered: registered[filepath.Join(full, name)]}
	if folder.Registered == "" {
		folder.Registered = registered[real]
	}
	if _, err := os.Lstat(filepath.Join(real, ".git")); err == nil {
		folder.Git = true
	}
	return folder, true
}

// within is whether path is root or below it, both resolved.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
