export const LEGACY_CV_FAVICON_URL =
  'https://s3.laisky.com/uploads/2025/12/favicon.ico'

export type CVPageMeta = {
  faviconHref: string
  ogImage: string
}

export type CVPageMetaPayload = {
  favicon?: string
  og_image?: string
}

/**
 * readMetaContent returns the trimmed content of a matching meta tag.
 */
function readMetaContent(
  attr: 'name' | 'property',
  key: string,
): string | null {
  const tag = document.querySelector<HTMLMetaElement>(`meta[${attr}="${key}"]`)
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
 * readDocumentFavicon returns the current favicon href from the document.
 */
function readDocumentFavicon(): string | null {
  const icon = document.querySelector<HTMLLinkElement>("link[rel~='icon']")
  if (!icon?.href) {
    return null
  }

  const trimmed = icon.href.trim()
  return trimmed.length > 0 ? trimmed : null
}

/**
 * resolveCVPageMeta resolves favicon and og:image with a legacy fallback.
 */
export function resolveCVPageMeta(
  fallbackFavicon = LEGACY_CV_FAVICON_URL,
): CVPageMeta {
  const normalizedFallback = fallbackFavicon.trim() || LEGACY_CV_FAVICON_URL
  const faviconHref = readDocumentFavicon() ?? normalizedFallback
  const ogImage = readMetaContent('property', 'og:image') ?? faviconHref

  return {
    faviconHref,
    ogImage,
  }
}

/**
 * mergeCVPageMeta merges metadata payload from API into current page metadata.
 */
export function mergeCVPageMeta(
  current: CVPageMeta,
  payload: CVPageMetaPayload,
): CVPageMeta {
  const nextFavicon = payload.favicon?.trim()
  const nextOGImage = payload.og_image?.trim()
  const faviconHref =
    nextFavicon && nextFavicon.length > 0 ? nextFavicon : current.faviconHref
  const ogImage =
    nextOGImage && nextOGImage.length > 0
      ? nextOGImage
      : nextFavicon && nextFavicon.length > 0
        ? nextFavicon
        : current.ogImage

  return {
    faviconHref,
    ogImage,
  }
}
