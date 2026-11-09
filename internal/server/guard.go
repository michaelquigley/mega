package server

import (
	"net"
	"net/http"
	"net/url"
)

// guard keeps the service reachable only from this machine's own pages. the
// loopback bind stops the network, but a browser will happily carry a request
// from any site to 127.0.0.1: the Host check defeats DNS rebinding, and the
// Origin check defeats cross-site requests. an agent workspace with no login
// cannot let a web page launch or approve anything.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !isSameOrigin(origin, r.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isSameOrigin(origin, host string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == host
}
