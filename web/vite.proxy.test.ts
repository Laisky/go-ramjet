import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

describe('Vite proxy config', () => {
  it('proxies CV API endpoints in dev proxy config', () => {
    const configPath = path.join(__dirname, 'vite.config.dev.ts')
    const config = readFileSync(configPath, 'utf8')
    expect(config).toContain('/cv/(content|pdf)')
  })

  it('proxies CV API endpoints in default dev config', () => {
    const configPath = path.join(__dirname, 'vite.config.ts')
    const config = readFileSync(configPath, 'utf8')
    expect(config).toContain('/cv/(content|pdf)')
  })
})
