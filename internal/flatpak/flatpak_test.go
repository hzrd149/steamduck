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
	if !slices.Contains(args, "--device=input") && !slices.Contains(args, "--device=all") {
		t.Errorf("window needs /dev/input for gamepads: %q", args)
	}
	if slices.Contains(args, "--share=network") {
		t.Errorf("window must not get network: %q", args)
	}
}

func TestVersionAtLeast(t *testing.T) {
	for v, want := range map[string]bool{
		"1.15.6": true, "1.16.0": true, "1.18.3": true, "2.0": true,
		"1.15.5": false, "1.14.10": false, "1.15": false, "": false,
	} {
		if got := versionAtLeast(v, 1, 15, 6); got != want {
			t.Errorf("versionAtLeast(%q) = %v, want %v", v, got, want)
		}
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
