/**
 * Update the page title.
 */
export function setPageTitle(title: string) {
  if (document.title !== title) {
    document.title = title
  }
}

/**
 * Update the page favicon.
 */
export function setPageFavicon(href: string) {
  let link: HTMLLinkElement | null = document.querySelector("link[rel~='icon']")
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  if (link.href !== href) {
    link.href = href
  }
}
