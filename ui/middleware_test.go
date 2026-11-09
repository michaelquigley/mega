package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticHandler(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := newStaticHandler(api, fstest.MapFS{
		"index.html":    {Data: []byte("<html>index</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	})
	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{
		{"/api/anything", http.StatusTeapot, ""},
		{"/", http.StatusOK, "<html>index</html>"},
		{"/some/route", http.StatusOK, "<html>index</html>"},
		{"/assets/app.js", http.StatusOK, "console.log(1)"},
		{"/assets/absent.js", http.StatusNotFound, "404 page not found\n"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.path, rec.Code, tc.status)
		}
		if tc.body != "" && rec.Body.String() != tc.body {
			t.Errorf("%s: body = %q", tc.path, rec.Body.String())
		}
	}
}
