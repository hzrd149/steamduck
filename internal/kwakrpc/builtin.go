package kwakrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Native built-in protocol DTOs. Kept independent of Kwakore's backend Go
// module so Steamduck remains a portable, cgo-free control client.
type BuiltinDefinition struct {
	Key        string            `json:"key"`
	HTML       string            `json:"html"`
	SHA256     string            `json:"sha256"`
	Name       string            `json:"name"`
	Width      int               `json:"width,omitempty"`
	Height     int               `json:"height,omitempty"`
	Domains    []string          `json:"domains"`
	Extensions []ExtensionDomain `json:"extensions"`
}
type ExtensionDomain struct {
	Name    string           `json:"name"`
	Version string           `json:"version"`
	Prelude string           `json:"prelude"`
	Routes  []ExtensionRoute `json:"routes"`
}
type ExtensionRoute struct {
	Type              string `json:"type"`
	MaxRequestBytes   int    `json:"max_request_bytes"`
	MaxResponseBytes  int    `json:"max_response_bytes"`
	Concurrency       int    `json:"concurrency"`
	RequestsPerSecond int    `json:"requests_per_second"`
	DeadlineMS        int    `json:"deadline_ms"`
}
type ExtensionRequest struct {
	Instance   string          `json:"instance"`
	Generation int             `json:"generation"`
	Domain     string          `json:"domain"`
	Envelope   json.RawMessage `json:"envelope"`
}
type BuiltinResponse struct {
	Envelope json.RawMessage
	Close    bool
}
type BuiltinHandler func(context.Context, ExtensionRequest) BuiltinResponse

