package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func star(starred bool) ProjectChange { return ProjectChange{Starred: &starred} }

// a star is written to the registry, browsers are told, and it survives a
// restart.
func TestProjectStarIsPersistedAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	svc := newService(t, path, allTrusted)
	project, err := svc.RegisterProject("fixture", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub := svc.Subscribe()
	defer sub.Close()

	starred, err := svc.UpdateProject(project.ID, star(true))
	if err != nil || !starred.Starred || starred.Name != "fixture" {
		t.Fatalf("star: %+v, %v", starred, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "starred: true") {
		t.Errorf("registry after the star:\n%s (%v)", data, err)
	}
	if update := <-sub.Updates(); !update.WorkspaceChanged {
		t.Errorf("browsers were not told: %+v", update)
	}
	if !workspaceOf(t, svc).Projects[0].Starred {
		t.Error("memory did not move after the confirmed write")
	}

	svc.Close()
	again := newService(t, path, allTrusted)
	if got := workspaceOf(t, again).Projects[0]; !got.Starred || got.Name != "fixture" {
		t.Errorf("after a restart: %+v", got)
	}
	unstarred, err := again.UpdateProject(project.ID, star(false))
	if err != nil || unstarred.Starred {
		t.Fatalf("unstar: %+v, %v", unstarred, err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "starred") {
		t.Errorf("an unstarred project still writes the field:\n%s", data)
	}
}

// a registry written before stars has no field: it reads as unstarred, and
// its version is unchanged.
func TestOldRegistryReadsAsUnstarred(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.yaml")
	checkout := t.TempDir()
	old := "version: 1\nprojects:\n  - id: p1\n    name: old\n    checkout: " + checkout + "\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := newService(t, path, allTrusted)
	projects := workspaceOf(t, svc).Projects
	if len(projects) != 1 || projects[0].ID != "p1" || projects[0].Starred {
		t.Fatalf("projects = %+v", projects)
	}
	if _, err := svc.UpdateProject("p1", star(true)); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "\nversion: 1\n") {
		t.Errorf("the registry version changed:\n%s", data)
	}
}

func TestUpdateProjectRules(t *testing.T) {
	svc := tempService(t)
	project, err := svc.RegisterProject("a", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.UpdateProject(project.ID, ProjectChange{})
	wantCode(t, err, CodeInvalid)
	_, err = svc.UpdateProject("absent", star(true))
	wantCode(t, err, CodeNotFound)
	name := "b"
	both, err := svc.UpdateProject(project.ID, ProjectChange{Name: &name, Starred: &[]bool{true}[0]})
	if err != nil || both.Name != "b" || !both.Starred {
		t.Fatalf("both at once: %+v, %v", both, err)
	}
	// a rename alone leaves the star.
	if renamed, err := svc.RenameProject(project.ID, "c"); err != nil || !renamed.Starred {
		t.Errorf("rename dropped the star: %+v, %v", renamed, err)
	}
}
