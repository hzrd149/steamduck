package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

func TestStoreDefinitionUsesApprovedAssets(t *testing.T) {
	d := BuiltinDefinition()
	sum := sha256.Sum256([]byte(d.HTML))
	if d.SHA256 != hex.EncodeToString(sum[:]) || d.HTML != NappletPage() || d.Extensions[0].Prelude != SteamduckPrelude() {
		t.Fatal("definition does not match embedded assets")
	}
	if len(d.Extensions[0].Routes) != 3 || d.Extensions[0].Routes[1].DeadlineMS != 180000 || d.Extensions[0].Routes[1].MaxResponseBytes != 262144 {
		t.Fatal("incorrect native budgets")
	}
	if strings.Contains(d.HTML, "installSteamduckBinding") {
		t.Fatal("runtime installer bundled into untrusted application")
	}
}

func TestNativeValidationRejectsBeforeDispatch(t *testing.T) {
	client, _ := kwakrpc.New("/nonexistent/steamduck-test.sock")
	api := &API{Client: client.WithoutRetry()}
	for _, row := range []struct {
		domain, envelope string
		code             float64
	}{
		{"other", `{"type":"steamduck.request","id":"x","method":"sync"}`, 1001},
		{"steamduck", `{"type":"steamduck.request","id":"x","method":"store.unknown"}`, -32601},
		{"steamduck", `{"type":"steamduck.request","id":"x","method":"install","params":{"address":9}}`, -32602},
		{"steamduck", `{"type":"steamduck.request","id":"x","method":"permissions.clear","params":{"address":"a","permission":"p","grant":true}}`, -32602},
		{"steamduck", `{"type":"steamduck.request","id":"x","method":"sync","instance":"forged"}`, -32600},
		{"steamduck", `{"type":"steamduck.kwakore.request","id":"x","method":"runtime.builtin.open"}`, 1100},
		{"steamduck", `{"type":"steamduck.kwakore.request","id":"x","method":"events.subscribe"}`, 1100},
		{"steamduck", `{"type":"steamduck.kwakore.request","id":"x","method":"napplet.try","params":{"attach":true}}`, 1100},
	} {
		response := api.HandleNative(context.Background(), kwakrpc.ExtensionRequest{Domain: row.domain, Envelope: json.RawMessage(row.envelope)})
		var decoded struct {
			Error struct {
				Code float64 `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(response.Envelope, &decoded) != nil || decoded.Error.Code != row.code {
			t.Fatalf("%s: %s", row.envelope, response.Envelope)
		}
	}
}

func TestNativeCancellationAndQuit(t *testing.T) {
	client, _ := kwakrpc.New("/nonexistent/steamduck-test.sock")
	api := &API{Client: client.WithoutRetry()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := api.HandleNative(ctx, kwakrpc.ExtensionRequest{Domain: "steamduck", Envelope: json.RawMessage(`{"type":"steamduck.request","id":"x","method":"install","params":{"address":"a"}}`)})
	if !strings.Contains(string(response.Envelope), `"code":1007`) {
		t.Fatal(string(response.Envelope))
	}
	response = api.HandleNative(context.Background(), kwakrpc.ExtensionRequest{Domain: "steamduck", Envelope: json.RawMessage(`{"type":"steamduck.request","id":"x","method":"quit"}`)})
	if !response.Close || !strings.Contains(string(response.Envelope), `"result":null`) {
		t.Fatal(string(response.Envelope))
	}
}

// Exercise a daemon page larger than the custom transport cap. Metadata and
// install state must survive, and the continuation must point after kept items.
func TestNativeInstalledPageFitsWithoutImages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		var request struct {
			ID     json.RawMessage             `json:"id"`
			Method string                      `json:"method"`
			Params struct{ Offset, Limit int } `json:"params"`
		}
		err = json.NewDecoder(conn).Decode(&request)
		if err != nil {
			done <- err
			return
		}
		items := make([]kwakrpc.InstalledItem, 4)
		for i := range items {
			items[i].Descriptor.Description = strings.Repeat("x", 100000)
			items[i].Descriptor.Icon = &kwakrpc.IconRef{SHA256: strings.Repeat("a", 64), Mime: "image/png"}
		}
		done <- json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "result": kwakrpc.Page[kwakrpc.InstalledItem]{Items: items, Total: 14}})
	}()
	client, _ := kwakrpc.New(path)
	defer client.Close()
	api := &API{Client: client.WithoutRetry()}
	response := api.HandleNative(context.Background(), kwakrpc.ExtensionRequest{Domain: "steamduck", Envelope: json.RawMessage(`{"type":"steamduck.request","id":"page","method":"installed","params":{"offset":10,"limit":4}}`)})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var result struct {
		Result kwakrpc.Page[kwakrpc.InstalledItem] `json:"result"`
		Error  any                                 `json:"error"`
	}
	if err := json.Unmarshal(response.Envelope, &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != nil || len(response.Envelope) > nativeResponseLimit || len(result.Result.Items) != 2 || result.Result.Total != 14 || result.Result.NextOffset == nil || *result.Result.NextOffset != 12 {
		t.Fatalf("incorrect bounded page: size=%d, items=%d, next=%v, error=%v", len(response.Envelope), len(result.Result.Items), result.Result.NextOffset, result.Error)
	}
	icon := result.Result.Items[0].Descriptor.Icon
	if icon.Resource == nil || icon.Resource.URL != "blossom:"+icon.SHA256+".png" || strings.Contains(string(response.Envelope), "data:image") {
		t.Fatal("missing resource reference or embedded image")
	}
}

func TestProfileResourceReferences(t *testing.T) {
	for _, raw := range []string{"data:image/png;base64,AAAA", "http://example.com/a.png", "file:///etc/passwd", "https://user:pass@example.com/a.png", strings.Repeat("x", 2049)} {
		if profileResource(raw) != nil {
			t.Fatalf("accepted unsafe reference: %q", raw)
		}
	}
	ref := profileResource(" https://example.com/avatar.png ")
	if ref == nil || ref.URL != "https://example.com/avatar.png" {
		t.Fatal("lost profile reference")
	}
}

func TestIconResourceUsesBUD10(t *testing.T) {
	hash, author := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, row := range []struct{ mime, extension string }{{"image/png", "png"}, {"image/jpeg", "jpg"}, {"image/webp", "webp"}, {"application/octet-stream", "bin"}} {
		d := kwakrpc.Descriptor{Address: "35129:" + author + ":app", Icon: &kwakrpc.IconRef{SHA256: hash, Mime: row.mime}}
		ref := d.IconResource()
		if ref == nil || ref.URL != "blossom:"+hash+"."+row.extension+"?as="+author {
			t.Fatalf("invalid BUD-10 icon ref: %#v", ref)
		}
	}
}