type builtinFrame struct {
	T         string            `json:"t"`
	ID        int               `json:"id,omitempty"`
	Method    string            `json:"method,omitempty"`
	Params    string            `json:"params,omitempty"`
	Result    json.RawMessage   `json:"result,omitempty"`
	Error     string            `json:"error,omitempty"`
	Code      string            `json:"code,omitempty"`
	Idx       *int              `json:"idx,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Extension *ExtensionRequest `json:"extension,omitempty"`
	Envelope  json.RawMessage   `json:"envelope,omitempty"`
}

// RunBuiltin registers approved native bytes and proxies the stock window.
// No request is retried. The window gets only its ordinary wire protocol;
// private requests stay between the daemon and this native process.
func (c *Client) RunBuiltin(ctx context.Context, definition BuiltinDefinition, command func(AttachedWindow) (*exec.Cmd, error), handler BuiltinHandler) error {
	conn, err := dial(ctx, c.path)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	params, err := json.Marshal(definition)
	if err != nil || len(params) > 900<<10 {
		return errors.New("built-in definition exceeds the registration limit")
	}
	frame, err := json.Marshal(request{JSONRPC: "2.0", Method: "runtime.builtin.open", Params: definition, ID: 1})
	if err != nil || len(frame) > 1<<20 {
		return errors.New("built-in request exceeds the registration limit")
	}
	var writeMu sync.Mutex
	send := func(value any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return err
		}
		return json.NewEncoder(conn).Encode(value)
	}
	_ = conn.SetReadDeadline(time.Now().Add(12 * time.Second))
	if _, err := conn.Write(append(frame, '\n')); err != nil {
		return err
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64<<10), 25<<20)
	if !scanner.Scan() {
		return errors.New("kwakore did not acknowledge the built-in store")
	}
	var ack struct {
		ID     uint64 `json:"id"`
		Result *struct {
			Outcome string          `json:"outcome"`
			Window  *AttachedWindow `json:"window"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	if json.Unmarshal(scanner.Bytes(), &ack) != nil || ack.ID != 1 {
		return errors.New("invalid built-in acknowledgement")
	}
	if ack.Error != nil {
		return fmt.Errorf("kwakore cannot open the built-in store: %w; update kwakore and restart its daemon", ack.Error)
	}
	if ack.Result == nil || ack.Result.Outcome != "attached" || ack.Result.Window == nil || ack.Result.Window.Instance == "" || ack.Result.Window.Format != "napplet" {
		return errors.New("invalid built-in window descriptor")
	}
	_ = conn.SetReadDeadline(timeZero)
	w := *ack.Result.Window
	cmd, err := command(w)
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return err
	}
	defer func() { stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	childDone := make(chan error, 1)
	go func() { childDone <- forwardBuiltinChild(stdout, send); conn.Close() }()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var pendingMu sync.Mutex
	pending := map[string]context.CancelFunc{}
	defer func() {
		pendingMu.Lock()
		defer pendingMu.Unlock()
		for _, cancel := range pending {
			cancel()
		}
	}()
	closing := false
	for scanner.Scan() {
		var msg builtinFrame
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			return errors.New("invalid built-in daemon frame")
		}
		if closing && msg.T != "extension.cancel" {
			continue
		}
		switch msg.T {
		case "extension.request":
			if msg.RequestID == "" || msg.Extension == nil || msg.Extension.Instance != w.Instance || msg.Extension.Generation < 1 {
				return errors.New("invalid native extension ownership")
			}
			pendingMu.Lock()
			_, exists := pending[msg.RequestID]
			if exists || len(pending) >= 64 {
				pendingMu.Unlock()
				return errors.New("native extension pending limit exceeded")
			}
			callCtx, callCancel := context.WithCancel(runCtx)
			pending[msg.RequestID] = callCancel
			pendingMu.Unlock()
			go func(msg builtinFrame) {
				defer callCancel()
				defer func() { pendingMu.Lock(); delete(pending, msg.RequestID); pendingMu.Unlock() }()
				response := invokeBuiltin(callCtx, handler, *msg.Extension)
				if callCtx.Err() != nil {
					return
				}
				if err := send(builtinFrame{T: "extension.response", RequestID: msg.RequestID, Envelope: response.Envelope}); err != nil {
					conn.Close()
					return
				}
				if response.Close {
					closeCtx, cancel := context.WithTimeout(runCtx, 5*time.Second)
					defer cancel()
					client := c.WithoutRetry()
					defer client.Close()
					_ = client.Stop(closeCtx, w.Instance)
				}
			}(msg)
		case "extension.cancel":
			pendingMu.Lock()
			cancel := pending[msg.RequestID]
			pendingMu.Unlock()
			if cancel != nil {
				cancel()
			}
		default:
			if strings.HasPrefix(msg.T, "extension.") {
				return errors.New("unknown private broker frame")
			}
			if msg.T == "close" {
				closing = true
				pendingMu.Lock()
				for _, cancel := range pending {
					cancel()
				}
				pendingMu.Unlock()
			}
			if err := json.NewEncoder(stdin).Encode(msg); err != nil {
				if closing {
					return nil
				}
				return err
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case err := <-childDone:
		return err
	default:
		return scanner.Err()
	}
}

func invokeBuiltin(ctx context.Context, handler BuiltinHandler, request ExtensionRequest) (response BuiltinResponse) {
	defer func() {
		if recover() != nil {
			var head struct {
				Type string          `json:"type"`
				ID   json.RawMessage `json:"id"`
			}
			_ = json.Unmarshal(request.Envelope, &head)
			response.Envelope, _ = json.Marshal(map[string]any{"type": head.Type + ".result", "id": head.ID, "error": map[string]any{"code": -32603, "message": "Internal error"}})
		}
	}()
	if ctx.Err() != nil {
		return response
	}
	return handler(ctx, request)
}

func forwardBuiltinChild(reader io.Reader, send func(any) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 25<<20)
	for scanner.Scan() {
		var msg builtinFrame
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			return errors.New("invalid window frame")
		}
		switch msg.T {
		case "rpc", "promptAnswer", "windowFailed":
		default:
			return errors.New("window attempted to send a private broker frame")
		}
		if err := send(builtinFrame{T: msg.T, ID: msg.ID, Method: msg.Method, Params: msg.Params, Code: msg.Code}); err != nil {
			return err
		}
	}
	return scanner.Err()
}
