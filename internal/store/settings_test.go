package store

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

// Exercise settings through the same native envelope and socket path the
// store uses, including the difference between a reset and an empty override.
func TestNativeSettingsRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	type request struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	received := make(chan request, 10)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					var req request
					if json.Unmarshal(scanner.Bytes(), &req) != nil {
						return
					}
					received <- req
					result := map[string]any{"settings": map[string]any{"relays": []string{"wss://configured.example"}}}
					if req.Method == "settings.inspect" {
						result["sources"] = map[string]string{"relays": "config"}
					}
					_ = json.NewEncoder(conn).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
				}
			}()
		}
	}()
	client, _ := kwakrpc.New(path)
	t.Cleanup(func() { client.Close() })
	api := &API{Client: client.WithoutRetry()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, row := range []struct{ method, params string }{
		{"settings.inspect", `null`},
		{"settings.set", `{"field":"relays","value":[]}`},
		{"settings.clear", `{"field":"relays"}`},
		{"settings.reload", `null`},
	} {
		envelope, _ := json.Marshal(map[string]any{"type": "steamduck.request", "id": "settings", "method": row.method, "params": json.RawMessage(row.params)})
		response := api.HandleNative(ctx, kwakrpc.ExtensionRequest{Domain: "steamduck", Envelope: envelope})
		var decoded map[string]json.RawMessage
		if json.Unmarshal(response.Envelope, &decoded) != nil || decoded["error"] != nil {
			t.Fatalf("%s: %s", row.method, response.Envelope)
		}
		select {
		case req := <-received:
			if req.Method != row.method {
				t.Fatalf("method = %s, want %s", req.Method, row.method)
			}
			if row.params != "null" {
				var actual, expected any
				_ = json.Unmarshal(req.Params, &actual)
				_ = json.Unmarshal([]byte(row.params), &expected)
				a, _ := json.Marshal(actual)
				e, _ := json.Marshal(expected)
				if string(a) != string(e) {
					t.Fatalf("params = %s, want %s", a, e)
				}
			}
		case <-ctx.Done():
			t.Fatal("settings request did not reach the daemon")
		}
		if row.method == "settings.inspect" {
			var result struct {
				Sources map[string]string `json:"sources"`
			}
			if json.Unmarshal(decoded["result"], &result) != nil || result.Sources["relays"] != "config" {
				t.Fatalf("inspection lost setting provenance: %s", response.Envelope)
			}
		}
	}
}
