package environ

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// versionDepth is how many of the file's commits are searched for an approved
// version.
const versionDepth = 50

// Version is an earlier .mega/environment.yaml found by its fingerprint: its
// content and where it was found, a commit or the index.
type Version struct {
	Content string
	Found   string
}

// FindVersion looks for the version of the checkout's .mega/environment.yaml
// whose fingerprint is fingerprint, in the git index and the file's last
// commits, so a changed file can be shown beside the version the operator
// approved without that version ever being stored. git runs in env with no
// optional locks. a checkout outside git, or a version that was never
// committed or staged, is simply not found.
func FindVersion(ctx context.Context, checkout, fingerprint string, env []string) (Version, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	path, ok := lookPath("git", env)
	if !ok {
		return Version{}, false
	}
	git := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Dir = checkout
		cmd.Env = append(append([]string{}, env...), "GIT_OPTIONAL_LOCKS=0")
		return cmd.Output()
	}
	if data, err := git("show", ":"+FilePath); err == nil && Fingerprint(data) == fingerprint {
		return Version{Content: string(data), Found: "the git index"}, true
	}
	out, err := git("log", "-n", strconv.Itoa(versionDepth), "--format=%h", "--", FilePath)
	if err != nil {
		return Version{}, false
	}
	for _, commit := range strings.Fields(string(bytes.TrimSpace(out))) {
		data, err := git("show", commit+":"+FilePath)
		if err == nil && Fingerprint(data) == fingerprint {
			return Version{Content: string(data), Found: "commit " + commit}, true
		}
	}
	return Version{}, false
}

// lookPath finds an executable on env's PATH.
func lookPath(name string, env []string) (string, bool) {
	path, _ := Value(env, "PATH")
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, true
		}
	}
	return "", false
}
