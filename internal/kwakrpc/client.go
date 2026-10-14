// Package kwakrpc is a client for the kwakore control protocol, version 1:
// newline-framed JSON-RPC 2.0 over the per-user Unix socket at
// $XDG_RUNTIME_DIR/kwakore/daemon.sock (see kwakore docs/control-protocol.md).
package kwakrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
)

// ProtocolVersion is the only control protocol version this client speaks.
const ProtocolVersion = 1

// maxResponse is the server's response line cap (8 MiB) plus slack.
const maxResponse = 9 << 20

// DefaultSocketPath returns $XDG_RUNTIME_DIR/kwakore/daemon.sock. Like
// kwakore itself it never falls back to a shared directory.
func DefaultSocketPath() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" || !filepath.IsAbs(dir) {
		return "", errors.New("XDG_RUNTIME_DIR is not set to an absolute path")
	}
	return filepath.Join(dir, "kwakore", "daemon.sock"), nil
}

// Error is a JSON-RPC error returned by the daemon.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("kwakore: %s (%d)", e.Message, e.Code) }

// Reason returns data.reason when the daemon attached one, such as
// "session_unavailable".
func (e *Error) Reason() string {
	var d struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(e.Data, &d)
	return d.Reason
}

// Wire error codes from the protocol's error table.
const (
	CodeNotFound    = 1002
	CodeBusy        = 1003
	CodeUnavailable = 1004
	CodeNoUpdate    = 1005
)

// IsCode reports whether err is a daemon error with the given code.
func IsCode(err error, code int) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      uint64 `json:"id"`
}

type response struct {
	ID     *uint64         `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Client sends requests over one connection. Responses arrive in request
// order, so calls are serialized. A Client is safe for concurrent use.
type Client struct {
	path    string
	noRetry bool

	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
	next atomic.Uint64
}

// New returns a client for the socket at path ("" for the default). It
// connects lazily and reconnects after a broken connection, because the
// daemon closes idle connections after 30 seconds.
func New(path string) (*Client, error) {
	if path == "" {
		p, err := DefaultSocketPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return &Client{path: path}, nil
}

// SocketPath is the socket this client dials.
func (c *Client) SocketPath() string { return c.path }

func dial(ctx context.Context, path string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	if err := checkPeer(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// checkPeer refuses a socket served by another user, as the protocol asks
// clients to do.
func checkPeer(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var cred *syscall.Ucred
	var serr error
	if err := raw.Control(func(fd uintptr) {
		cred, serr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if serr != nil {
		return serr
	}
	if int(cred.Uid) != os.Getuid() {
		return fmt.Errorf("kwakore socket is owned by uid %d, not %d", cred.Uid, os.Getuid())
	}
	return nil
}

// Call sends method with params and decodes the result into out (which may
// be nil). A failed connection is retried once on a fresh one, so a call
// after the daemon dropped an idle connection still succeeds.
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := c.call(ctx, method, params)
	var rpcErr *Error
	if err != nil && !c.noRetry && !errors.As(err, &rpcErr) && ctx.Err() == nil {
		c.closeLocked()
		raw, err = c.call(ctx, method, params)
	}
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// WithoutRetry opens an independent client that never replays an ambiguous
// request after losing its response. Forks preserve this policy.
func (c *Client) WithoutRetry() *Client { return &Client{path: c.path, noRetry: true} }

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.conn == nil {
		conn, err := dial(ctx, c.path)
		if err != nil {
			return nil, err
		}
		c.conn, c.r = conn, bufio.NewReaderSize(conn, 64<<10)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(dl)
	} else {
		_ = c.conn.SetDeadline(timeZero)
	}
	stop := context.AfterFunc(ctx, func() { _ = c.conn.SetDeadline(timePast) })
	defer stop()

	id := c.next.Add(1)
	frame, err := json.Marshal(request{JSONRPC: "2.0", Method: method, Params: params, ID: id})
	if err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(append(frame, '\n')); err != nil {
		c.closeLocked()
		return nil, err
	}
	resp, err := readFrame(c.r)
	if err != nil {
		c.closeLocked()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if resp.ID == nil || *resp.ID != id {
		c.closeLocked()
		if resp.Error != nil {
			return nil, resp.Error
		}
		return nil, errors.New("kwakore: response id mismatch")
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

func readFrame(r *bufio.Reader) (*response, error) {
	var line []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		line = append(line, chunk...)
		if len(line) > maxResponse {
			return nil, errors.New("kwakore: response frame too large")
		}
		if !isPrefix {
			break
		}
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("kwakore: bad frame: %w", err)
	}
	return &resp, nil
}

func (c *Client) closeLocked() {
	if c.conn != nil {
		c.conn.Close()
		c.conn, c.r = nil, nil
	}
}

// Close closes the connection; the client can still be used afterwards.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
	return nil
}
