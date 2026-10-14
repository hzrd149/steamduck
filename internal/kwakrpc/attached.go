package kwakrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
)

// AttachedWindow is what an attached launch returns: the values kwakore
// would otherwise put in the window program's KWAKORE_* environment.
type AttachedWindow struct {
	NappID    string `json:"napp_id"`
	Name      string `json:"name"`
	Instance  string `json:"instance_id"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Format    string `json:"format"`
	Theme     string `json:"theme"`
	ThemeVars string `json:"theme_vars"`
}

// Env is the window program's environment for this window, without
// WEBVIEW_PATH.
func (w AttachedWindow) Env() []string {
	return []string{
		"KWAKORE_NAPP_ID=" + w.NappID,
		"KWAKORE_NAPP_NAME=" + w.Name,
		"KWAKORE_INSTANCE_ID=" + w.Instance,
		"KWAKORE_WINDOW_WIDTH=" + strconv.Itoa(w.Width),
		"KWAKORE_WINDOW_HEIGHT=" + strconv.Itoa(w.Height),
		"KWAKORE_NAPP_FORMAT=" + w.Format,
		"KWAKORE_THEME=" + w.Theme,
		"KWAKORE_THEME_VARS=" + w.ThemeVars,
	}
}

// LaunchAttached asks kwakore to prepare a window without starting a
// process for it (napplet.launch, or napplet.try, with attach:true). On
// success the returned connection carries the window's wire messages and
// belongs to the window program, which must get it as stdin and stdout.
func (c *Client) LaunchAttached(ctx context.Context, address string, try bool) (*AttachedWindow, *net.UnixConn, error) {
	conn, err := dial(ctx, c.path)
	if err != nil {
		return nil, nil, err
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return nil, nil, errors.New("kwakore socket is not a Unix socket")
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = uc.SetDeadline(dl)
	}
	method := "napplet.launch"
	if try {
		method = "napplet.try"
	}
	frame, _ := json.Marshal(request{JSONRPC: "2.0", Method: method,
		Params: map[string]any{"address": address, "attach": true}, ID: 1})
	if _, err := uc.Write(append(frame, '\n')); err != nil {
		uc.Close()
		return nil, nil, err
	}
	line, err := readLineUnbuffered(uc, maxResponse)
	if err != nil {
		uc.Close()
		return nil, nil, err
	}
	var resp struct {
		Result *struct {
			Outcome  string          `json:"outcome"`
			WindowID string          `json:"window_id"`
			Window   *AttachedWindow `json:"window"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		uc.Close()
		return nil, nil, fmt.Errorf("kwakore: bad frame: %w", err)
	}
	if resp.Error != nil {
		uc.Close()
		return nil, nil, resp.Error
	}
	r := resp.Result
	if r == nil || r.Outcome != "attached" || r.Window == nil || r.Window.Instance != r.WindowID || r.WindowID == "" {
		uc.Close()
		return nil, nil, errors.New("kwakore: unexpected attached launch reply (kwakore too old?)")
	}
	_ = uc.SetDeadline(timeZero)
	return r.Window, uc, nil
}

// readLineUnbuffered reads up to and excluding '\n' one byte at a time, so
// nothing after the line is consumed from the connection.
func readLineUnbuffered(conn net.Conn, max int) ([]byte, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := conn.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return line, nil
			}
			line = append(line, b[0])
			if len(line) > max {
				return nil, errors.New("kwakore: response frame too large")
			}
		}
		if err != nil {
			return nil, err
		}
	}
}
