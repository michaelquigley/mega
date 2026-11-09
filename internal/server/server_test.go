package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/harness/harnesstest"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/registry"
	"git.hq.quigley.com/research/roundhouse/internal/service"
)

type fixture struct {
	url      string
	svc      *service.Service
	fake     *harnesstest.Harness
	regPath  string
	checkout string
}

func start(t *testing.T) *fixture {
	t.Helper()
	return startWith(t, defaultHeartbeat)
}

func startWith(t *testing.T, heartbeat time.Duration) *fixture {
	t.Helper()
	regPath := filepath.Join(t.TempDir(), "registry.yaml")
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(filepath.Dir(regPath), "conversations")
	fake := harnesstest.New(model.HarnessClaude)
	svc, err := service.New(reg, service.Options{
		IsUntrusted:  func(checkout string) bool { return strings.HasSuffix(checkout, "untrusted") },
		EventLogPath: func(id string) string { return filepath.Join(logs, id, "events.jsonl") },
		Harnesses:    []harness.Harness{fake},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := New(svc)
	srv.heartbeat = heartbeat
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		svc.Close()
		ts.Close()
	})
	return &fixture{url: ts.URL, svc: svc, fake: fake, regPath: regPath, checkout: t.TempDir()}
}

func (f *fixture) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, f.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	// test-only inspection of emitted bytes.
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("%s %s: decoding: %v", method, path, err)
	}
	return resp.StatusCode, out
}

func createBody(name, checkout string) string {
	data, _ := json.Marshal(map[string]string{"name": name, "checkout": checkout})
	return string(data)
}

func TestWorkspaceStartsEmptyWithArrays(t *testing.T) {
	f := start(t)
	status, body := f.do(t, http.MethodGet, "/api/workspace", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	for _, key := range []string{"projects", "conversations"} {
		list, ok := body[key].([]any)
		if !ok || len(list) != 0 {
			t.Errorf("%s = %#v, want an empty array", key, body[key])
		}
	}
}

func TestProjectLifecycle(t *testing.T) {
	f := start(t)
	status, created := f.do(t, http.MethodPost, "/api/projects", createBody("fixture", f.checkout))
	if status != http.StatusCreated {
		t.Fatalf("create status = %d, body %v", status, created)
	}
	id, _ := created["id"].(string)
	if id == "" || created["name"] != "fixture" || created["checkout"] != f.checkout || created["untrusted"] != false {
		t.Fatalf("created = %v", created)
	}

	status, updated := f.do(t, http.MethodPatch, "/api/projects/"+id, `{"name":"renamed"}`)
	if status != http.StatusOK || updated["name"] != "renamed" {
		t.Fatalf("update: %d %v", status, updated)
	}

	_, workspace := f.do(t, http.MethodGet, "/api/workspace", "")
	projects := workspace["projects"].([]any)
	if len(projects) != 1 || projects[0].(map[string]any)["name"] != "renamed" {
		t.Fatalf("workspace = %v", workspace)
	}

	status, _ = f.do(t, http.MethodDelete, "/api/projects/"+id, "")
	if status != http.StatusNoContent {
		t.Fatalf("delete status = %d", status)
	}
	if _, err := os.Stat(f.checkout); err != nil {
		t.Errorf("checkout was touched: %v", err)
	}
	_, workspace = f.do(t, http.MethodGet, "/api/workspace", "")
	if len(workspace["projects"].([]any)) != 0 {
		t.Fatalf("workspace after delete = %v", workspace)
	}
}

func TestTrustNoticeReachesTheWire(t *testing.T) {
	f := start(t)
	dir := filepath.Join(t.TempDir(), "untrusted")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, created := f.do(t, http.MethodPost, "/api/projects", createBody("x", dir))
	if created["untrusted"] != true {
		t.Errorf("created = %v", created)
	}
	_, workspace := f.do(t, http.MethodGet, "/api/workspace", "")
	if workspace["projects"].([]any)[0].(map[string]any)["untrusted"] != true {
		t.Errorf("workspace = %v", workspace)
	}
	body, err := os.ReadFile(f.regPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "untrusted:") {
		t.Errorf("notice was persisted:\n%s", body)
	}
}

func TestErrorStatuses(t *testing.T) {
	f := start(t)
	if status, _ := f.do(t, http.MethodPost, "/api/projects", createBody("a", f.checkout)); status != http.StatusCreated {
		t.Fatalf("seed status = %d", status)
	}
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"malformed body", http.MethodPost, "/api/projects", "{nope", http.StatusBadRequest, "invalid"},
		{"bad checkout", http.MethodPost, "/api/projects", createBody("b", "/definitely/absent"), http.StatusBadRequest, "invalid"},
		{"duplicate", http.MethodPost, "/api/projects", createBody("b", f.checkout), http.StatusConflict, "conflict"},
		{"rename absent", http.MethodPatch, "/api/projects/absent", `{"name":"x"}`, http.StatusNotFound, "not_found"},
		{"rename empty", http.MethodPatch, "/api/projects/absent", `{"name":" "}`, http.StatusBadRequest, "invalid"},
		{"delete absent", http.MethodDelete, "/api/projects/absent", "", http.StatusNotFound, "not_found"},
	} {
		status, body := f.do(t, tc.method, tc.path, tc.body)
		if status != tc.status || body["code"] != tc.code || body["message"] == "" {
			t.Errorf("%s: %d %v", tc.name, status, body)
		}
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	f := start(t)
	huge := `{"name":"` + strings.Repeat("x", maxRequestBody+1) + `"}`
	status, body := f.do(t, http.MethodPost, "/api/projects", huge)
	if status != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, body %v", status, body)
	}
}

