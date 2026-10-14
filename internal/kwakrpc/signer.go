package kwakrpc

import (
	"context"
	"net/url"
)

// SignerStatus is the daemon's public signer state. It never carries a
// credential.
type SignerStatus struct {
	Mode            string `json:"mode"`       // none, nsec, bunker or system
	PublicKey       string `json:"public_key"` // hex, empty while disconnected
	ConnectionState string `json:"connection_state"`
}

// SignerSwitch selects a signer. Secret is an nsec or a bunker:// URL;
// Socket names the system signer's socket.
type SignerSwitch struct {
	Mode   string `json:"mode"`
	Secret string `json:"secret,omitempty"`
	Socket string `json:"socket,omitempty"`
}

// PairStart is the signer.pair.start result: the public half of a
// nostrconnect offer.
type PairStart struct {
	ClientPublicKey string `json:"client_public_key"`
	Relay           string `json:"relay"`
}

// PairURI is the nostrconnect:// link a signer app scans. It holds the
// one-time secret, so it goes only to the signer.
func (p PairStart) PairURI(secret, name string) string {
	q := url.Values{}
	q.Set("relay", p.Relay)
	q.Set("secret", secret)
	q.Set("perms", "get_public_key,sign_event,nip44_encrypt,nip44_decrypt,nip04_encrypt,nip04_decrypt")
	q.Set("name", name)
	return "nostrconnect://" + p.ClientPublicKey + "?" + q.Encode()
}

// SavedRule is one persisted permission decision for a napplet.
type SavedRule struct {
	Permission string `json:"permission"`
	Subject    string `json:"subject"`
	Decision   string `json:"decision"`
}

// Permissions is the napplet.permissions.get result.
type Permissions struct {
	Address         string      `json:"address"`
	RequiredDomains []string    `json:"required_domains"`
	OptionalDomains []string    `json:"optional_domains"`
	SavedRules      []SavedRule `json:"saved_rules"`
}

func (c *Client) SignerStatus(ctx context.Context) (*SignerStatus, error) {
	var s SignerStatus
	return &s, c.Call(ctx, "signer.status", nil, &s)
}

// SwitchSigner ends the current signer session and starts the new one.
func (c *Client) SwitchSigner(ctx context.Context, p SignerSwitch) (*SignerStatus, error) {
	var s SignerStatus
	return &s, c.Call(ctx, "signer.switch", p, &s)
}

// PairStart opens a two-minute nostrconnect offer; secret is 32 lowercase
// hex characters.
func (c *Client) PairStart(ctx context.Context, secret string) (*PairStart, error) {
	var p PairStart
	return &p, c.Call(ctx, "signer.pair.start", map[string]string{"secret": secret}, &p)
}

// PairWait waits for the signer app to answer the offer.
func (c *Client) PairWait(ctx context.Context) (*SignerStatus, error) {
	var s SignerStatus
	return &s, c.Call(ctx, "signer.pair.wait", nil, &s)
}

func (c *Client) PairCancel(ctx context.Context) error {
	return c.Call(ctx, "signer.pair.cancel", nil, nil)
}

func (c *Client) Permissions(ctx context.Context, address string) (*Permissions, error) {
	var p Permissions
	return &p, c.Call(ctx, "napplet.permissions.get", map[string]string{"address": address}, &p)
}

// ClearPermission forgets one saved decision, so the napplet asks again.
func (c *Client) ClearPermission(ctx context.Context, address string, rule SavedRule) error {
	p := map[string]string{"address": address, "permission": rule.Permission}
	if rule.Subject != "" {
		p["subject"] = rule.Subject
	}
	return c.Call(ctx, "napplet.permissions.clear", p, nil)
}

// Settings returns the daemon's non-secret settings.
func (c *Client) Settings(ctx context.Context) (map[string]any, error) {
	var s map[string]any
	return s, c.Call(ctx, "settings.get", nil, &s)
}

// SetSetting persists one settings override.
func (c *Client) SetSetting(ctx context.Context, field string, value any) error {
	return c.Call(ctx, "settings.set", map[string]any{"field": field, "value": value}, nil)
}

// Fork returns a client with its own connection to the same daemon, for a
// call that waits a long time without holding up this client's others.
// Close it when done.
func (c *Client) Fork() *Client { return &Client{path: c.path, noRetry: c.noRetry} }
