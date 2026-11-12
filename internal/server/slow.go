package server

import (
	"net/http"
	"time"

	"github.com/michaelquigley/df/dl"
)

// slowAfter is how long a request may take before it is logged.
const slowAfter = time.Second

// slowReport is what a slow request leaves in the log.
type slowReport struct {
	Method string
	Path   string // with its query, which says what was asked for
	Took   time.Duration
	Bytes  int64
	Status int
}

func logSlow(report slowReport) {
	dl.Warnf("slow request: '%s' '%s' took %s, %d bytes, status %d", report.Method, report.Path, report.Took.Round(time.Millisecond), report.Bytes, report.Status)
}

// slowRequests reports every request that takes longer than after, with its
// path, how long it took and how much it answered, so the next slow load says
// what was slow. the stream is long-lived by design and is not timed.
func slowRequests(next http.Handler, after time.Duration, report func(slowReport)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stream" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		counted := &countingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(counted, r)
		if took := time.Since(start); took > after {
			report(slowReport{Method: r.Method, Path: r.URL.RequestURI(), Took: took, Bytes: counted.bytes, Status: counted.status})
		}
	})
}

// countingWriter counts what a response writes and keeps its status.
type countingWriter struct {
	http.ResponseWriter
	bytes  int64
	status int
}

func (c *countingWriter) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