func TestUnavailableCarriesItsMessage(t *testing.T) {
	// the service refuses writes this way once registry durability is uncertain;
	// the registry has already logged it, and the operator needs the reason.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects", nil)
	writeError(rec, req, &service.Error{Code: service.CodeUnavailable, Message: "restart required"})
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"restart required"`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServerSideFaultHidesDetail(t *testing.T) {
	f := start(t)
	// make the registry unwritable, which is an internal fault rather than a caller error.
	if err := os.Chmod(filepath.Dir(f.regPath), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Dir(f.regPath), 0o700) })
	status, body := f.do(t, http.MethodPost, "/api/projects", createBody("a", f.checkout))
	if status == http.StatusCreated {
		t.Skip("directory permissions are not enforced here")
	}
	if status != http.StatusInternalServerError || body["code"] != "internal" || body["message"] != "internal error" {
		t.Errorf("%d %v", status, body)
	}
}

func TestGuard(t *testing.T) {
	f := start(t)
	for _, tc := range []struct {
		name, host, origin string
		status             int
	}{
		{"same origin", "", "SAME", http.StatusOK},
		{"no origin", "", "", http.StatusOK},
		{"localhost host", "localhost:8470", "", http.StatusOK},
		{"ipv6 loopback host", "[::1]:8470", "", http.StatusOK},
		{"rebound host", "evil.example:8470", "", http.StatusForbidden},
		{"cross-site origin", "", "http://evil.example", http.StatusForbidden},
		{"other local port", "", "http://127.0.0.1:1", http.StatusForbidden},
		{"null origin", "", "null", http.StatusForbidden},
	} {
		req, err := http.NewRequest(http.MethodGet, f.url+"/api/workspace", nil)
		if err != nil {
			t.Fatal(err)
		}
		if tc.host != "" {
			req.Host = tc.host
		}
		switch tc.origin {
		case "":
		case "SAME":
			req.Header.Set("Origin", f.url)
		default:
			req.Header.Set("Origin", tc.origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.name, resp.StatusCode, tc.status)
		}
	}
}

func dialStream(t *testing.T, f *fixture) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.url, "http") + "/api/stream"
	header := http.Header{"Origin": {f.url}}
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, status)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	return conn
}

func TestStreamRejectsCrossSite(t *testing.T) {
	f := start(t)
	url := "ws" + strings.TrimPrefix(f.url, "http") + "/api/stream"
	header := http.Header{"Origin": {"http://evil.example"}}
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err == nil {
		conn.Close()
		t.Fatal("cross-site stream connected")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("resp = %+v", resp)
	}
}

func TestStreamFansOutToEveryBrowser(t *testing.T) {
	f := start(t)
	first := dialStream(t, f)
	second := dialStream(t, f)

	if status, body := f.do(t, http.MethodPost, "/api/projects", createBody("a", f.checkout)); status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, body)
	}
	for name, conn := range map[string]*websocket.Conn{"first": first, "second": second} {
		var frame map[string]any
		if err := conn.ReadJSON(&frame); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if frame["workspace_changed"] != true {
			t.Errorf("%s: frame = %v", name, frame)
		}
		if _, present := frame["event"]; present {
			t.Errorf("%s: empty fields leaked: %v", name, frame)
		}
	}
}

func TestStreamHeartbeats(t *testing.T) {
	f := startWith(t, 20*time.Millisecond)
	conn := dialStream(t, f)
	var frame map[string]any
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	if _, ok := frame["heartbeat"]; !ok {
		t.Errorf("frame = %v", frame)
	}
}

func TestStreamEndsOnServiceClose(t *testing.T) {
	f := start(t)
	conn := dialStream(t, f)
	f.svc.Close()
	_, _, err := conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
		t.Errorf("err = %v", err)
	}
}

func TestSinglePageFallbackAndAPIBoundary(t *testing.T) {
	f := start(t)
	resp, err := http.Get(f.url + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("index: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp, err = http.Get(f.url + "/api/absent")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown api path: %d", resp.StatusCode)
	}
}
