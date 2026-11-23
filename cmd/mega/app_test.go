package main

import (
	"encoding/xml"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestABareApplicationLaunchRunsTheDaemon(t *testing.T) {
	app := func(name string) string {
		if name == appLaunchEnv {
			return "1"
		}
		return ""
	}
	none := func(string) string { return "" }
	cases := []struct {
		name   string
		args   []string
		getenv func(string) string
		want   []string
		asApp  bool
	}{
		{"the application, bare", []string{}, app, []string{"daemon"}, true},
		{"the application with a command", []string{"version"}, app, []string{"version"}, false},
		{"the application's binary from a terminal", []string{}, none, []string{}, false},
		{"a command from a terminal", []string{"serve"}, none, []string{"serve"}, false},
	}
	for _, c := range cases {
		got, asApp := launchArgs(c.args, c.getenv)
		if !reflect.DeepEqual(got, c.want) || asApp != c.asApp {
			t.Errorf("%s: launchArgs = %q, %v, want %q, %v", c.name, got, asApp, c.want, c.asApp)
		}
	}
}

func TestHostOfARunningMega(t *testing.T) {
	got, err := hostOf("http://127.0.0.1:6342")
	if err != nil || got != "127.0.0.1:6342" {
		t.Errorf("hostOf = %q, %v", got, err)
	}
	for _, bad := range []string{"", "127.0.0.1:6342", "http://"} {
		if got, err := hostOf(bad); err == nil {
			t.Errorf("hostOf(%q) = %q, want an error", bad, got)
		}
	}
}

func TestLangFromTheMacOSLocale(t *testing.T) {
	installed := func(lang string) bool { return lang == "sv_SE.UTF-8" || lang == "en_US.UTF-8" }
	cases := map[string]string{
		"sv_SE":          "sv_SE.UTF-8",
		" sv_SE\n":       "sv_SE.UTF-8",
		"sv_SE@rg=sezzz": "sv_SE.UTF-8",
		// a language and region with no locale of their own installed.
		"en_SE@rg=sezzzz": fallbackLang,
		"":                fallbackLang,
	}
	for locale, want := range cases {
		if got := langFor(locale, installed); got != want {
			t.Errorf("langFor(%q) = %q, want %q", locale, got, want)
		}
	}
}

func TestBrowserOpeners(t *testing.T) {
	for goos, want := range map[string]string{"linux": "xdg-open", "darwin": "open"} {
		argv, err := browserArgv(goos, "http://127.0.0.1:6342")
		if err != nil || !reflect.DeepEqual(argv, []string{want, "http://127.0.0.1:6342"}) {
			t.Errorf("%s: browserArgv = %q, %v", goos, argv, err)
		}
	}
	if _, err := browserArgv("windows", "http://127.0.0.1:6342"); err == nil {
		t.Error("windows has no opener, yet browserArgv found one")
	}
}

func TestTrayItemsAtTheirIndices(t *testing.T) {
	var clicked []string
	items := trayItems(
		func() { clicked = append(clicked, "open") },
		func() { clicked = append(clicked, "browse") },
		func() { clicked = append(clicked, "gate") },
	)
	if len(items) != itemGate+1 {
		t.Fatalf("%d items, want %d", len(items), itemGate+1)
	}
	if items[itemOpen].Label != "open mega" || items[itemGate].Label != gateArmLabel {
		t.Errorf("labels: %q at itemOpen, %q at itemGate", items[itemOpen].Label, items[itemGate].Label)
	}
	for _, item := range items {
		item.OnClick()
	}
	want := []string{"open", "gate"}
	if nativeWindow {
		// "open mega" is the window; the browser is the second item.
		if items[itemBrowser].Label != "open in browser" {
			t.Errorf("label at itemBrowser: %q", items[itemBrowser].Label)
		}
		want = []string{"open", "browse", "gate"}
	}
	if !reflect.DeepEqual(clicked, want) {
		t.Errorf("clicks reached %q, want %q", clicked, want)
	}
}

// plistValues reads a flat Info.plist dict, with one nested dict, as
// key -> value ("true" and "false" for booleans, "key=value" lines for a
// dict).
func plistValues(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	values := map[string]string{}
	var key, nested, inner string
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "dict":
				depth++
			case "key", "string":
				var text string
				if err := dec.DecodeElement(&text, &el); err != nil {
					t.Fatal(err)
				}
				switch {
				case el.Name.Local == "key" && depth == 1:
					key = text
				case el.Name.Local == "key":
					inner = text
				case depth == 1:
					values[key] = text
				default:
					nested += inner + "=" + text + "\n"
				}
			case "true", "false":
				values[key] = el.Name.Local
			}
		case xml.EndElement:
			if el.Name.Local == "dict" {
				depth--
				if depth == 1 {
					values[key] = nested
					nested = ""
				}
			}
		}
	}
	return values
}

func TestTheApplicationsInfoPlist(t *testing.T) {
	values := plistValues(t, "macos/Info.plist")
	want := map[string]string{
		"CFBundleIdentifier":  appID,
		"CFBundleName":        desktopAppName,
		"CFBundleExecutable":  "mega",
		"CFBundleIconFile":    "mega",
		"CFBundlePackageType": "APPL",
		// a menu bar application: no Dock icon until a window opens.
		"LSUIElement": "true",
		// a launch from Finder runs the daemon (launchArgs).
		"LSEnvironment": appLaunchEnv + "=1\n",
	}
	for key, value := range want {
		if values[key] != value {
			t.Errorf("Info.plist %s = %q, want %q", key, values[key], value)
		}
	}
}
