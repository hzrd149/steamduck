package kwakrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeCallsNeverReplay(t *testing.T) {
	var calls atomic.Int32
	path := fakeDaemon(t, func(method string, params json.RawMessage, encoder *json.Encoder, id json.RawMessage) {
		calls.Add(1)
		// A malformed response is indistinguishable from a lost response after
		// committing a mutation. The non-replaying client must not try again.
		_ = encoder.Encode(map[string]any{"id": "wrong", "result": nil})
	})
	base, _ := New(path)
	client := base.WithoutRetry().Fork()
	defer client.Close()
	if err := client.Call(context.Background(), "napplet.install", nil, nil); err == nil {
		t.Fatal("expected transport failure")
	}
	if calls.Load() != 1 {
		t.Fatalf("mutation dispatched %d times", calls.Load())
	}
}

func TestBuiltinChildCannotSendPrivateFrames(t *testing.T) {
	for _, typ := range []string{"extension.response", "extension.request", "eval", "close"} {
		count := 0
		err := forwardBuiltinChild(strings.NewReader(`{"t":"`+typ+`"}`), func(any) error { count++; return nil })
		if err == nil || count != 0 {
			t.Fatalf("accepted private child frame %s", typ)
		}
	}
	var sent builtinFrame
	err := forwardBuiltinChild(strings.NewReader(`{"t":"rpc","id":7,"method":"nap.start","params":"null","extension":{"instance":"forged"},"request_id":"forged"}`), func(value any) error { sent = value.(builtinFrame); return nil })
	if err != nil || sent.ID != 7 || sent.Extension != nil || sent.RequestID != "" {
		t.Fatalf("unclean child frame: %#v, %v", sent, err)
	}
}

func TestBuiltinWindowHelper(t *testing.T) {
	if os.Getenv("STEAMDUCK_BUILTIN_TEST_CHILD") != "1" {
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	_ = encoder.Encode(builtinFrame{T: "rpc", ID: 1, Method: "nap.start", Params: "null"})
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var frame builtinFrame
		if json.Unmarshal(scanner.Bytes(), &frame) != nil || strings.HasPrefix(frame.T, "extension.") {
			os.Exit(2)
		}
		if frame.T == "close" {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func TestBuiltinBrokerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		decoder, encoder := json.NewDecoder(conn), json.NewEncoder(conn)
		var registration struct {
			Method string `json:"method"`
		}
		if err := decoder.Decode(&registration); err != nil {
			serverDone <- err
			return
		}
		if registration.Method != "runtime.builtin.open" {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{"outcome": "attached", "window": AttachedWindow{Instance: "store-test", Format: "napplet"}}})
		var first builtinFrame
		if err := decoder.Decode(&first); err != nil {
			serverDone <- err
			return
		}
		if first.T != "rpc" || first.Method != "nap.start" {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		_ = encoder.Encode(builtinFrame{T: "resp", ID: first.ID, Result: json.RawMessage(`1`)})
		_ = encoder.Encode(builtinFrame{T: "extension.request", RequestID: "native-1", Extension: &ExtensionRequest{Instance: "store-test", Generation: 1, Domain: "steamduck", Envelope: json.RawMessage(`{"type":"steamduck.info","id":"ui-1"}`)}})
		var answer builtinFrame
		if err := decoder.Decode(&answer); err != nil {
			serverDone <- err
			return
		}
		if answer.T != "extension.response" || answer.RequestID != "native-1" || !strings.Contains(string(answer.Envelope), `"version":1`) {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		_ = encoder.Encode(builtinFrame{T: "close"})
		// Let the child close the carrier, just like a real window.
		_, err = decoder.Token()
		if err != io.EOF {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()
	client, _ := New(path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = client.RunBuiltin(ctx, BuiltinDefinition{}, func(w AttachedWindow) (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestBuiltinWindowHelper$")
		cmd.Env = append(os.Environ(), "STEAMDUCK_BUILTIN_TEST_CHILD=1")
		return cmd, nil
	}, func(ctx context.Context, request ExtensionRequest) BuiltinResponse {
		return BuiltinResponse{Envelope: json.RawMessage(`{"type":"steamduck.info.result","id":"ui-1","result":{"version":1}}`)}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}
