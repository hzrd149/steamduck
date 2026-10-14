package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

const nativeResponseLimit = 262144

// BuiltinDefinition contains only the approved assets embedded in this binary.
// No manifest, UI parameter or remote document can supply an extension prelude.
func BuiltinDefinition() kwakrpc.BuiltinDefinition {
	html := NappletPage()
	sum := sha256.Sum256([]byte(html))
	routes := []kwakrpc.ExtensionRoute{}
	for _, typ := range []string{"steamduck.info", "steamduck.request", "steamduck.kwakore.request"} {
		deadline := 180000
		if typ == "steamduck.info" {
			deadline = 30000
		}
		routes = append(routes, kwakrpc.ExtensionRoute{Type: typ, MaxRequestBytes: 262144, MaxResponseBytes: nativeResponseLimit, Concurrency: 16, RequestsPerSecond: 50, DeadlineMS: deadline})
	}
	return kwakrpc.BuiltinDefinition{Key: "org.steamduck.store", HTML: html, SHA256: hex.EncodeToString(sum[:]), Name: "Steamduck Store", Width: 1280, Height: 800, Domains: []string{"gamepad", "theme", "storage", "resource"}, Extensions: []kwakrpc.ExtensionDomain{{Name: "steamduck", Version: "1", Prelude: SteamduckPrelude(), Routes: routes}}}
}

type nativeInput struct {
	Type   string          `json:"type"`
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}
type paramRule struct {
	required string
	fields   string
}

// Rules are independent of the daemon method catalog. Store methods never
// fall through to arbitrary RPC. Direct control calls have their own route.
var nativeMethods = map[string]paramRule{
	"status": {}, "installed": {fields: "offset:i limit:i"}, "sync": {}, "quit": {},
	"discover": {fields: "query:s refresh:b offset:i limit:i archetype:s compatible:b"},
	"detail":   {required: "address", fields: "address:s"}, "icon": {required: "address", fields: "address:s"},
	"install": {required: "address", fields: "address:s"}, "update": {required: "address", fields: "address:s"},
	"uninstall": {required: "address", fields: "address:s"}, "launch": {required: "address", fields: "address:s"}, "try": {required: "address", fields: "address:s"},
	"copy":          {required: "text", fields: "text:s"},
	"signer.status": {}, "signer.pair.start": {}, "signer.pair.wait": {}, "signer.pair.cancel": {},
	"signer.switch":     {required: "mode", fields: "mode:s secret:s socket:s"},
	"profile.get":       {fields: "refresh:b"},
	"permissions.get":   {required: "address", fields: "address:s"},
	"permissions.clear": {required: "address permission", fields: "address:s permission:s subject:s decision:s"},
	"settings.get":      {}, "settings.set": {required: "field value", fields: "field:s value:j"},
	"settings.inspect": {}, "settings.reload": {}, "settings.clear": {required: "field", fields: "field:s"},
	"napplet.social":    {required: "address", fields: "address:s refresh:b categories:a"},
	"napplet.reactions": {required: "address", fields: "address:s event_id:s offset:i limit:i"},
	"napplet.comments":  {required: "address", fields: "address:s parent_id:s offset:i limit:i"},
	"napplet.mentions":  {required: "address", fields: "address:s offset:i limit:i"},
	"napplet.zaps":      {required: "address", fields: "address:s offset:i limit:i"},
	"napplet.vote":      {required: "address vote", fields: "address:s vote:s event_id:s"},
	"napplet.react":     {required: "address emoji", fields: "address:s emoji:s emoji_url:s event_id:s"},
	"napplet.comment":   {required: "address content", fields: "address:s content:s parent_id:s"},
	"napplet.delete":    {required: "address event_id", fields: "address:s event_id:s"},
	"napplet.share":     {required: "address", fields: "address:s content:s"},
	"napplet.zap":       {required: "address amount_msats", fields: "address:s amount_msats:i comment:s event_id:s"},
}

func validateNativeParams(rule paramRule, raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(`{}`)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	allowed := map[string]string{}
	for _, field := range strings.Fields(rule.fields) {
		kv := strings.Split(field, ":")
		allowed[kv[0]] = kv[1]
	}
	for _, required := range strings.Fields(rule.required) {
		if _, ok := fields[required]; !ok {
			return false
		}
	}
	for name, value := range fields {
		kind, ok := allowed[name]
		if !ok || string(value) == "null" {
			return false
		}
		switch kind {
		case "s":
			var s string
			if json.Unmarshal(value, &s) != nil {
				return false
			}
		case "b":
			var b bool
			if json.Unmarshal(value, &b) != nil {
				return false
			}
		case "i":
			var n int64
			if json.Unmarshal(value, &n) != nil {
				return false
			}
		case "a":
			var a []string
			if json.Unmarshal(value, &a) != nil {
				return false
			}
		}
	}
	return true
}

func nativeError(err error) map[string]any {
	var rpc *kwakrpc.Error
	if errors.As(err, &rpc) {
		out := map[string]any{"code": rpc.Code, "message": rpc.Message}
		if len(rpc.Data) > 0 && string(rpc.Data) != "null" {
			out["data"] = rpc.Data
		}
		if reason := rpc.Reason(); reason != "" {
			out["reason"] = reason
		}
		return out
	}
	code, message := 1004, "Unavailable"
	if errors.Is(err, context.DeadlineExceeded) {
		code, message = 1008, "Timeout"
	}
	if errors.Is(err, context.Canceled) {
		code, message = 1007, "Closing"
	}
	return map[string]any{"code": code, "message": message}
}

