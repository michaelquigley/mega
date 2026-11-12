package service

import (
	"bytes"
	"fmt"
	"github.com/michaelquigley/mega/internal/harness"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// pathsCheckout is a git checkout with tracked, untracked and ignored files,
// a link inside it and links pointing outside it.
func pathsCheckout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "secret.txt", "outside")
	git(t, root, "init", "-q")
	write(t, root, ".gitignore", "build/\n*.log\n")
	write(t, root, "README.md", "readme")
	write(t, root, "src/main.go", "package main")
	write(t, root, "src/lib/util.go", "package lib")
	write(t, root, "util/readme.txt", "util")
	for _, link := range [][2]string{{filepath.Join(outside, "secret.txt"), "out-file.txt"}, {"README.md", "alias.md"}} {
		if err := os.Symlink(link[0], filepath.Join(root, link[1])); err != nil {
			t.Fatal(err)
		}
	}
	git(t, root, "add", ".")
	// after the index: untracked, ignored, and a link to an outside directory.
	write(t, root, "notes/new.txt", "new")
	write(t, root, "build/out.bin", "built")
	write(t, root, "debug.log", "log")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPathListingIsTrackedAndUntrackedNotIgnoredNorOutside(t *testing.T) {
	svc := tempService(t)
	root := pathsCheckout(t)
	project, err := svc.RegisterProject("paths", root)
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	all := svc.paths.listing(harness.Place{Cwd: root, Env: os.Environ()}, time.Now())
	for _, want := range []string{".gitignore", "README.md", "src/", "src/lib/", "src/main.go", "src/lib/util.go", "util/", "util/readme.txt", "notes/", "notes/new.txt", "alias.md"} {
		if !slices.Contains(all, want) {
			t.Errorf("listing lacks %q: %v", want, all)
		}
	}
	for _, never := range []string{"build/", "build/out.bin", "debug.log", "escape/", "escape/secret.txt", "secret.txt"} {
		if slices.Contains(all, never) {
			t.Errorf("listing has %q: %v", never, all)
		}
	}

	// links resolving outside are never offered; one resolving inside is.
	got, err := svc.Paths(project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, never := range []string{"escape", "out-file.txt"} {
		if slices.Contains(got, never) {
			t.Errorf("search offers %q, which leaves the checkout: %v", never, got)
		}
	}
	if !slices.Contains(got, "alias.md") {
		t.Errorf("search lacks the inside link alias.md: %v", got)
	}

	// listing changed nothing in the checkout.
	after, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(index, after) {
		t.Error("listing rewrote the git index")
	}
}

func TestPathMatching(t *testing.T) {
	svc := tempService(t)
	root := pathsCheckout(t)
	project, err := svc.RegisterProject("paths", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		q    string
		want []string
	}{
		// basename matches first, shorter first; then directory matches.
		{"util", []string{"util/", "src/lib/util.go", "util/readme.txt"}},
		// case-insensitive.
		{"README", []string{"README.md", "util/readme.txt"}},
		{"readme", []string{"README.md", "util/readme.txt"}},
		// fragments in order, at slashes and spaces.
		{"src/go", []string{"src/main.go", "src/lib/util.go"}},
		{"src lib", []string{"src/lib/", "src/lib/util.go"}},
		{"go/src", nil},
		{"nothing-like-this", nil},
	} {
		got, err := svc.Paths(project.ID, c.q)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("q %q = %v, want %v", c.q, got, c.want)
		}
	}
	if _, err := svc.Paths("no-such-project", "x"); ErrorCode(err) != CodeNotFound {
		t.Errorf("unknown project: %v", err)
	}
}

