package web

import (
	"strings"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
)

// SiteConfig describes a site entry loaded from settings, including host/path selectors and metadata fields.
type SiteConfig struct {
	ID            string   `mapstructure:"id"`
	Hosts         []string `mapstructure:"hosts"`
	PathPrefixes  []string `mapstructure:"path_prefixes"`
	Theme         string   `mapstructure:"theme"`
	Title         string   `mapstructure:"title"`
	Favicon       string   `mapstructure:"favicon"`
	Description   string   `mapstructure:"description"`
	OGTitle       string   `mapstructure:"og_title"`
	OGDescription string   `mapstructure:"og_description"`
	OGImage       string   `mapstructure:"og_image"`
	ThemeColor    string   `mapstructure:"theme_color"`
}

// LoadSiteMetadataFromSettings loads site metadata from config and registers it, returning any load error.
func LoadSiteMetadataFromSettings() error {
	if !gconfig.Shared.IsSet("web.sites") {
		return nil
	}

	var sites []SiteConfig
	if err := gconfig.Shared.UnmarshalKey("web.sites", &sites); err != nil {
		return errors.Wrap(err, "unmarshal web.sites")
	}

	for idx, site := range sites {
		if err := registerSiteConfig(site); err != nil {
			return errors.Wrapf(err, "register web.sites[%d]", idx)
		}
	}

	return nil
}

// registerSiteConfig registers the site config provided in site as metadata entries and returns any validation error.
func registerSiteConfig(site SiteConfig) error {
	normalized := normalizeSiteConfig(site)
	if normalized.ID == "" {
		return errors.WithStack(errors.New("site id is empty"))
	}

	if len(normalized.Hosts) == 0 && len(normalized.PathPrefixes) == 0 {
		return errors.WithStack(errors.New("site hosts or path prefixes are required"))
	}

	metadata := SiteMetadata{
		ID:            normalized.ID,
		Theme:         normalized.Theme,
		Title:         normalized.Title,
		Favicon:       normalized.Favicon,
		Description:   normalized.Description,
		OGTitle:       normalized.OGTitle,
		OGDescription: normalized.OGDescription,
		OGImage:       normalized.OGImage,
		ThemeColor:    normalized.ThemeColor,
	}

	if len(normalized.Hosts) > 0 {
		RegisterSiteMetadata(normalized.Hosts, metadata)
	}
	if len(normalized.PathPrefixes) > 0 {
		RegisterSiteMetadata(normalized.PathPrefixes, metadata)
	}

	return nil
}

// normalizeSiteConfig trims empty values in site and returns a normalized SiteConfig.
func normalizeSiteConfig(site SiteConfig) SiteConfig {
	site.ID = strings.TrimSpace(site.ID)
	site.Theme = strings.TrimSpace(site.Theme)
	site.Title = strings.TrimSpace(site.Title)
	site.Favicon = strings.TrimSpace(site.Favicon)
	site.Description = strings.TrimSpace(site.Description)
	site.OGTitle = strings.TrimSpace(site.OGTitle)
	site.OGDescription = strings.TrimSpace(site.OGDescription)
	site.OGImage = strings.TrimSpace(site.OGImage)
	site.ThemeColor = strings.TrimSpace(site.ThemeColor)

	site.Hosts = filterNonEmpty(site.Hosts)
	site.PathPrefixes = normalizePathPrefixes(site.PathPrefixes)

	return site
}

// filterNonEmpty drops empty strings from values and returns the filtered slice.
func filterNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// normalizePathPrefixes ensures path prefixes are non-empty and start with "/" and returns the normalized slice.
func normalizePathPrefixes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "/") {
			trimmed = "/" + trimmed
		}
		out = append(out, trimmed)
	}
	return out
}