// HandleNative runs only on the native broker lane bound to the store instance.
// Kwakore authenticates ownership/generation; this adapter validates the domain,
// envelope and method before touching any native API.
func (a *API) HandleNative(ctx context.Context, request kwakrpc.ExtensionRequest) kwakrpc.BuiltinResponse {
	var input nativeInput
	decoder := json.NewDecoder(bytes.NewReader(request.Envelope))
	decoder.DisallowUnknownFields()
	decodeErr := decoder.Decode(&input)
	var trailing any
	end := decoder.Decode(&trailing)
	var id string
	validID := json.Unmarshal(input.ID, &id) == nil && id != "" && len(id) <= 128
	response := map[string]any{"type": input.Type + ".result", "id": input.ID}
	fail := func(code int, message string) { response["error"] = map[string]any{"code": code, "message": message} }
	closeWindow := false
	if request.Domain != "steamduck" {
		fail(1001, "Unauthorized")
	} else if decodeErr != nil || end != io.EOF || !validID {
		fail(-32600, "Invalid Request")
	} else if err := ctx.Err(); err != nil {
		response["error"] = nativeError(err)
	} else {
		deadline := 30 * time.Second
		if input.Type == "steamduck.kwakore.request" || input.Method == "install" || input.Method == "update" {
			deadline = 180 * time.Second
		}
		if input.Method == "signer.pair.wait" {
			deadline = 135 * time.Second
		}
		if input.Method == "launch" || input.Method == "try" {
			deadline = 90 * time.Second
		}
		if d, ok := socialMethods[input.Method]; ok {
			deadline = d
		}
		if input.Method == "discover" {
			deadline = 45 * time.Second
		}
		callCtx, cancel := context.WithTimeout(ctx, deadline)
		defer cancel()
		var result any
		var err error
		switch input.Type {
		case "steamduck.info":
			if input.Method != "" || len(input.Params) != 0 {
				fail(-32602, "Invalid params")
				break
			}
			methods := []string{}
			for method := range nativeMethods {
				methods = append(methods, method)
			}
			sort.Strings(methods)
			var protocol *int
			statusCtx, stop := context.WithTimeout(callCtx, 3*time.Second)
			client := a.Client.Fork()
			defer client.Close()
			if status, statusErr := client.Status(statusCtx); statusErr == nil {
				protocol = &status.ProtocolVersion
			}
			stop()
			result = map[string]any{"version": 1, "methods": methods, "kwakore_protocol_version": protocol}
		case "steamduck.request":
			rule, ok := nativeMethods[input.Method]
			if !ok {
				fail(-32601, "Method not found")
				break
			}
			if !validateNativeParams(rule, input.Params) {
				fail(-32602, "Invalid params")
				break
			}
			if input.Method == "quit" {
				closeWindow = true
				result = nil
				break
			}
			result, err = a.Call(callCtx, input.Method, input.Params)
		case "steamduck.kwakore.request":
			var fields map[string]json.RawMessage
			if input.Method == "" || len(input.Params) != 0 && string(input.Params) != "null" && (json.Unmarshal(input.Params, &fields) != nil || fields == nil) {
				fail(-32602, "Invalid params")
				break
			}
			if input.Method == "events.subscribe" || input.Method == "napplet.icon" || strings.HasPrefix(input.Method, "runtime.") ||
				(input.Method == "napplet.launch" || input.Method == "napplet.try") && string(fields["attach"]) == "true" {
				fail(1100, "Unsupported operation")
				break
			}
			client := a.Client.Fork()
			defer client.Close()
			var raw json.RawMessage
			var params any = input.Params
			if len(input.Params) == 0 || string(input.Params) == "null" {
				params = nil // Omit absent params; the daemon rejects literal null.
			}
			err = client.Call(callCtx, input.Method, params, &raw)
			result = raw
		default:
			fail(-32601, "Method not found")
		}
		if err != nil {
			response["error"] = nativeError(err)
		}
		if _, failed := response["error"]; !failed {
			addIconReferences(result)
			response["result"] = result
		}
	}
	raw, err := json.Marshal(response)
	if len(raw) > nativeResponseLimit && input.Type == "steamduck.request" {
		// Return fewer records with a correct continuation offset, never a
		// silently truncated complete list. Other metadata retains its schema.
		encoded, _ := json.Marshal(response["result"])
		var page map[string]json.RawMessage
		var items []json.RawMessage
		if json.Unmarshal(encoded, &page) == nil && json.Unmarshal(page["items"], &items) == nil {
			var params struct {
				Offset int `json:"offset"`
			}
			_ = json.Unmarshal(input.Params, &params)
			for len(raw) > nativeResponseLimit && len(items) > 1 {
				items = items[:len(items)-1]
				page["items"], _ = json.Marshal(items)
				page["next_offset"], _ = json.Marshal(params.Offset + len(items))
				encoded, _ = json.Marshal(page)
				response["result"] = json.RawMessage(encoded)
				raw, err = json.Marshal(response)
			}
		}
	}
	if err != nil || len(raw) > nativeResponseLimit {
		delete(response, "result")
		fail(-32603, "Internal error")
		raw, _ = json.Marshal(response)
	}
	return kwakrpc.BuiltinResponse{Envelope: raw, Close: closeWindow}
}

func addIconReferences(result any) {
	add := func(d *kwakrpc.Descriptor) {
		if d.Icon != nil {
			d.Icon.Resource = d.IconResource()
		}
	}
	switch value := result.(type) {
	case *kwakrpc.DiscoverPage:
		for i := range value.Items {
			add(&value.Items[i])
		}
	case *kwakrpc.Page[kwakrpc.InstalledItem]:
		for i := range value.Items {
			add(&value.Items[i].Descriptor)
		}
	case *kwakrpc.Detail:
		add(&value.Descriptor)
	}
}
