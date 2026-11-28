package e2e

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
)

// this file is the ordinary gate's hold on the map: it has no build tag, so
// `make test` runs it, and it runs no harness.

// interfaces are the adapter interfaces whose every method or callback the
// map answers for, harness by harness.
var interfaces = map[string]reflect.Type{
	ListHarness:   reflect.TypeFor[harness.Harness](),
	ListRuntime:   reflect.TypeFor[harness.Runtime](),
	ListSink:      reflect.TypeFor[harness.Sink](),
	ListLooker:    reflect.TypeFor[harness.Looker](),
	ListLocator:   reflect.TypeFor[harness.Locator](),
	ListVersioner: reflect.TypeFor[harness.Versioner](),
}

// decisions are what an answer carries to a harness, harness.Decision*. a
// constant cannot be listed by reflection, so a new decision is added here
// and in the map.
var decisions = []string{harness.DecisionAllow, harness.DecisionDeny, harness.DecisionAllowSession, harness.DecisionDismiss}

var mapped = []string{hClaude, hCodex, hPi}

// roundTrips are the round trips the e2e files declare: every func TestX(t
// *testing.T) in a test file behind the e2e build tag. the files are parsed,
// not built, so the ordinary gate reads them without the tag.
func roundTrips(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if !taggedE2E(f) {
			continue
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" {
				found[fn.Name.Name] = true
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no round trip found in the e2e test files")
	}
	return found
}

func taggedE2E(f *ast.File) bool {
	for _, group := range f.Comments {
		if group.Pos() > f.Package {
			break
		}
		for _, c := range group.List {
			if c.Text == "//go:build e2e" {
				return true
			}
		}
	}
	return false
}

type rowKey struct{ harness, list, entry string }

func rowsByKey(t *testing.T) map[rowKey]Row {
	t.Helper()
	rows := map[rowKey]Row{}
	for _, r := range Map {
		k := rowKey{r.Harness, r.List, r.Entry}
		if _, dup := rows[k]; dup {
			t.Errorf("two rows for %s %s %q", r.Harness, r.List, r.Entry)
		}
		rows[k] = r
	}
	return rows
}

// TestTheMapAnswersForEveryInterfaceMethod: every method of the adapter
// interfaces, and every decision an answer carries, has a row for each
// harness, and no row names one that is not there.
func TestTheMapAnswersForEveryInterfaceMethod(t *testing.T) {
	rows := rowsByKey(t)
	want := map[rowKey]bool{}
	for list, typ := range interfaces {
		for i := range typ.NumMethod() {
			for _, h := range mapped {
				want[rowKey{h, list, typ.Method(i).Name}] = true
			}
		}
	}
	for _, d := range decisions {
		for _, h := range mapped {
			want[rowKey{h, ListAnswers, d}] = true
		}
	}
	for k := range want {
		if _, ok := rows[k]; !ok {
			t.Errorf("%s has no row in the map for %s %s", k.harness, k.list, k.entry)
		}
	}
	for k := range rows {
		_, iface := interfaces[k.list]
		if (iface || k.list == ListAnswers) && !want[k] {
			t.Errorf("the map has a row for %s %s %s, which is not there", k.harness, k.list, k.entry)
		}
	}
}

// TestTheMapNamesRoundTripsThatExist: every row names round trips that are
// declared, or says why none makes it, and not both; every round trip is
// named by a row, so that it counts toward its harness's admission.
func TestTheMapNamesRoundTripsThatExist(t *testing.T) {
	declared := roundTrips(t)
	named := map[string]bool{}
	known := map[string]bool{hClaude: true, hCodex: true, hPi: true}
	for _, r := range Map {
		if !known[r.Harness] {
			t.Errorf("row %s %q names no harness mega has: %q", r.List, r.Entry, r.Harness)
		}
		switch {
		case len(r.By) == 0 && r.None == "":
			t.Errorf("%s %s %q names no round trip and says no reason", r.Harness, r.List, r.Entry)
		case len(r.By) > 0 && r.None != "":
			t.Errorf("%s %s %q names round trips and a reason for none", r.Harness, r.List, r.Entry)
		}
		for _, name := range r.By {
			named[name] = true
			if !declared[name] {
				t.Errorf("%s %s %q names %s, which is not a round trip", r.Harness, r.List, r.Entry, name)
			}
		}
	}
	for name := range declared {
		if !named[name] {
			t.Errorf("no row names the round trip %s, so it counts toward no harness", name)
		}
	}
}
