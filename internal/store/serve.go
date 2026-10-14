package store

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
)

//go:embed web
var webFS embed.FS

// Page returns the UI as one self-contained HTML document: the webview
// loads it with SetHtml, so there is no server and no URL to reach it by.
func Page() (string, error) {
	read := func(name string) (string, error) {
		b, err := webFS.ReadFile("web/" + name)
		return string(b), err
	}
	html, err := read("index.html")
	if err != nil {
		return "", err
	}
	for _, name := range []string{"store.css", "account.css"} {
		css, err := read(name)
		if err != nil {
			return "", err
		}
		html = strings.Replace(html, `<link rel="stylesheet" href="`+name+`">`, "<style>\n"+css+"</style>", 1)
	}
	for _, name := range []string{"store.js", "account.js"} {
		js, err := read(name)
		if err != nil {
			return "", err
		}
		html = strings.Replace(html, `<script src="`+name+`"></script>`, "<script>\n"+js+"</script>", 1)
	}
	return html, nil
}

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
