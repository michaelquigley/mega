package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, trusted ...string) string {
	t.Helper()
	body := `{"numStartups": 3, "projects": {`
	for i, dir := range trusted {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`%q: {"allowedTools": [], "hasTrustDialogAccepted": true}`, dir)
	}
	body += `}}`
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// checkoutBase returns a fresh temporary directory for test checkouts, failing
// first if any directory above it holds a .git. the trust lookup takes the
// nearest .git as the git root, so a stray one (an empty /tmp/.git appeared once)
// silently turns every checkout below it into part of that repository.
func checkoutBase(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	for dir := filepath.Dir(base); ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			t.Fatalf("'%s' exists above the test's temporary directory, so every test checkout resolves to it as a git root; remove it or point TMPDIR elsewhere", filepath.Join(dir, ".git"))
		}
		if parent := filepath.Dir(dir); parent == dir {
			return base
		}
	}
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestExactPathTrusted(t *testing.T) {
	checkout := mkdir(t, checkoutBase(t), "plain")
	trusted, err := CheckoutTrusted(writeConfig(t, checkout), checkout)
	if err != nil || !trusted {
		t.Errorf("trusted = %v, err = %v", trusted, err)
	}
}

func TestUnlistedPathIsUntrusted(t *testing.T) {
	checkout := mkdir(t, checkoutBase(t), "plain")
	trusted, err := CheckoutTrusted(writeConfig(t, "/some/other/place"), checkout)
	if err != nil || trusted {
		t.Errorf("trusted = %v, err = %v", trusted, err)
	}
}

func TestListedWithoutAcceptanceIsUntrusted(t *testing.T) {
	checkout := mkdir(t, checkoutBase(t), "plain")
	path := filepath.Join(t.TempDir(), ".claude.json")
	body := fmt.Sprintf(`{"projects": {%q: {"hasTrustDialogAccepted": false}}}`, checkout)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	trusted, err := CheckoutTrusted(path, checkout)
	if err != nil || trusted {
		t.Errorf("trusted = %v, err = %v", trusted, err)
	}
}

func TestSubdirectoryOfTrustedGitRoot(t *testing.T) {
	root := mkdir(t, checkoutBase(t), "repo")
	mkdir(t, root, ".git")
	sub := mkdir(t, root, "pkg", "deep")
	trusted, err := CheckoutTrusted(writeConfig(t, root), sub)
	if err != nil || !trusted {
		t.Errorf("trusted = %v, err = %v", trusted, err)
	}
}

func TestParentOfNonRepoIsNotInherited(t *testing.T) {
	parent := mkdir(t, checkoutBase(t), "parent")
	child := mkdir(t, parent, "child")
	trusted, err := CheckoutTrusted(writeConfig(t, parent), child)
	if err != nil || trusted {
		t.Errorf("trusted = %v, err = %v", trusted, err)
	}
}

func TestSymlinkResolves(t *testing.T) {
	base := checkoutBase(t)
	target := mkdir(t, base, "target")
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	trusted, err := CheckoutTrusted(writeConfig(t, target), link)
	if err != nil || !trusted {
		t.Errorf("trusted = %v, err = %v", trusted, err)
	}
}

func TestMissingConfigMeansUntrusted(t *testing.T) {
	checkout := mkdir(t, checkoutBase(t), "plain")
	trusted, err := CheckoutTrusted(filepath.Join(t.TempDir(), "absent.json"), checkout)
	if err != nil || trusted {
		t.Errorf("trusted = %v, err = %v", trusted, err)
	}
}

func TestMalformedConfigIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckoutTrusted(path, "/anywhere"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestConfigIsNeverWritten(t *testing.T) {
	checkout := mkdir(t, checkoutBase(t), "plain")
	path := writeConfig(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CheckoutTrusted(path, checkout); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the trust check altered the operator's config")
	}
}
