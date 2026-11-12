// Package environ resolves the shell environment a project's work runs in:
// the operator's interactive shell, started in the checkout from a scrubbed
// base, with the project's activation applied. it reads the project's
// definition, .mega/environment.yaml, and never stores an environment.
package environ

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/michaelquigley/df/dd"
)

// FilePath is where a checkout defines its environment, relative to the
// checkout.
const FilePath = ".mega/environment.yaml"

// fileBody is the file's shape. activate is a short shell snippet run in the
// checkout; later keys may describe more, and are ignored here.
type fileBody struct {
	Activate string
}

// File is a checkout's .mega/environment.yaml as read now: whether it exists,
// its exact content and its fingerprint, when it last changed, and the
// activation it names. Malformed says why the file cannot be used, or is
// empty when it can.
type File struct {
	Path        string
	Present     bool
	Content     string
	Fingerprint string
	ModifiedAt  time.Time
	Activate    string
	Malformed   string
}

// ReadFile reads the checkout's definition. a missing file is no definition,
// not an error; one that cannot be read or bound is Malformed.
func ReadFile(checkout string) File {
	f := File{Path: filepath.Join(checkout, FilePath)}
	info, err := os.Stat(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return f
	}
	f.Present = true
	if err != nil {
		f.Malformed = fmt.Sprintf("'%s' cannot be read: %v", f.Path, err)
		return f
	}
	f.ModifiedAt = info.ModTime().UTC()
	if !info.Mode().IsRegular() {
		f.Malformed = fmt.Sprintf("'%s' is not a regular file", f.Path)
		return f
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		f.Malformed = fmt.Sprintf("'%s' cannot be read: %v", f.Path, err)
		return f
	}
	f.Content = string(data)
	f.Fingerprint = Fingerprint(data)
	body := &fileBody{}
	if err := dd.BindYAML(body, data); err != nil {
		f.Malformed = fmt.Sprintf("'%s' is not a valid environment file: %v", f.Path, err)
		return f
	}
	f.Activate = body.Activate
	return f
}

// Fingerprint names content by its hash: the first 16 hex digits of its
// sha256, prefixed so it reads as a fingerprint, never as the content.
func Fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}
