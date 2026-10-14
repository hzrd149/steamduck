// Package launcher runs a napplet as a Steam game: it opens the window
// through kwakore, makes gamescope show it, and stays alive until the window
// closes so Steam sees the game as running for exactly that long.
package launcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

// Options configure one launch.
type Options struct {
	Address string
	// AppID is the shortcut's 32-bit Steam app id; 0 skips window tagging.
	AppID uint32
	// Try opens the napplet without installing it.
	Try bool
	Log *slog.Logger
}

// AppIDFromEnv reads the app id Steam exports to a game it starts.
// SteamGameId holds either the 32-bit app id or the 64-bit game id whose
// upper half is the app id, depending on the client version.
func AppIDFromEnv() uint32 {
	for _, k := range []string{"SteamGameId", "SteamAppId"} {
		v, err := strconv.ParseUint(os.Getenv(k), 10, 64)
		if err != nil || v == 0 {
			continue
		}
		if v > 0xffffffff {
			v >>= 32
		}
		return uint32(v)
	}
	return 0
}

// Run launches the napplet and blocks until its window closes or ctx ends.
// When ctx ends first (Steam's "Exit game" sends SIGTERM) the window is
// closed through kwakore.
func Run(ctx context.Context, client *kwakrpc.Client, opts Options) error {
	log := opts.Log
	if err := EnsureSession(ctx, client, log); err != nil {
		log.Warn("could not update the service's display", "err", err)
	}

	// Subscribe before launching, so a window that closes at once is seen.
	subCtx, cancelSub := context.WithTimeout(ctx, 60*time.Second)
	sub, err := client.Subscribe(subCtx, "window.closed")
	cancelSub()
	if err != nil {
		log.Warn("event subscription failed; polling windows instead", "err", err)
	}

	launchCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	var res *kwakrpc.LaunchResult
	if opts.Try {
		res, err = client.Try(launchCtx, opts.Address)
	} else {
		res, err = client.Launch(launchCtx, opts.Address)
	}
	cancel()
	if err != nil {
		if sub != nil {
			sub.Close()
		}
		return explain(err)
	}
	log.Info("napplet window opened", "window", res.WindowID)

	if opts.AppID != 0 {
		go TagLoop(ctx, res.WindowID, opts.AppID, log)
	}

	err = wait(ctx, client, sub, res.WindowID)
	if ctx.Err() != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.Stop(stopCtx, res.WindowID); err != nil && !kwakrpc.IsCode(err, kwakrpc.CodeNotFound) {
			log.Warn("close napplet window", "err", err)
		}
		return nil
	}
	return err
}

func explain(err error) error {
	var rpcErr *kwakrpc.Error
	if errors.As(err, &rpcErr) && rpcErr.Reason() == "session_unavailable" {
		return fmt.Errorf("%w: the kwakore service has no display; run `steamduck doctor`", err)
	}
	return err
}

// TagLoop keeps tagging the window's X11 windows for a short while: the
// first window may be mapped late, and a napplet can open a second one.
func TagLoop(ctx context.Context, windowID string, appID uint32, log *slog.Logger) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(15 * time.Second)
	reported := false
	for {
		pids := windowPIDs("/proc", windowID)
		if len(pids) > 0 {
			n, err := TagWindows(display, pids, appID)
			if err != nil && !reported {
				log.Warn("tag window for gamescope", "err", err)
				reported = true
			} else if n > 0 && !reported {
				log.Info("window tagged for gamescope", "app_id", appID, "windows", n)
				reported = true
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			return
		case <-tick.C:
		}
	}
}

// wait returns when windowID closes. It follows the event stream and falls
// back to polling napplet.windows if the stream is unavailable or ends.
func wait(ctx context.Context, client *kwakrpc.Client, sub *kwakrpc.Subscription, windowID string) error {
	if sub != nil {
		defer sub.Close()
	loop:
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ev, ok := <-sub.C:
				if !ok {
					break loop
				}
				if ev.Type == "window.closed" && ev.WindowID == windowID {
					return nil
				}
				if ev.Type == "events.overflow" {
					break loop
				}
			}
		}
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		windows, err := client.Windows(pctx)
		cancel()
		if err != nil {
			continue // daemon restarting; try again
		}
		open := false
		for _, w := range windows {
			if w.WindowID == windowID {
				open = true
			}
		}
		if !open {
			return nil
		}
	}
}
