package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	"rsc.io/qr"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

// signerView is the signer status the UI shows, with the key as an npub.
type signerView struct {
	kwakrpc.SignerStatus
	Npub string `json:"npub"`
}

func viewSigner(s *kwakrpc.SignerStatus) signerView {
	return signerView{SignerStatus: *s, Npub: npub(s.PublicKey)}
}

// pairOffer is what the UI shows for a nostrconnect pairing: the link and
// its QR code. The link carries the one-time secret, which stays between
// the store and the signer app.
type pairOffer struct {
	URI   string `json:"uri"`
	QR    string `json:"qr"` // PNG data URL
	Relay string `json:"relay"`
}

type ruleParams struct {
	Address string `json:"address"`
	kwakrpc.SavedRule
}

type settingParams struct {
	Field string `json:"field"`
	Value any    `json:"value"`
}

// callAccount serves the signer, permission and settings calls. ok is false
// for a method it does not know.
func (a *API) callAccount(ctx context.Context, method string, raw json.RawMessage) (res any, ok bool, err error) {
	ok = true
	switch method {
	case "signer.status":
		s, err := a.Client.SignerStatus(ctx)
		if err != nil {
			return nil, ok, err
		}
		return viewSigner(s), ok, nil
	case "signer.switch":
		var p kwakrpc.SignerSwitch
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, ok, err
		}
		// a bunker handshake may take the daemon's full 20 seconds
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		c := a.Client.Fork()
		defer c.Close()
		s, err := c.SwitchSigner(ctx, p)
		if err != nil {
			return nil, ok, err
		}
		return viewSigner(s), ok, nil
	case "signer.pair.start":
		return a.pairStart(ctx)
	case "signer.pair.wait":
		// the offer lasts two minutes; the daemon gives up after 125 seconds
		ctx, cancel := context.WithTimeout(ctx, 135*time.Second)
		defer cancel()
		c := a.Client.Fork()
		defer c.Close()
		s, err := c.PairWait(ctx)
		if err != nil {
			return nil, ok, err
		}
		return viewSigner(s), ok, nil
	case "profile.get":
		var p profileParams
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &p)
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		res, err := a.getProfile(ctx, p.Refresh)
		return res, ok, err
	case "signer.pair.cancel":
		return nil, ok, a.Client.PairCancel(ctx)
	case "permissions.get":
		var p addressParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, ok, err
		}
		res, err := a.Client.Permissions(ctx, p.Address)
		if err == nil && res.SavedRules == nil {
			res.SavedRules = []kwakrpc.SavedRule{}
		}
		return res, ok, err
	case "permissions.clear":
		var p ruleParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, ok, err
		}
		return nil, ok, a.Client.ClearPermission(ctx, p.Address, p.SavedRule)
	case "settings.get":
		res, err := a.Client.Settings(ctx)
		return res, ok, err
	case "settings.set":
		var p settingParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, ok, err
		}
		return nil, ok, a.Client.SetSetting(ctx, p.Field, p.Value)
	}
	return nil, false, nil
}

func (a *API) pairStart(ctx context.Context) (any, bool, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, true, err
	}
	secret := hex.EncodeToString(b[:])
	start, err := a.Client.PairStart(ctx, secret)
	if err != nil {
		return nil, true, err
	}
	uri := start.PairURI(secret, "Steamduck")
	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		return nil, true, err
	}
	code.Scale = 6
	return pairOffer{
		URI:   uri,
		QR:    "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()),
		Relay: start.Relay,
	}, true, nil
}
