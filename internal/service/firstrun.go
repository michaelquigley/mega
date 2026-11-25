package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
)

// HarnessStatus is whether a harness can run in the operator's shell
// environment, as a project with no activation gets it: where its programs
// were found, or why it cannot run. it is what the start-up log says and what
// the empty workspace shows; a project's own environment may differ, and its
// page says what is missing there.
type HarnessStatus struct {
	Name  string
	Found bool
	// Where names every program it runs, for the log; Path is the harness's
	// own, for the first page.
	Where string
	Path  string
	// Reason is why it cannot run, without the harness's name, which the
	// page and the log put before it.
	Reason string
	// Version is what the harness's own --version printed, Tested its tested
	// range (harness.Tested), and Standing and Note where the version stands
	// against that range and the sentence for one that needs a look (older,
	// or unreadable; a newer build has none). Version and Standing
	// are empty until the version has been read; an unreadable one is
	// StandingUnknown.
	Version  string
	Tested   string
	Standing string
	Note     string
}

// versionRead is a harness's version as last read, where its programs were
// then, and the identity of its own program's file (programIdentity): a read
// is kept until the harness is found somewhere else or its program is
// another file, as after an update behind the same path.
type versionRead struct {
	where    string
	identity string
	version  string
}

// programIdentity names the file path resolves to, as it is now: its
// resolved path, device, inode, size, and modification and change times. an
// updater that moves a link to a new build changes the resolved path; one
// that replaces the program in place, as an npm update does, writes a new
// file, a new inode and change time, even where it keeps the modification
// time npm's tarballs carry. empty when path cannot be read.
func programIdentity(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return ""
	}
	id := fmt.Sprintf("%s:%d:%d", resolved, info.Size(), info.ModTime().UnixNano())
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		csec, cnsec := changeTime(st)
		id += fmt.Sprintf(":%d:%d:%d.%d", st.Dev, st.Ino, csec, cnsec)
	}
	return id
}

// checkHarnesses finds, once at start-up and in the background, where each
// harness's programs are in the operator's shell environment: the shell run
// in the home directory with no activation, as a project without one gets it.
// a shell that cannot be resolved leaves mega's own environment to look in.
// each harness gets one log line, found where or missing and why, and the
// environment is kept, so the empty workspace's report follows an install
// without a restart. callers do not hold s.mu.
func (s *Service) checkHarnesses() {
	home, err := os.UserHomeDir()
	var find []string
	where := "the operator's shell environment"
	if err == nil {
		resolution, resolveErr := s.resolver.Resolve(context.Background(), home, "")
		if resolveErr == nil {
			find = resolution.Before
		} else {
			err = resolveErr
		}
	}
	if find == nil {
		dl.Warnf("the shell environment a project gets could not be resolved, so the harnesses are looked for in mega's own: %v", err)
		find, where = os.Environ(), "mega's own environment"
	}
	s.mu.Lock()
	s.harnessFind, s.harnessWhere = find, where
	s.versionsReading = true
	s.mu.Unlock()
	s.readVersions()
	// a snapshot of the configuration, in this same background work, when
	// it differs from the newest one or a version is new.
	defer s.snapshotConfiguration(false, nil)
	s.mu.Lock()
	report := s.harnessStatusLocked()
	s.changed()
	s.mu.Unlock()
	for _, status := range report {
		switch {
		case !status.Found:
			dl.Warnf("harness '%s' cannot run: %s (looked for in %s)", status.Name, status.Reason, where)
		case status.Standing == harness.StandingTested:
			dl.Infof("harness '%s' can run, in %s: %s; version '%s', of the '%s' mega was tested with", status.Name, where, status.Where, status.Version, status.Tested)
		case status.Standing == harness.StandingNewer:
			// the normal state for a harness that updates itself.
			dl.Infof("harness '%s' can run, in %s: %s; version '%s', newer than the '%s' mega was tested with", status.Name, where, status.Where, status.Version, status.Tested)
		default:
			dl.Warnf("harness '%s' can run, in %s: %s; %s", status.Name, where, status.Where, status.Note)
		}
	}
}

// readVersions reads each harness's version where the start-up check found
// it, with harness.VersionTimeout each, and keeps them for the reports. a
// read that fails keeps an empty version, reported as unknown. callers do
// not hold s.mu; it clears versionsReading when done.
func (s *Service) readVersions() {
	s.mu.Lock()
	find := s.harnessFind
	found := map[string]versionRead{}
	for _, status := range s.harnessStatusLocked() {
		if status.Found {
			// the identity is taken before the read, so a change during it
			// is seen as one on the next look.
			found[status.Name] = versionRead{where: status.Where, identity: programIdentity(status.Path)}
		}
	}
	s.mu.Unlock()
	reads := map[string]versionRead{}
	for name, read := range found {
		if v, ok := s.harnesses[name].(harness.Versioner); ok {
			version, err := v.Version(context.Background(), find)
			if err != nil {
				dl.Debugf("reading the version of '%s': %v", name, err)
			}
			read.version = version
		}
		reads[name] = read
	}
	s.mu.Lock()
	s.harnessVersions = reads
	s.versionsReading = false
	s.mu.Unlock()
}

// Harnesses is whether each harness can run in the operator's shell
// environment, with its version against its tested range, as the workspace
// read carries it; nil until the start-up check has run. once the service
// has closed it is refused as unavailable.
func (s *Service) Harnesses() ([]HarnessStatus, error) {
	if err := s.lockOpen(); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	return s.harnessStatusLocked(), nil
}

// harnessStatusLocked checks each harness, in name order, in the environment
// the start-up check found; nil until it has. a check reads names, modes and
// codex's package manifest only, so it is cheap enough for every workspace
// read. versions come from the last read (readVersions), never from running
// anything here. callers hold s.mu.
func (s *Service) harnessStatusLocked() []HarnessStatus {
	if s.harnessFind == nil {
		return nil
	}
	names := make([]string, 0, len(s.harnesses))
	for name := range s.harnesses {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]HarnessStatus, 0, len(names))
	stale := false
	defer func() {
		// a harness found somewhere new since its version was read, or whose
		// program is another file now (an install, an update moving the link
		// or replacing the program in place), is read again, in the
		// background. the file watch looks every envWatchEvery, so the page
		// hears of an update with nothing else changing.
		if stale && !s.versionsReading && !s.closed {
			s.versionsReading = true
			s.background.Add(1)
			go func() {
				defer s.background.Done()
				s.readVersions()
				s.mu.Lock()
				s.changed()
				s.mu.Unlock()
			}()
		}
	}()
	for _, name := range names {
		h := s.harnesses[name]
		status := HarnessStatus{Name: name}
		var missing *harness.MissingError
		switch err := h.Check(s.harnessFind); {
		case err == nil:
			status.Found = true
			if locator, ok := h.(harness.Locator); ok {
				status.Where = locator.Locate(s.harnessFind)
				status.Path = locator.Path(s.harnessFind)
			}
			status.Tested = harness.Tested[name].String()
			if read, ok := s.harnessVersions[name]; ok && read.where == status.Where && read.identity == programIdentity(status.Path) {
				status.Version = read.version
				status.Standing, status.Note = harness.Stand(name, read.version)
			} else {
				stale = true
			}
		case errors.As(err, &missing):
			// the check reads a project's PATH; here it is the shell's own,
			// before any project exists.
			status.Reason = strings.ReplaceAll(missing.Reason, "the project's PATH", "your shell's PATH")
		default:
			status.Reason = err.Error()
		}
		out = append(out, status)
	}
	return out
}
