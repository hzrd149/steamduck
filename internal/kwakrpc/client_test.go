package kwakrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDaemon serves one canned handler per method on a temporary socket.
func fakeDaemon(t *testing.T, handle func(method string, params json.RawMessage, enc *json.Encoder, id json.RawMessage)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "daemon.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				enc := json.NewEncoder(conn)
				for sc.Scan() {
					var req struct {
						Method string          `json:"method"`
						Params json.RawMessage `json:"params"`
						ID     json.RawMessage `json:"id"`
					}
					if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
						return
					}
					handle(req.Method, req.Params, enc, req.ID)
				}
			}()
		}
	}()
	return path
}

func reply(enc *json.Encoder, id json.RawMessage, result any) {
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func TestCallAndPagination(t *testing.T) {
	path := fakeDaemon(t, func(method string, params json.RawMessage, enc *json.Encoder, id json.RawMessage) {
		switch method {
		case "service.status":
			reply(enc, id, map[string]any{"protocol_version": 1, "health": map[string]any{"ready": true, "version": "test"}})
		case "napplet.installed":
			var p struct{ Offset int }
			_ = json.Unmarshal(params, &p)
			if p.Offset == 0 {
				reply(enc, id, map[string]any{"items": []any{map[string]any{"address": "a", "archetypes": []string{"game"}}}, "total": 2, "next_offset": 1})
			} else {
				reply(enc, id, map[string]any{"items": []any{map[string]any{"address": "b"}}, "total": 2, "next_offset": nil})
			}
		case "napplet.launch":
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": 1004, "message": "Unavailable", "data": map[string]string{"reason": "session_unavailable"}}})
		}
	})
	c, _ := New(path)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	st, err := c.Status(ctx)
	if err != nil || st.ProtocolVersion != 1 || !st.Health.Ready {
		t.Fatalf("status = %+v, %v", st, err)
	}
	items, err := c.Installed(ctx)
	if err != nil || len(items) != 2 || items[1].Address != "b" || !items[0].IsGame() {
		t.Fatalf("installed = %+v, %v", items, err)
	}
	_, err = c.Launch(ctx, "a")
	var rpcErr *Error
	if !IsCode(err, CodeUnavailable) || !asErr(err, &rpcErr) || rpcErr.Reason() != "session_unavailable" {
		t.Fatalf("launch error = %v", err)
	}
}

func asErr(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}

func TestSubscribe(t *testing.T) {
	path := fakeDaemon(t, func(method string, params json.RawMessage, enc *json.Encoder, id json.RawMessage) {
		if method != "events.subscribe" {
			return
		}
		reply(enc, id, map[string]any{"subscribed": true, "types": []string{"window.closed"}})
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": "window.closed", "time": "2026-10-12T09:00:00Z", "window_id": "w1", "address": "x"}})
	})
	c, _ := New(path)
	sub, err := c.Subscribe(context.Background(), "window.closed")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	select {
	case ev := <-sub.C:
		if ev.Type != "window.closed" || ev.WindowID != "w1" {
			t.Fatalf("event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}
}

func TestLaunchAttached(t *testing.T) {
	path := fakeDaemon(t, func(method string, params json.RawMessage, enc *json.Encoder, id json.RawMessage) {
		var p struct {
			Address string `json:"address"`
			Attach  bool   `json:"attach"`
		}
		_ = json.Unmarshal(params, &p)
		if method != "napplet.launch" || !p.Attach {
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32602, "message": "Invalid params"}})
			return
		}
		reply(enc, id, map[string]any{"address": p.Address, "outcome": "attached", "window_id": "w1",
			"window": map[string]any{"napp_id": "n", "name": "Game", "instance_id": "w1", "width": 800, "height": 600, "format": "napplet"}})
		// the first frame for the window, right behind the reply
		_ = enc.Encode(map[string]any{"t": "eval", "code": "1"})
	})
	c, _ := New(path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w, conn, err := c.LaunchAttached(ctx, "35129:aa:game", false)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if w.Instance != "w1" || w.Width != 800 {
		t.Fatalf("window = %+v", w)
	}
	// the window's frame must still be on the connection for the window program
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.Contains(line, `"eval"`) {
		t.Fatalf("window frame = %q, %v", line, err)
	}
	if _, _, err := c.LaunchAttached(ctx, "35129:aa:game", true); !IsCode(err, -32602) {
		t.Fatalf("try on a daemon without attach: %v", err)
	}
}