func TestPathSearchIsCapped(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	for i := range 30 {
		write(t, root, fmt.Sprintf("f%02d.txt", i), "")
	}
	svc := tempService(t)
	project, err := svc.RegisterProject("many", root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Paths(project.ID, "txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != PathsLimit || got[0] != "f00.txt" {
		t.Errorf("got %d: %v", len(got), got)
	}
}

func TestPathListingIsHeldBriefly(t *testing.T) {
	var cache pathCache
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, root, "first.txt", "")
	t0 := time.Now()
	if got := cache.listing(harness.Place{Cwd: root, Env: os.Environ()}, t0); !slices.Equal(got, []string{"first.txt"}) {
		t.Fatalf("first listing = %v", got)
	}
	write(t, root, "second.txt", "")
	if got := cache.listing(harness.Place{Cwd: root, Env: os.Environ()}, t0.Add(time.Second)); !slices.Equal(got, []string{"first.txt"}) {
		t.Errorf("within the hold = %v", got)
	}
	if got := cache.listing(harness.Place{Cwd: root, Env: os.Environ()}, t0.Add(pathsFresh+time.Second)); !slices.Contains(got, "second.txt") {
		t.Errorf("after the hold = %v", got)
	}
}

// a checkout outside any git repository is walked: dot-directories skipped,
// links not followed, outside links not offered.
func TestPathListingWalksANonGitCheckout(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "secret.txt", "outside")
	write(t, root, "a/b.txt", "")
	write(t, root, ".hidden/c.txt", "")
	write(t, root, ".env", "")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	// GIT_CEILING_DIRECTORIES keeps git from finding a repository above the
	// temporary directory.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	all := listPaths(harness.Place{Cwd: root, Env: os.Environ()})
	slices.Sort(all)
	if want := []string{".env", "a/", "a/b.txt", "empty/", "escape"}; !slices.Equal(all, want) {
		t.Errorf("walk = %v, want %v", all, want)
	}
	if got := matchPaths(root, all, "", PathsLimit); slices.Contains(got, "escape") {
		t.Errorf("search offers the outside link: %v", got)
	}
}

// a file git still tracks but that was deleted from the working tree is not
// offered, nor is a directory only such files imply.
func TestDeletedTrackedFilesAreNotOffered(t *testing.T) {
	svc := tempService(t)
	root := pathsCheckout(t)
	write(t, root, "gone/old.txt", "old")
	git(t, root, "add", "gone/old.txt")
	if err := os.RemoveAll(filepath.Join(root, "gone")); err != nil {
		t.Fatal(err)
	}
	project, err := svc.RegisterProject("paths", root)
	if err != nil {
		t.Fatal(err)
	}
	if all := svc.paths.listing(harness.Place{Cwd: root, Env: os.Environ()}, time.Now()); !slices.Contains(all, "gone/old.txt") {
		t.Fatalf("git no longer lists the deleted file, so this test proves nothing: %v", all)
	}
	for _, q := range []string{"old", "gone", ""} {
		got, err := svc.Paths(project.ID, q)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(got, "gone/old.txt") || slices.Contains(got, "gone/") {
			t.Errorf("q %q offers the deleted file: %v", q, got)
		}
	}
}

// with nothing typed after the @, hidden entries come last; typed, they rank
// as any other.
func TestEmptyQueryRanksHiddenEntriesLast(t *testing.T) {
	svc := tempService(t)
	root := pathsCheckout(t)
	write(t, root, ".claude/settings.json", "{}")
	project, err := svc.RegisterProject("paths", root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Paths(project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	firstHidden := slices.IndexFunc(got, hidden)
	if firstHidden < 0 || slices.ContainsFunc(got[firstHidden:], func(p string) bool { return !hidden(p) }) {
		t.Errorf("hidden entries are not last: %v", got)
	}
	if got[0] != "src/" && got[0] != "util/" {
		t.Errorf("the shortest visible path leads: %v", got)
	}
	typed, err := svc.Paths(project.ID, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(typed) == 0 || typed[0] != ".claude/" {
		t.Errorf("typed, a hidden match ranks as any other: %v", typed)
	}
}
