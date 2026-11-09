// Package registry persists the workspace arrangement as a plain YAML file.
package registry

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/durable"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

const currentVersion = 1

// Document is the registry's full contents.
type Document struct {
	Version       int
	Projects      []model.Project
	Conversations []model.Conversation
}

func (d *Document) clone() *Document {
	return &Document{
		Version:       d.Version,
		Projects:      slices.Clone(d.Projects),
		Conversations: slices.Clone(d.Conversations),
	}
}

// ErrUncertain marks a registry that no longer accepts changes because one
// landed on disk without its directory sync succeeding.
var ErrUncertain = errors.New("registry durability uncertain after a failed directory sync, restart required")

// syncDir is a variable so a test can force the one failure that leaves the
// file changed.
var syncDir = durable.SyncDir

// Registry is the on-disk authority for projects and conversations. it is not
// safe for concurrent use; the service is its only caller and serializes access.
type Registry struct {
	path      string
	doc       *Document
	uncertain error
}

// Load reads the registry at path. a missing file is an empty registry and is
// not created until the first update.
func Load(path string) (*Registry, error) {
	doc := &Document{Version: currentVersion}
	if err := dd.BindYAMLFile(doc, path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading registry '%s': %w", path, err)
	}
	if doc.Version != currentVersion {
		return nil, fmt.Errorf("registry '%s' has version %d, this build reads %d", path, doc.Version, currentVersion)
	}
	return &Registry{path: path, doc: doc}, nil
}

// Snapshot returns a copy of the current contents.
func (r *Registry) Snapshot() *Document {
	return r.doc.clone()
}

// Update applies fn to a copy and writes the result. the in-memory state only
// advances once the file is durable, so what callers read is always what a
// restart would read.
//
// a failure before the rename leaves the file and memory as they were, and a
// retry is honest. a failure of the directory sync after the rename is not: the
// file may hold a change that was never confirmed. memory keeps the last
// confirmed document and reads continue, but every later update is refused with
// ErrUncertain so nothing builds on an uncertain base until a restart reads
// whatever reached the disk.
func (r *Registry) Update(fn func(*Document) error) error {
	if r.uncertain != nil {
		return r.uncertain
	}
	next := r.doc.clone()
	if err := fn(next); err != nil {
		return err
	}
	if err := r.replace(next); err != nil {
		return fmt.Errorf("writing registry '%s': %w", r.path, err)
	}
	if err := syncDir(filepath.Dir(r.path)); err != nil {
		r.uncertain = fmt.Errorf("%w: '%s': %w", ErrUncertain, r.path, err)
		dl.Errorf("%v", r.uncertain)
		return r.uncertain
	}
	r.doc = next
	return nil
}

// replace writes doc to a synced temporary file and renames it over the
// registry. the caller syncs the directory.
func (r *Registry) replace(doc *Document) error {
	body, err := dd.UnbindYAML(doc)
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.path)
	if err := durable.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(r.path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), r.path)
}

// NewID mints an identifier that is safe to use as a directory name.
func NewID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("reading random bytes: %w", err))
	}
	return hex.EncodeToString(b)
}
