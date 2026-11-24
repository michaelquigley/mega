package service

import (
	"sort"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/snapshot"
)

// snapshotConfiguration takes a configuration snapshot in the operator's
// shell environment, as the start-up check found it: at start-up when the
// configuration differs from the newest snapshot, and whenever a harness
// reports a version no snapshot has recorded (onlyNewVersion, with that
// version in seen). it reads the harnesses' configuration and writes only
// under the store; what it logs is names and counts, never a file's
// content. callers do not hold s.mu.
func (s *Service) snapshotConfiguration(onlyNewVersion bool, seen map[string]string) {
	if s.snapshots == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.snapshotting.Add(1)
	defer s.snapshotting.Done()
	find := s.harnessFind
	versions := map[string]string{}
	for name, read := range s.harnessVersions {
		versions[name] = read.version
	}
	s.mu.Unlock()
	if find == nil {
		return
	}
	for name, version := range seen {
		if version != "" {
			versions[name] = version
		}
	}
	names := make([]string, 0, len(s.harnesses))
	for name := range s.harnesses {
		names = append(names, name)
	}
	sort.Strings(names)
	var roots []harness.ConfigRoot
	for _, name := range names {
		if c, ok := s.harnesses[name].(harness.Configurer); ok {
			roots = append(roots, c.Configuration(find)...)
		}
	}
	res, err := s.snapshots.Take(roots, versions, onlyNewVersion)
	switch {
	case err != nil:
		dl.Warnf("a configuration snapshot could not be taken in '%s': %v", s.snapshots.Dir(), err)
	case res.Manifest != nil:
		dl.Infof("configuration snapshot '%s' taken at '%s' in '%s': %d files, %d new, %d bytes new", res.Manifest.Name, res.Manifest.Reason, s.snapshots.Dir(), len(res.Manifest.Files), res.NewFiles, res.NewBytes)
		s.mu.Lock()
		s.changed()
		s.mu.Unlock()
	}
}

// versionSeen is told the version a launch is about to run. one no snapshot
// has recorded, the moment a migration may just have run, takes a snapshot
// in the background. callers do not hold s.mu.
func (s *Service) versionSeen(harnessName, version string) {
	if s.snapshots == nil || version == "" {
		return
	}
	fresh, err := s.snapshots.NewVersions(map[string]string{harnessName: version})
	if err != nil || len(fresh) == 0 {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.background.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.background.Done()
		s.snapshotConfiguration(true, map[string]string{harnessName: version})
	}()
}

// SnapshotSummary is what the newest configuration snapshot is, by name and
// time, and where the store is: never what a snapshot holds.
type SnapshotSummary struct {
	Dir    string
	Count  int
	Newest *snapshot.Manifest
}

// Snapshots summarizes the configuration snapshots; nil when none are kept.
// once the service has closed it is refused as unavailable.
func (s *Service) Snapshots() (*SnapshotSummary, error) {
	if err := s.lockOpen(); err != nil {
		return nil, err
	}
	s.mu.Unlock()
	if s.snapshots == nil {
		return nil, nil
	}
	all, err := s.snapshots.List()
	sum := &SnapshotSummary{Dir: s.snapshots.Dir(), Count: len(all)}
	if err == nil && len(all) > 0 {
		sum.Newest = all[0]
	}
	return sum, nil
}
