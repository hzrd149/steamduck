// Package store is the napplet store app: a controller-friendly web UI,
// shown in a webview, that drives kwakore over its control socket.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
	"github.com/hzrd149/steamduck/internal/launcher"
)

// API is what the UI may call. Every method takes and returns JSON values,
// so the same table serves the webview binding and the development HTTP
// server.
type API struct {
	Client *kwakrpc.Client
	Log    *slog.Logger
	// AppID is the store's own Steam app id. Napplets launched from the
	// store are tagged with it so gamescope shows them over the store.
	AppID uint32
	// OnSync, when set, asks for a Steam shortcut sync and reports it.
	OnSync func(ctx context.Context) (any, error)
	// Quit closes the store window.
	Quit func()
	// Stderr receives the output of napplet windows the store starts.
	Stderr *os.File
	// StartWindow, when set, opens napplet windows instead of the default
	// launch paths (the store's Flatpak sandbox sets it).
	StartWindow func(ctx context.Context, address string, try bool) error

	iconMu sync.Mutex
	icons  map[string]string // address -> data URL ("" when none)

	profiles profileCache
}

type addressParams struct {
	Address string `json:"address"`
}

type callClientKey struct{}

func (a *API) callClient(ctx context.Context) *kwakrpc.Client {
	if client, ok := ctx.Value(callClientKey{}).(*kwakrpc.Client); ok {
		return client
	}
	return a.Client
}

// Call dispatches one UI request.
func (a *API) Call(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client := a.Client.Fork()
	defer client.Close()
	ctx = context.WithValue(ctx, callClientKey{}, client)
	var p addressParams
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &p)
	}
	switch method {
	case "status":
		return a.callClient(ctx).Status(ctx)
	case "discover":
		var dp kwakrpc.DiscoverParams
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &dp); err != nil {
				return nil, err
			}
		}
		if dp.Limit == 0 {
			dp.Limit = 60
		}
		if dp.Refresh {
			ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			return a.callClient(ctx).Discover(ctx, dp)
		}
		return a.callClient(ctx).Discover(ctx, dp)
	case "installed":
		return a.callClient(ctx).Installed(ctx)
	case "detail":
		return a.callClient(ctx).Detail(ctx, p.Address)
	case "icon":
		return a.icon(ctx, p.Address), nil
	// Installs and updates get their own connection, so browsing goes on
	// while they download.
	case "install":
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		c := a.callClient(ctx).Fork()
		defer c.Close()
		return c.Install(ctx, p.Address)
	case "update":
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		c := a.callClient(ctx).Fork()
		defer c.Close()
		return nil, c.Update(ctx, p.Address)
	case "uninstall":
		return nil, a.callClient(ctx).Uninstall(ctx, p.Address)
	case "launch", "try":
		return a.launch(ctx, method == "try", p.Address)
	case "sync":
		if a.OnSync == nil {
			return nil, errors.New("shortcut sync is not available")
		}
		return a.OnSync(ctx)
	case "copy":
		var cp struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &cp); err != nil {
			return nil, err
		}
		return nil, copyText(ctx, cp.Text)
	case "quit":
		if a.Quit != nil {
			a.Quit()
		}
		return nil, nil
	}
	if res, ok, err := a.callAccount(ctx, method, raw); ok {
		return res, err
	}
	if res, ok, err := a.callSocial(ctx, method, raw); ok {
		return res, err
	}
	return nil, fmt.Errorf("unknown method %q", method)
}

func (a *API) launch(ctx context.Context, try bool, address string) (any, error) {
	if a.StartWindow != nil {
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		if err := a.StartWindow(ctx, address, try); err != nil {
			return nil, err
		}
		return &kwakrpc.LaunchResult{Address: address, Outcome: "opened"}, nil
	}
	// Preferred: run the window as our own child (kwakore attached launch),
	// so it stays in the store's Steam session.
	if !try && launcher.SupportsRun() {
		if err := launcher.StartRun(address, a.Stderr); err != nil {
			return nil, err
		}
		return &kwakrpc.LaunchResult{Address: address, Outcome: "opened"}, nil
	}
	if err := launcher.EnsureSession(ctx, a.callClient(ctx), a.Log); err != nil {
		a.Log.Warn("update the service's display", "err", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var res *kwakrpc.LaunchResult
	var err error
	if try {
		res, err = a.callClient(ctx).Try(ctx, address)
	} else {
		res, err = a.callClient(ctx).Launch(ctx, address)
	}
	if err != nil {
		return nil, err
	}
	if a.AppID != 0 {
		go launcher.TagLoop(context.Background(), res.WindowID, a.AppID, a.Log)
	}
	return res, nil
}

// icon returns a napplet's icon as a data URL, cached for the session.
func (a *API) icon(ctx context.Context, address string) string {
	a.iconMu.Lock()
	if a.icons == nil {
		a.icons = map[string]string{}
	}
	if v, ok := a.icons[address]; ok {
		a.iconMu.Unlock()
		return v
	}
	a.iconMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	url := ""
	if icon, err := a.callClient(ctx).Icon(ctx, address); err == nil {
		url = "data:image/png;base64," + icon.Data
	}
	a.iconMu.Lock()
	a.icons[address] = url
	a.iconMu.Unlock()
	return url
}

// ErrorJSON turns an error into what the UI shows: the daemon's fixed
// message and code, or the error text.
func ErrorJSON(err error) map[string]any {
	var rpcErr *kwakrpc.Error
	if errors.As(err, &rpcErr) {
		return map[string]any{"code": rpcErr.Code, "message": rpcErr.Message, "reason": rpcErr.Reason()}
	}
	return map[string]any{"message": err.Error()}
}
