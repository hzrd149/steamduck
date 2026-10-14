package kwakrpc

import (
	"context"
	"time"
)

var (
	timeZero time.Time
	timePast = time.Unix(1, 0)
)

// Version identifies one publication of a napplet.
type Version struct {
	EventID      string `json:"event_id"`
	CreatedAt    int64  `json:"created_at"`
	ArtifactHash string `json:"artifact_hash"`
}

// Descriptor describes a napplet in the catalog or install list.
type Descriptor struct {
	Address   string  `json:"address"`
	Name      string  `json:"name"`
	Format    string  `json:"format"`
	Available bool    `json:"available"`
	Version   Version `json:"version"`

	// Catalog fields beyond the documented minimum (kwakore's
	// ServiceDescriptor); the protocol allows optional additions.
	Description       string   `json:"description,omitempty"`
	Author            string   `json:"author,omitempty"`
	AuthorName        string   `json:"author_name,omitempty"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
	Kind              int      `json:"kind,omitempty"`
	Icon              *IconRef `json:"icon,omitempty"`
	Archetypes        []string `json:"archetypes,omitempty"`
	RequiredDomains   []string `json:"required_domains,omitempty"`
	OptionalDomains   []string `json:"optional_domains,omitempty"`
	MissingDomains    []string `json:"missing_domains,omitempty"`
	Compatible        bool     `json:"compatible"`
}

// IconRef names a manifest icon by hash; napplet.icon serves its bytes.
type IconRef struct {
	SHA256 string `json:"sha256"`
	Mime   string `json:"mime"`
}

// IsGame reports whether the napplet declares the "game" archetype.
func (d Descriptor) IsGame() bool {
	for _, a := range d.Archetypes {
		if a == "game" {
			return true
		}
	}
	return false
}

// InstalledItem is one napplet.installed entry.
type InstalledItem struct {
	Descriptor
	UpdateAvailable *Version `json:"update_available"`
}

// Detail is the napplet.detail result.
type Detail struct {
	Descriptor
	Installed        bool      `json:"installed"`
	InstalledVersion *Version  `json:"installed_version"`
	UpdateAvailable  *Version  `json:"update_available"`
	Versions         []Version `json:"versions"`
}

// Health is the service's health report.
type Health struct {
	Ready         bool    `json:"ready"`
	Version       string  `json:"version"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	ConfigStatus  string  `json:"config_status"`
	StorageStatus string  `json:"storage_status"`
	ActiveWindows int     `json:"active_windows"`
}

// Status is the service.status result.
type Status struct {
	ProtocolVersion int    `json:"protocol_version"`
	Health          Health `json:"health"`
}

// Page is a paginated list result.
type Page[T any] struct {
	Items      []T  `json:"items"`
	Total      int  `json:"total"`
	NextOffset *int `json:"next_offset"`
}

// DiscoverPage is the napplet.discover result.
type DiscoverPage struct {
	Page[Descriptor]
	FetchedAt *string `json:"fetched_at"`
	Complete  bool    `json:"complete"`
}

// DiscoverParams are napplet.discover's named params. Nil pointers are
// omitted, because the daemon rejects explicit nulls.
type DiscoverParams struct {
	Query      string `json:"query,omitempty"`
	Refresh    bool   `json:"refresh,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Archetype  string `json:"archetype,omitempty"`
	Compatible *bool  `json:"compatible,omitempty"`
}

// InstallResult is the napplet.install result.
type InstallResult struct {
	Address          string  `json:"address"`
	Outcome          string  `json:"outcome"`
	InstalledVersion Version `json:"installed_version"`
}

// LaunchResult is the napplet.launch result.
type LaunchResult struct {
	Address  string `json:"address"`
	WindowID string `json:"window_id"`
	Outcome  string `json:"outcome"`
}

// Window is one napplet.windows item.
type Window struct {
	WindowID string `json:"window_id"`
	Address  string `json:"address"`
	Name     string `json:"name"`
}

// Icon is the napplet.icon result; Data is base64 PNG bytes.
type Icon struct {
	SHA256 string `json:"sha256"`
	Mime   string `json:"mime"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Data   string `json:"data"`
}

func (c *Client) Status(ctx context.Context) (*Status, error) {
	var s Status
	return &s, c.Call(ctx, "service.status", nil, &s)
}

func (c *Client) Discover(ctx context.Context, p DiscoverParams) (*DiscoverPage, error) {
	var out DiscoverPage
	return &out, c.Call(ctx, "napplet.discover", p, &out)
}

// Installed returns every installed napplet, following pagination.
func (c *Client) Installed(ctx context.Context) ([]InstalledItem, error) {
	all := []InstalledItem{} // an empty list, not null, for the store UI
	offset := 0
	for {
		var page Page[InstalledItem]
		if err := c.Call(ctx, "napplet.installed", map[string]int{"offset": offset, "limit": 500}, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.NextOffset == nil {
			return all, nil
		}
		offset = *page.NextOffset
	}
}

func (c *Client) Detail(ctx context.Context, address string) (*Detail, error) {
	var d Detail
	return &d, c.Call(ctx, "napplet.detail", map[string]string{"address": address}, &d)
}

func (c *Client) Install(ctx context.Context, address string) (*InstallResult, error) {
	var r InstallResult
	return &r, c.Call(ctx, "napplet.install", map[string]string{"address": address}, &r)
}

func (c *Client) Update(ctx context.Context, address string) error {
	return c.Call(ctx, "napplet.update", map[string]string{"address": address}, nil)
}

func (c *Client) Uninstall(ctx context.Context, address string) error {
	return c.Call(ctx, "napplet.uninstall", map[string]any{"address": address, "confirm": true}, nil)
}

func (c *Client) Launch(ctx context.Context, address string) (*LaunchResult, error) {
	var r LaunchResult
	return &r, c.Call(ctx, "napplet.launch", map[string]string{"address": address}, &r)
}

func (c *Client) Try(ctx context.Context, address string) (*LaunchResult, error) {
	var r LaunchResult
	return &r, c.Call(ctx, "napplet.try", map[string]string{"address": address}, &r)
}

func (c *Client) Stop(ctx context.Context, windowID string) error {
	return c.Call(ctx, "napplet.stop", map[string]string{"window_id": windowID}, nil)
}

func (c *Client) Windows(ctx context.Context) ([]Window, error) {
	var r struct {
		Items []Window `json:"items"`
	}
	return r.Items, c.Call(ctx, "napplet.windows", nil, &r)
}

func (c *Client) Icon(ctx context.Context, address string) (*Icon, error) {
	var i Icon
	return &i, c.Call(ctx, "napplet.icon", map[string]string{"address": address}, &i)
}
