import { describe, it, expect } from 'vitest'
import fs from 'fs'
import path from 'path'

describe('site-themes.css', () => {
  const cssPath = path.resolve(__dirname, 'site-themes.css')
  const cssContent = fs.readFileSync(cssPath, 'utf-8')

  it('chat site theme should be scoped to dark mode', () => {
    // Expect :root[data-site='chat'].dark
    expect(cssContent).toContain(":root[data-site='chat'].dark")
    // Should NOT contain unconditional chat selector
    expect(cssContent).not.toContain(":root[data-site='chat'] {")
  })

  it('cv site theme should be scoped to non-dark mode', () => {
    // Expect :root[data-site='cv']:not(.dark)
    expect(cssContent).toContain(":root[data-site='cv']:not(.dark)")
    // Should NOT contain unconditional cv selector
    expect(cssContent).not.toContain(":root[data-site='cv'] {")
  })
})
