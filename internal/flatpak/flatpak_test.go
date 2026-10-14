package flatpak

import (
	"slices"
	"testing"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
	"github.com/hzrd149/steamduck/internal/paths"
)

func TestWindowArgs(t *testing.T) {
	w := kwakrpc.AttachedWindow{NappID: "n", Name: "Big\nRig\x00Rush", Instance: "abc", Width: 800, Height: 600, Format: "napplet"}
	args := WindowArgs("org.gnome.Platform//50", "/home/deck/.local/lib/kwakore/releases/x", w)
	want := []string{
		"run",
		"--command=/home/deck/.local/lib/kwakore/releases/x/kwaklet",
		"--filesystem=/home/deck/.local/lib/kwakore/releases/x:ro",
		"--unshare=network",
		"--env=WEBKIT_DISABLE_DMABUF_RENDERER=1",
		"--env=KWAKORE_INSTANCE_ID=abc",
		"--env=KWAKORE_NAPP_NAME=Big Rig Rush",
		"--env=WEBVIEW_PATH=/home/deck/.local/lib/kwakore/releases/x",
	}
	for _, a := range want {
		if !slices.Contains(args, a) {
			t.Errorf("missing %q in %q", a, args)
		}
	}
	if args[len(args)-1] != "org.gnome.Platform//50" {
		t.Errorf("runtime must come last: %q", args)
	}
	if slices.Contains(args, "--share=network") {
		t.Errorf("window must not get network: %q", args)
	}
}

func TestStoreArgs(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	args := StoreArgs("rt//1", "/home/deck/.local/lib/steamduck/steamduck", "/rel", "/run/user/1000/kwakore/daemon.sock", []string{"--debug"})
	for _, a := range []string{
		"--command=/home/deck/.local/lib/steamduck/steamduck",
		"--filesystem=xdg-run/kwakore",
		"--talk-name=org.freedesktop.Flatpak",
		"--env=STEAMDUCK_IN_FLATPAK=1",
		"--env=STEAMDUCK_KWAKORE_SOCKET=/run/user/1000/kwakore/daemon.sock",
	} {
		if !slices.Contains(args, a) {
			t.Errorf("missing %q in %q", a, args)
		}
	}
	tail := args[len(args)-3:]
	if !slices.Equal(tail, []string{"rt//1", "store", "--debug"}) {
		t.Errorf("tail = %q", tail)
	}
	// a socket outside the runtime dir (testing) is exposed by path
	if got := socketDir("/tmp/kwk/kwakore/daemon.sock"); got != "/tmp/kwk/kwakore" {
		t.Errorf("socketDir = %q", got)
	}
}

func TestUse(t *testing.T) {
	if !Use(paths.Config{WindowRuntime: paths.WindowRuntimeFlatpak}) {
		t.Error("flatpak mode")
	}
	if Use(paths.Config{WindowRuntime: paths.WindowRuntimeHost}) {
		t.Error("host mode")
	}
}
