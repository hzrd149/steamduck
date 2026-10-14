package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	if len(d.Extensions[0].Routes) != 3 || d.Extensions[0].Routes[1].DeadlineMS != 180000 || d.Extensions[0].Routes[1].MaxResponseBytes != 8<<20 {
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
