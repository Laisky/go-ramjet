import { describe, expect, it } from 'vitest'

import rootPackage from '../../../package.json'
import webPackage from '../../package.json'

describe('reproducible frontend tooling', () => {
  it('pins the same exact pnpm release for root and web commands', () => {
    expect(webPackage.packageManager).toMatch(/^pnpm@\d+\.\d+\.\d+$/)
    expect(rootPackage.packageManager).toBe(webPackage.packageManager)
  })

  it('keeps both package manifests private', () => {
    expect(rootPackage.private).toBe(true)
    expect(webPackage.private).toBe(true)
  })
})

describe('native compiler and API compatibility', () => {
  it('keeps the TypeScript 7 compiler separate from the supported lint API', () => {
    expect(webPackage.devDependencies['@typescript/native']).toBe(
      'npm:typescript@^7.0.2',
    )
    expect(webPackage.devDependencies.typescript).toBe(
      'npm:@typescript/typescript6@^6.0.2',
    )
  })
})
