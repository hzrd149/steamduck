package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
	"github.com/hzrd149/steamduck/internal/paths"
)

// This opt-in test runs the actual embedded store and injected binding in
// WebKit with an isolated daemon. It never edits the user's daemon or Steam.
func TestRealBuiltinStore(t *testing.T) {
	if os.Getenv("STEAMDUCK_STORE_SMOKE") != "1" {
		t.Skip("set STEAMDUCK_STORE_SMOKE=1 with a built runtime and display")
	}
	bundle, err := filepath.Abs(os.Getenv("STEAMDUCK_KWAKORE_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"runtime", "config/kwakore", "data/kwakore/icons"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "runtime"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("KWAKORE_CONFIG_FILE", filepath.Join(root, "config/kwakore/config.json"))
	t.Setenv("STEAMDUCK_KWAKORE_DIR", bundle)
	t.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	t.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")
	config := []byte(`{"relays":[],"blossom_servers":[],"discover_on_user_relays":false,"desktop_entries":false,"gnome_search":false,"social_refresh":"0","signer":{"mode":"none"}}`)
	if err := os.WriteFile(os.Getenv("KWAKORE_CONFIG_FILE"), config, 0600); err != nil {
		t.Fatal(err)
	}
	var icon bytes.Buffer
	if err := png.Encode(&icon, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(icon.Bytes())
	sha := hex.EncodeToString(sum[:])
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	daemon := exec.CommandContext(ctx, filepath.Join(bundle, "kwakore"))
	var logs bytes.Buffer
	daemon.Stdout, daemon.Stderr = &logs, &logs
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = daemon.Process.Kill()
		_ = daemon.Wait()
		if t.Failed() {
			t.Log(logs.String())
		}
	}()
	client, err := kwakrpc.New("")
	if err != nil {
		t.Fatal(err)
	}
	client = client.WithoutRetry()
	defer client.Close()
	for {
		if _, err := client.Status(ctx); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("daemon startup timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Startup prunes icons unreferenced by installed napplets; seed afterwards.
	if err := os.WriteFile(filepath.Join(root, "data/kwakore/icons", sha+".blob"), icon.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	original := nappletHTML
	defer func() { nappletHTML = original }()
	// The test-owned document remains hash-approved by BuiltinDefinition. Run
	// the probe alongside the actual production app; signal via isolated settings.
	nappletHTML += `<script>(async()=>{const s=window.napplet.steamduck;let verdict="PASS",stage="info";try{
 const info=await s.info();if(info.version!==1)throw Error("info");
 stage="status";await s.call("status");stage="direct";await s.kwakore.call("service.status");
 stage="installed";const page=await s.call("installed",{offset:0,limit:2});if(!Array.isArray(page.items))throw Error("installed page");
 if(!window.napplet.gamepad||!window.napplet.theme||!window.napplet.storage)throw Error("stock domains");
 stage="resource";const blob=await window.napplet.resource.bytes("blossom:` + sha + `.png");if(!(blob instanceof Blob)||blob.type!=="image/png"||blob.size===0)throw Error("cached resource");
 try{await s.kwakore.call("napplet.icon",{});throw Error("raw icon allowed")}catch(e){if(e.code!==1100)throw e}
 }catch(e){verdict="FAIL-"+stage+"-"+String(e.message).replace(/[^a-zA-Z0-9_-]/g,"_").slice(0,45)}
 await s.call("settings.set",{field:"sync_id",value:verdict});await s.call("sync");await s.call("quit");
 })()</script>`
	done := make(chan string, 1)
	api := &API{Client: client, Stderr: os.Stderr, OnSync: func(ctx context.Context) (any, error) {
		settings, err := client.Settings(ctx)
		if err != nil {
			return nil, err
		}
		done <- settings["sync_id"].(string)
		return map[string]any{"deferred": true, "shortcut_count": 0}, nil
	}}
	err = RunBuiltin(ctx, api, paths.Config{WindowRuntime: paths.WindowRuntimeHost}, false)
	if err != nil {
		t.Fatalf("builtin run: %v", err)
	}
	select {
	case verdict := <-done:
		if verdict != "PASS" {
			t.Fatal(verdict)
		}
	case <-ctx.Done():
		t.Fatal("store probe did not finish")
	}
}
