package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// SiteSettings is what the sites list reads and writes (settings.Service).
type SiteSettings interface {
	ChefSites(ctx context.Context) ([]settings.ChefSite, error)
	AddChefSite(ctx context.Context, domain, lang string) (string, error)
	SetChefSiteEnabled(ctx context.Context, domain string, enabled bool) error
	RemoveChefSite(ctx context.Context, domain string) error
}

// SiteBlocks are the domains the find search skips for a while.
type SiteBlocks interface {
	SearchBlocks(ctx context.Context, now time.Time) (map[string]time.Time, error)
	ClearSearchBlock(ctx context.Context, domain string) error
}

// SitesHandler serves the recipe sites finds search (FIND-7).
type SitesHandler struct {
	sites  SiteSettings
	blocks SiteBlocks
	now    func() time.Time
}

// NewSitesHandler creates a SitesHandler.
func NewSitesHandler(sites SiteSettings, blocks SiteBlocks) *SitesHandler {
	return &SitesHandler{sites: sites, blocks: blocks, now: time.Now}
}

type siteView struct {
	Domain       string     `json:"domain"`
	Lang         string     `json:"lang"`
	Enabled      bool       `json:"enabled"`
	Default      bool       `json:"default"`
	BlockedUntil *time.Time `json:"blocked_until"`
}

// List handles GET /api/chef/sites: {items: [{domain, lang, enabled,
// default, blocked_until}]}, blocked_until null unless the search provider
// refuses the site right now.
func (h *SitesHandler) List(w http.ResponseWriter, r *http.Request) {
	sites, err := h.sites.ChefSites(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	blocks, err := h.blocks.SearchBlocks(r.Context(), h.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	items := make([]siteView, 0, len(sites))
	for _, s := range sites {
		v := siteView{Domain: s.Domain, Lang: s.Lang, Enabled: s.Enabled, Default: settings.IsDefaultChefSite(s.Domain)}
		if until, ok := blocks[s.Domain]; ok {
			v.BlockedUntil = &until
		}
		items = append(items, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *SitesHandler) fail(w http.ResponseWriter, err error) {
	status, key := toHTTPWithResource(err, "site")
	writeError(w, status, key)
}

// Add handles POST /api/chef/sites {domain, lang}: the new site, enabled.
func (h *SitesHandler) Add(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain string `json:"domain"`
		Lang   string `json:"lang"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if _, err := h.sites.AddChefSite(r.Context(), req.Domain, req.Lang); err != nil {
		h.fail(w, err)
		return
	}
	h.List(w, r)
}

func siteParam(r *http.Request) string {
	d, err := url.PathUnescape(chi.URLParam(r, "domain"))
	if err != nil {
		return chi.URLParam(r, "domain")
	}
	return d
}

// SetEnabled handles PUT /api/chef/sites/{domain} {enabled}.
func (h *SitesHandler) SetEnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.sites.SetChefSiteEnabled(r.Context(), siteParam(r), *req.Enabled); err != nil {
		h.fail(w, err)
		return
	}
	h.List(w, r)
}

// Remove handles DELETE /api/chef/sites/{domain}: a default site is refused
// (409), it can only be disabled.
func (h *SitesHandler) Remove(w http.ResponseWriter, r *http.Request) {
	if err := h.sites.RemoveChefSite(r.Context(), siteParam(r)); err != nil {
		h.fail(w, err)
		return
	}
	h.List(w, r)
}

// Retry handles POST /api/chef/sites/{domain}/retry: clears the search block
// so the next find tries the site again.
func (h *SitesHandler) Retry(w http.ResponseWriter, r *http.Request) {
	if err := h.blocks.ClearSearchBlock(r.Context(), siteParam(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	h.List(w, r)
}
