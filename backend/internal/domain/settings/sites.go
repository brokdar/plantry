package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// hostRE is a registrable-looking host name: labels of letters, digits and
// hyphens, at least one dot, a letter-only top-level label.
var hostRE = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

// NormalizeSiteDomain turns what the user typed (a host, or a pasted page
// address) into the stored form: lowercase host without scheme, path, port
// or a leading www. Anything that is not a host name is domain.ErrInvalidInput.
func NormalizeSiteDomain(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", fmt.Errorf("%w: site domain is empty", domain.ErrInvalidInput)
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil {
		return "", fmt.Errorf("%w: site domain %q", domain.ErrInvalidInput, raw)
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")
	if !hostRE.MatchString(host) {
		return "", fmt.Errorf("%w: site domain %q", domain.ErrInvalidInput, raw)
	}
	return host, nil
}

func isDefaultSite(d string) bool {
	return slices.ContainsFunc(DefaultChefSites(), func(s ChefSite) bool { return s.Domain == d })
}

// IsDefaultChefSite reports whether d is one of FIND-7's built-in sites.
func IsDefaultChefSite(d string) bool { return isDefaultSite(d) }

func (s *Service) saveChefSites(ctx context.Context, sites []ChefSite) error {
	raw, err := json.Marshal(sites)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", KeyChefSites, err)
	}
	return s.Set(ctx, KeyChefSites, string(raw))
}

// AddChefSite adds an enabled site the user typed; an unknown language or a
// bad domain is domain.ErrInvalidInput, a listed one domain.ErrDuplicateName.
// It returns the stored domain.
func (s *Service) AddChefSite(ctx context.Context, rawDomain, lang string) (string, error) {
	d, err := NormalizeSiteDomain(rawDomain)
	if err != nil {
		return "", err
	}
	if lang != "en" && lang != "de" {
		return "", fmt.Errorf("%w: site language must be en or de", domain.ErrInvalidInput)
	}
	sites, err := s.ChefSites(ctx)
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(sites, func(x ChefSite) bool { return x.Domain == d }) {
		return "", fmt.Errorf("%w: site %s", domain.ErrDuplicateName, d)
	}
	return d, s.saveChefSites(ctx, append(sites, ChefSite{Domain: d, Lang: lang, Enabled: true}))
}

// SetChefSiteEnabled switches a listed site on or off; an unlisted one is
// domain.ErrNotFound.
func (s *Service) SetChefSiteEnabled(ctx context.Context, d string, enabled bool) error {
	sites, err := s.ChefSites(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(sites, func(x ChefSite) bool { return x.Domain == d })
	if i < 0 {
		return fmt.Errorf("%w: site %s", domain.ErrNotFound, d)
	}
	sites[i].Enabled = enabled
	return s.saveChefSites(ctx, sites)
}

// RemoveChefSite removes a site the user added; a default site can only be
// disabled (domain.ErrInUse), an unlisted one is domain.ErrNotFound.
func (s *Service) RemoveChefSite(ctx context.Context, d string) error {
	sites, err := s.ChefSites(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(sites, func(x ChefSite) bool { return x.Domain == d })
	if i < 0 {
		return fmt.Errorf("%w: site %s", domain.ErrNotFound, d)
	}
	if isDefaultSite(d) {
		return fmt.Errorf("%w: default site %s can only be disabled", domain.ErrInUse, d)
	}
	return s.saveChefSites(ctx, slices.Delete(sites, i, i+1))
}
