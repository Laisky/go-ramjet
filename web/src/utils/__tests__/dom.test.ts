import { beforeEach, describe, expect, it } from 'vitest'
import { setPageFavicon, setPageTitle } from '../dom'

describe('DOM utilities', () => {
  beforeEach(() => {
    document.title = 'Default'
    const existingIcon = document.querySelector("link[rel~='icon']")
    if (existingIcon) {
      existingIcon.remove()
    }
  })

  it('should set page title', () => {
    setPageTitle('New Title')
    expect(document.title).toBe('New Title')
  })

  it('should set page favicon', () => {
    setPageFavicon('/new-icon.ico')
    const link = document.querySelector("link[rel~='icon']") as HTMLLinkElement
    expect(link).not.toBeNull()
    expect(link.href).toContain('/new-icon.ico')
  })

  it('should update existing page favicon', () => {
    // Create initial icon
    const initialLink = document.createElement('link')
    initialLink.rel = 'icon'
    initialLink.href = '/old-icon.ico'
    document.head.appendChild(initialLink)

    setPageFavicon('/updated-icon.ico')
    const link = document.querySelector("link[rel~='icon']") as HTMLLinkElement
    expect(link).not.toBeNull()
    expect(link.href).toContain('/updated-icon.ico')

    // Ensure we didn't create a second link
    const links = document.querySelectorAll("link[rel~='icon']")
    expect(links.length).toBe(1)
  })
})
