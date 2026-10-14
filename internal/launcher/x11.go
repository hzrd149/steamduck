package launcher

import (
	"errors"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Gamescope (SteamOS game mode) only shows windows belonging to the app
// Steam is running, which it learns from the STEAM_GAME property Steam's
// overlay sets on the game's own windows. A napplet window is started by
// the kwakore service, outside Steam's process tree, so steamduck sets the
// property itself.

// TagWindows sets STEAM_GAME=appID on every top-level X11 window whose
// _NET_WM_PID is one of pids, and returns how many it tagged.
func TagWindows(display string, pids map[int]bool, appID uint32) (int, error) {
	conn, err := xgb.NewConnDisplay(display)
	if err != nil {
		return 0, fmt.Errorf("connect to X display %q: %w", display, err)
	}
	defer conn.Close()

	pidAtom, err := atom(conn, "_NET_WM_PID")
	if err != nil {
		return 0, err
	}
	steamAtom, err := atom(conn, "STEAM_GAME")
	if err != nil {
		return 0, err
	}

	root := xproto.Setup(conn).DefaultScreen(conn).Root
	windows, err := descendants(conn, root, 3)
	if err != nil {
		return 0, err
	}
	tagged := 0
	for _, w := range windows {
		reply, err := xproto.GetProperty(conn, false, w, pidAtom, xproto.AtomCardinal, 0, 1).Reply()
		if err != nil || reply.Format != 32 || len(reply.Value) < 4 {
			continue
		}
		pid := int(xgb.Get32(reply.Value))
		if !pids[pid] {
			continue
		}
		val := make([]byte, 4)
		xgb.Put32(val, appID)
		if err := xproto.ChangePropertyChecked(conn, xproto.PropModeReplace, w, steamAtom, xproto.AtomCardinal, 32, 1, val).Check(); err != nil {
			return tagged, fmt.Errorf("set STEAM_GAME on window 0x%x: %w", w, err)
		}
		tagged++
	}
	return tagged, nil
}

func atom(conn *xgb.Conn, name string) (xproto.Atom, error) {
	r, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	if r.Atom == xproto.AtomNone {
		return 0, errors.New("no atom " + name)
	}
	return r.Atom, nil
}

// descendants lists windows below root down to depth levels; reparenting
// window managers put the client window one or two levels down.
func descendants(conn *xgb.Conn, root xproto.Window, depth int) ([]xproto.Window, error) {
	tree, err := xproto.QueryTree(conn, root).Reply()
	if err != nil {
		return nil, err
	}
	var out []xproto.Window
	for _, w := range tree.Children {
		out = append(out, w)
		if depth > 1 {
			sub, err := descendants(conn, w, depth-1)
			if err == nil {
				out = append(out, sub...)
			}
		}
	}
	return out, nil
}
