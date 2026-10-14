package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"rsc.io/qr"
)

// socialMethods are the kwakore social calls the napplet page makes, passed
// through as they are, with how long each may take. Reads of held data are
// quick; a refresh asks relays, and actions sign and publish.
var socialMethods = map[string]time.Duration{
	"napplet.social":    70 * time.Second,
	"napplet.reactions": 20 * time.Second,
	"napplet.comments":  20 * time.Second,
	"napplet.mentions":  20 * time.Second,
	"napplet.zaps":      20 * time.Second,
	"napplet.vote":      70 * time.Second,
	"napplet.react":     70 * time.Second,
	"napplet.comment":   70 * time.Second,
	"napplet.delete":    70 * time.Second,
	"napplet.share":     70 * time.Second,
	"napplet.zap":       55 * time.Second,
}

// zapInvoice is a zap's lightning invoice with a QR code for a phone
// wallet: kwakore has no wallet of its own.
type zapInvoice struct {
	Invoice     string `json:"invoice"`
	QR          string `json:"qr"` // PNG data URL
	AmountMsats int64  `json:"amount_msats"`
}

// callSocial serves the social calls. ok is false for a method it does not
// know. Each runs on its own connection, so a slow relay refresh or signer
// never holds up browsing.
func (a *API) callSocial(ctx context.Context, method string, raw json.RawMessage) (res any, ok bool, err error) {
	timeout, ok := socialMethods[method]
	if !ok {
		return nil, false, nil
	}
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := a.callClient(ctx).Fork()
	defer c.Close()
	var out json.RawMessage
	if err := c.Call(ctx, method, raw, &out); err != nil {
		return nil, ok, err
	}
	if method != "napplet.zap" {
		return out, ok, nil
	}
	var z zapInvoice
	if err := json.Unmarshal(out, &z); err != nil {
		return nil, ok, err
	}
	// uppercase bolt11 makes a denser, alphanumeric-mode QR code
	code, err := qr.Encode(strings.ToUpper("lightning:"+z.Invoice), qr.L)
	if err != nil {
		return nil, ok, err
	}
	code.Scale = 4
	z.QR = "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())
	return z, ok, nil
}
