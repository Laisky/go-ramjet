package web

import (
	"strings"
	"sync"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
)

const (
	defaultSiteID    = "default"
	defaultSiteTheme = "default"
)

// SiteMetadata represents metadata for a specific site, including identity and SEO fields.
type SiteMetadata struct {
	ID            string
	Theme         string
	Title         string
	Favicon       string
	Description   string
	OGTitle       string
	OGDescription string
	OGImage       string
	ThemeColor    string
}

var (
	siteMetadataMu     sync.RWMutex
	siteMetadataByHost = make(map[string]SiteMetadata)
	siteMetadataByPath = make(map[string]SiteMetadata)
)

// RegisterSiteMetadata registers metadata for the provided hosts or path prefixes in hostsOrPaths using metadata.
// It uses hostsOrPaths to determine which requests match the metadata and returns no value.
func RegisterSiteMetadata(hostsOrPaths []string, metadata SiteMetadata) {
	siteMetadataMu.Lock()
	defer siteMetadataMu.Unlock()

	for _, hop := range hostsOrPaths {
		if strings.HasPrefix(hop, "/") {
			siteMetadataByPath[hop] = metadata
			continue
		}
		siteMetadataByHost[normalizeHost(hop)] = metadata
	}
}

// getSiteMetadata returns the best matching metadata for the provided host and path, using logger for debug output.
func getSiteMetadata(logger glog.Logger, host, path string) SiteMetadata {
	siteMetadataMu.RLock()
	defer siteMetadataMu.RUnlock()

	normalizedHost := normalizeHost(host)
	defaultMeta := defaultSiteMetadata()

	if logger != nil {
		logger.Debug("get site metadata", zap.String("host", normalizedHost), zap.String("path", path))
	}

	// Try host match first.
	if meta, ok := siteMetadataByHost[normalizedHost]; ok {
		result := mergeSiteMetadata(defaultMeta, meta)
		if logger != nil {
			logger.Debug("host match", zap.String("host", normalizedHost), zap.String("title", result.Title))
		}
		return result
	}

	// Try path prefix match (longest match first).
	var bestMatch string
	var bestMeta SiteMetadata
	for p, meta := range siteMetadataByPath {
		if strings.HasPrefix(path, p) && len(p) > len(bestMatch) {
			bestMatch = p
			bestMeta = meta
		}
	}

	if bestMatch != "" {
		result := mergeSiteMetadata(defaultMeta, bestMeta)
		if logger != nil {
			logger.Debug("path match", zap.String("path", path), zap.String("bestMatch", bestMatch), zap.String("title", result.Title))
		}
		return result
	}

	if logger != nil {
		logger.Debug("use default metadata", zap.String("host", normalizedHost), zap.String("path", path))
	}
	return defaultMeta
}

// normalizeHost normalizes the provided host string for metadata matching and returns the normalized host.
func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}

	if idx := strings.Index(host, ":"); idx > 0 {
		host = host[:idx]
	}

	return strings.ToLower(host)
}

// mergeSiteMetadata merges override values on top of base metadata and returns the merged result.
func mergeSiteMetadata(base, override SiteMetadata) SiteMetadata {
	overrideID := override.ID
	overrideTheme := override.Theme
	if override.ID != "" {
		base.ID = override.ID
	}
	if override.Theme != "" {
		base.Theme = override.Theme
	}
	if override.Title != "" {
		base.Title = override.Title
	}
	if override.Favicon != "" {
		base.Favicon = override.Favicon
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.OGTitle != "" {
		base.OGTitle = override.OGTitle
	}
	if override.OGDescription != "" {
		base.OGDescription = override.OGDescription
	}
	if override.OGImage != "" {
		base.OGImage = override.OGImage
	}
	if override.ThemeColor != "" {
		base.ThemeColor = override.ThemeColor
	}

	if overrideTheme == "" && overrideID != "" {
		base.Theme = overrideID
	}

	return finalizeSiteMetadata(base)
}

// finalizeSiteMetadata ensures required metadata fields are populated and returns the normalized metadata.
func finalizeSiteMetadata(meta SiteMetadata) SiteMetadata {
	if meta.ID == "" {
		meta.ID = defaultSiteID
	}
	if meta.Theme == "" {
		meta.Theme = meta.ID
	}
	return meta
}

// defaultSiteMetadata returns the default metadata used when no host/path match is found.
func defaultSiteMetadata() SiteMetadata {
	return finalizeSiteMetadata(SiteMetadata{
		ID:      defaultSiteID,
		Theme:   defaultSiteTheme,
		Title:   "Laisky",
		Favicon: "https://s3.laisky.com/uploads/2025/12/favicon.ico",
	})
}
