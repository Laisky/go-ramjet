import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { describe, expect, it, vi } from 'vitest'

describe('legacy chat.js test fixture', () => {
  it('locates chat.js regardless of the working directory', async () => {
    const original = process.cwd()
    const elsewhere = mkdtempSync(path.join(tmpdir(), 'legacy-chat-cwd-'))
    try {
      process.chdir(elsewhere)
      vi.resetModules()
      const fixture = await import('./fixtures/legacy-chat-script')

      expect(fixture.readLegacyChatScript()).toContain(
        'function applyUrlConfigOverrides(',
      )
    } finally {
      process.chdir(original)
      rmSync(elsewhere, { recursive: true, force: true })
    }
  })
})
