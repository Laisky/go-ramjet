/**
 * SiteMeta represents resolved site identifiers used by the frontend.
 */
export type SiteMeta = {
  id: string
  theme: string
}

const DEFAULT_SITE_ID = 'default'
const DEFAULT_THEME = 'default'

/**
 * readMetaContent returns the content attribute for the meta tag identified by name.
 */
function readMetaContent(name: string): string | null {
  if (typeof document === 'undefined') {
    return null
  }
  const tag = document.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
  if (!tag) {
    return null
  }
  const content = tag.getAttribute('content')
  if (!content) {
    return null
  }
  const trimmed = content.trim()
  return trimmed.length > 0 ? trimmed : null
}

/**
 * resolveSiteId determines the active site id from env, DOM meta, or defaults and returns it.
 */
export function resolveSiteId(): string {
  const envSite = import.meta.env.VITE_SITE_ID
  if (typeof envSite === 'string' && envSite.trim().length > 0) {
    return envSite.trim()
  }

  if (typeof document === 'undefined') {
    return DEFAULT_SITE_ID
  }

  const metaSite = readMetaContent('ramjet-site')
  if (metaSite) {
    return metaSite
  }

  const dataSite = document.documentElement.dataset.site
  if (dataSite && dataSite.trim().length > 0) {
    return dataSite.trim()
  }

  return DEFAULT_SITE_ID
}

/**
 * resolveSiteTheme determines the active theme from env, DOM meta, or defaults and returns it.
 */
export function resolveSiteTheme(): string {
  const envTheme = import.meta.env.VITE_SITE_THEME
  if (typeof envTheme === 'string' && envTheme.trim().length > 0) {
    return envTheme.trim()
  }

  if (typeof document === 'undefined') {
    return DEFAULT_THEME
  }

  const metaTheme = readMetaContent('ramjet-theme')
  if (metaTheme) {
    return metaTheme
  }

  const dataTheme = document.documentElement.dataset.theme
  if (dataTheme && dataTheme.trim().length > 0) {
    return dataTheme.trim()
  }

  const siteId = resolveSiteId()
  return siteId || DEFAULT_THEME
}

/**
 * bootstrapSite applies the resolved site metadata to the document element and returns it.
 */
export function bootstrapSite(): SiteMeta {
  const id = resolveSiteId()
  const theme = resolveSiteTheme()
  if (typeof document !== 'undefined') {
    document.documentElement.dataset.site = id
    document.documentElement.dataset.theme = theme
  }

  return { id, theme }
}

/**
 * getActiveSiteId reads the active site id from the document element and returns it.
 */
export function getActiveSiteId(): string {
  if (typeof document === 'undefined') {
    return DEFAULT_SITE_ID
  }
  const dataSite = document.documentElement.dataset.site
  if (dataSite && dataSite.trim().length > 0) {
    return dataSite.trim()
  }

  return resolveSiteId()
}
