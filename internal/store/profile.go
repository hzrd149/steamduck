package store

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/hzrd149/steamduck/internal/kwakrpc"
)

// profileRelays are asked for a profile besides the user's own relays:
// they keep profiles for most accounts.
var profileRelays = []string{
	"wss://purplepag.es",
	"wss://relay.damus.io",
	"wss://nos.lol",
	"wss://relay.primal.net",
}

// profile is the signed-in account's public profile as the UI shows it,
// with resource references rather than inline image bytes.
type profile struct {
	PublicKey   string                     `json:"public_key"`
	Npub        string                     `json:"npub"`
	Found       bool                       `json:"found"`
	Name        string                     `json:"name,omitempty"`
	DisplayName string                     `json:"display_name,omitempty"`
	About       string                     `json:"about,omitempty"`
	Website     string                     `json:"website,omitempty"`
	NIP05       string                     `json:"nip05,omitempty"`
	LUD16       string                     `json:"lud16,omitempty"`
	Picture     *kwakrpc.ResourceReference `json:"picture,omitempty"`
	Banner      *kwakrpc.ResourceReference `json:"banner,omitempty"`
	CreatedAt   int64                      `json:"created_at,omitempty"`
}

type profileParams struct {
	Refresh bool `json:"refresh"`
}

type profileCache struct {
	mu      sync.Mutex
	entries map[string]*profile
}

// getProfile returns the signed-in account's profile, or nil when nobody
// is signed in. It is cached for the session; refresh asks the relays again.
func (a *API) getProfile(ctx context.Context, refresh bool) (*profile, error) {
	s, err := a.callClient(ctx).SignerStatus(ctx)
	if err != nil {
		return nil, err
	}
	pk := s.PublicKey
	if pk == "" {
		return nil, nil
	}
	a.profiles.mu.Lock()
	if a.profiles.entries == nil {
		a.profiles.entries = map[string]*profile{}
	}
	p, ok := a.profiles.entries[pk]
	a.profiles.mu.Unlock()
	if ok && !refresh {
		return p, nil
	}

	relays := append([]string{}, profileRelays...)
	if settings, err := a.callClient(ctx).Settings(ctx); err == nil {
		if list, ok := settings["relays"].([]any); ok {
			for _, r := range list {
				if r, ok := r.(string); ok {
					relays = append(relays, r)
				}
			}
		}
	}
	relayCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	ev := newestMetadata(relayCtx, relays, pk)
	cancel()
	p = &profile{PublicKey: pk, Npub: npub(pk)}
	if ev != nil {
		var m struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			About       string `json:"about"`
			Website     string `json:"website"`
			NIP05       string `json:"nip05"`
			LUD16       string `json:"lud16"`
			Picture     string `json:"picture"`
			Banner      string `json:"banner"`
		}
		if json.Unmarshal([]byte(ev.Content), &m) == nil {
			p.Found = true
			p.CreatedAt = ev.CreatedAt
			p.Name = clip(m.Name, 64)
			p.DisplayName = clip(m.DisplayName, 64)
			p.About = clip(m.About, 2000)
			p.Website = clip(m.Website, 200)
			p.NIP05 = clip(m.NIP05, 200)
			p.LUD16 = clip(m.LUD16, 200)
			p.Picture = profileResource(m.Picture)
			p.Banner = profileResource(m.Banner)
		}
	}
	// a lookup that found nothing is not cached, so the next visit tries again
	if p.Found {
		a.profiles.mu.Lock()
		a.profiles.entries[pk] = p
		a.profiles.mu.Unlock()
	}
	return p, nil
}

type metadataEvent struct {
	PubKey    string `json:"pubkey"`
	Kind      int    `json:"kind"`
	CreatedAt int64  `json:"created_at"`
	Content   string `json:"content"`
}

// newestMetadata asks every relay at once for the key's kind 0 and keeps
// the newest one any of them has. Signatures are not checked: this is a
// read-only preview, and the key only signs what its owner approves.
func newestMetadata(ctx context.Context, relays []string, pk string) *metadataEvent {
	seen := map[string]bool{}
	results := make(chan *metadataEvent)
	n := 0
	for _, r := range relays {
		r = strings.TrimRight(strings.TrimSpace(r), "/")
		if seen[r] || !(strings.HasPrefix(r, "wss://") || strings.HasPrefix(r, "ws://")) {
			continue
		}
		seen[r] = true
		n++
		go func() { results <- queryMetadata(ctx, r, pk) }()
	}
	var best *metadataEvent
	for range n {
		if ev := <-results; ev != nil && (best == nil || ev.CreatedAt > best.CreatedAt) {
			best = ev
		}
	}
	return best
}

func queryMetadata(ctx context.Context, relay, pk string) *metadataEvent {
	conn, _, err := websocket.Dial(ctx, relay, nil)
	if err != nil {
		return nil
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	req, _ := json.Marshal([]any{"REQ", "p", map[string]any{"kinds": []int{0}, "authors": []string{pk}, "limit": 1}})
	if conn.Write(ctx, websocket.MessageText, req) != nil {
		return nil
	}
	var best *metadataEvent
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return best
		}
		var msg []json.RawMessage
		if json.Unmarshal(data, &msg) != nil || len(msg) < 2 {
			continue
		}
		var typ string
		_ = json.Unmarshal(msg[0], &typ)
		switch typ {
		case "EVENT":
			var ev metadataEvent
			if len(msg) < 3 || json.Unmarshal(msg[2], &ev) != nil {
				continue
			}
			if ev.Kind == 0 && ev.PubKey == pk && (best == nil || ev.CreatedAt > best.CreatedAt) {
				best = &ev
			}
		case "EOSE", "CLOSED":
			return best
		}
	}
}

// Profile references are fetched by NAP-RESOURCE with the built-in's normal
// fetch consent, DNS/redirect checks, MIME sniffing and resource byte limits.
func profileResource(raw string) *kwakrpc.ResourceReference {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil
	}
	return &kwakrpc.ResourceReference{URL: u.String()}
}

// clip trims s and cuts it to at most n runes.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
