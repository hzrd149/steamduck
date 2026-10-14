package store

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
)

// The checked-in artifacts are rebuilt with npm run build. The production
// napplet is kept separate from the compatibility window/browser transport.
//
//go:embed built/napplet.html
var nappletHTML string

//go:embed built/legacy.html
var legacyHTML string

//go:embed built/steamduck.js
var steamduckPrelude string

// SteamduckPrelude is runtime-owned code for injection into the authorized
// built-in store's trusted preamble. It must never be added to ordinary napplets.
func SteamduckPrelude() string { return steamduckPrelude }

// NappletPage returns the single-file store for a trusted Kwakore built-in
// session. Its host must provide NAP-STEAMDUCK before application scripts run.
func NappletPage() string { return nappletHTML }

// Page returns the compatibility build for --legacy-window and the loopback
// dev server. The default store uses NappletPage through Kwakore's built-in API.
func Page() (string, error) { return legacyHTML, nil }

// Serve runs the UI over HTTP on a loopback address for development in an
// ordinary browser. Calls need the random token printed in the URL, so other
// pages the browser has open cannot drive kwakore.
func Serve(ctx context.Context, api *API, addr string, ready func(url string)) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return &net.AddrError{Err: "the development server only listens on loopback", Addr: addr}
	}
	tok := make([]byte, 16)
	_, _ = rand.Read(tok)
	token := hex.EncodeToString(tok)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page, err := Page()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' data:")
		_, _ = io.WriteString(w, page)
	})
	mux.HandleFunc("POST /api/{method}", func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Steamduck-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.dispatch(r.Context(), r.PathValue("method"), body))
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if ready != nil {
		ready("http://" + ln.Addr().String() + "/#token=" + token)
	}
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}
