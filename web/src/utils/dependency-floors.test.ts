import { describe, expect, it } from 'vitest'

import lockfile from '../../pnpm-lock.yaml?raw'
import { readResolvedPackages } from '../test/dependency-lockfile'

const packages = readResolvedPackages(lockfile)

/** compareVersions compares two stable major.minor.patch package versions. */
function compareVersions(actual: string, minimum: string): number {
  const actualParts = actual.split('.').map(Number)
  const minimumParts = minimum.split('.').map(Number)
  for (let index = 0; index < 3; index += 1) {
    const difference = actualParts[index] - minimumParts[index]
    if (difference !== 0) return difference
  }
  return 0
}

const dependencyFloors = [
  ['ajv', 6, '6.14.0'],
  ['minimatch', 3, '3.1.5'],
  ['flatted', 3, '3.4.2'],
  ['picomatch', 4, '4.0.4'],
  ['brace-expansion', 1, '1.1.13'],
  ['vite', 8, '8.3.3'],
  ['postcss', 8, '8.5.29'],
  ['mermaid', 11, '11.15.0'],
  ['mermaid', 12, '12.1.0'],
  ['uuid', 11, '11.1.1'],
  ['uuid', 12, '12.0.1'],
  ['uuid', 13, '13.0.1'],
  ['dompurify', 3, '3.4.17'],
  ['katex', 0, '0.19.0'],
  ['vitest', 4, '4.1.11'],
  ['vitest', 5, '5.0.3'],
  ['minimatch', 11, '11.0.1'],
  ['glob', 14, '14.0.2'],
  ['lodash-es', 4, '4.18.0'],
  ['lodash', 4, '4.18.0'],
  ['lodash-amd', 4, '4.18.0'],
  ['lodash.template', 4, '4.18.0'],
  ['lodash.omit', 4, '4.18.0'],
  ['lodash.unset', 4, '4.18.0'],
  ['undici', 6, '6.29.1'],
  ['undici', 7, '7.30.1'],
  ['undici', 8, '8.12.2'],
  ['postcss-selector-parser', 6, '6.1.5'],
  ['postcss-selector-parser', 7, '7.1.6'],
  ['browserslist', 4, '4.28.7'],
  ['baseline-browser-mapping', 2, '2.11.0'],
  ['@humanfs/node', 0, '0.16.8'],
  ['@babel/core', 7, '7.29.6'],
  // Floors accept stable releases only, so this also rejects 8.0 prereleases.
  ['@babel/core', 8, '8.0.0'],
  ['zod', 4, '4.6.6'],
  ['@ungap/structured-clone', 1, '1.3.1'],
] as const

// Advisories that publish no fix for these majors require a newer major instead.
const unpatchedMajors = [
  ['minimatch', 7, 10],
  ['glob', 9, 13],
  ['uuid', 0, 10],
  ['dompurify', 0, 2],
  ['browserslist', 0, 3],
  ['postcss-selector-parser', 0, 5],
  ['undici', 4, 5],
  ['zod', 0, 3],
] as const

// A version inside every affected major line of each advisory this lockfile
// addresses, paired with the first fixed release that the floors accept.
// Ranges come from `gh api /advisories/<GHSA>`; older majors appear when the
// advisory publishes no fix for them.
const addressedAdvisoryVersions = [
  ['@babel/core', '7.29.0', '7.29.6'], // GHSA-4x5r-pxfx-6jf8
  ['@babel/core', '8.0.0-rc.4', '8.0.0'],
  ['baseline-browser-mapping', '2.10.0', '2.11.0'], // GHSA-w5vr-8v7q-w6rv
  ['browserslist', '4.28.6', '4.28.7'], // GHSA-73wf-gq98-2v4g
  ['browserslist', '3.2.8', '4.28.7'],
  ['dompurify', '3.4.16', '3.4.17'], // GHSA-c6gr-qvv4-5v3f and two others
  ['dompurify', '2.5.8', '3.4.17'],
  ['@humanfs/node', '0.16.7', '0.16.8'], // GHSA-p498-v437-472g
  ['lodash-es', '4.17.23', '4.18.0'], // GHSA-r5fr-rjxr-66jc, GHSA-xxjr-mmjv-4gpg
  ['lodash', '4.17.23', '4.18.0'],
  ['lodash-amd', '4.17.23', '4.18.0'],
  ['lodash.template', '4.5.0', '4.18.0'],
  ['lodash.omit', '4.4.0', '4.18.0'],
  ['lodash.unset', '4.5.2', '4.18.0'],
  ['minimatch', '7.0.0', '11.0.1'], // GHSA-g3hv-75wg-m249
  ['minimatch', '11.0.0', '11.0.1'],
  ['glob', '9.0.0', '14.0.2'], // GHSA-3g3m-2fjr-6pw4
  ['glob', '14.0.1', '14.0.2'],
  ['postcss-selector-parser', '6.1.4', '6.1.5'], // GHSA-rj75-hqrm-r3gf
  ['postcss-selector-parser', '5.0.0', '6.1.5'],
  ['postcss-selector-parser', '7.1.5', '7.1.6'],
  ['undici', '8.12.1', '8.12.2'], // GHSA-2m5v-pch6-74hx and four others
  ['undici', '7.30.0', '7.30.1'],
  ['undici', '6.29.0', '6.29.1'],
  ['undici', '5.29.0', '6.29.1'],
  ['undici', '4.0.0', '6.29.1'],
  ['uuid', '8.3.2', '11.1.1'], // GHSA-w5hq-g745-h8pq
  ['uuid', '11.1.0', '11.1.1'],
  ['uuid', '12.0.0', '12.0.1'],
  ['uuid', '13.0.0', '13.0.1'],
  ['zod', '4.6.5', '4.6.6'], // GHSA-mrhx-c7xc-m3w5
  ['zod', '3.25.76', '4.6.6'],
] as const

/** assertFloor checks that every resolved copy of name in the given major meets minimum. */
function assertFloor(
  resolved: Map<string, Set<string>>,
  name: string,
  major: number,
  minimum: string,
): void {
  for (const version of resolved.get(name) ?? []) {
    if (!version.startsWith(`${major}.`)) continue
    expect(version).toMatch(/^\d+\.\d+\.\d+$/)
    expect(compareVersions(version, minimum)).toBeGreaterThanOrEqual(0)
  }
}

/** assertNoUnpatchedMajor checks that no resolved copy of name uses a major from firstMajor through lastMajor. */
function assertNoUnpatchedMajor(
  resolved: Map<string, Set<string>>,
  name: string,
  firstMajor: number,
  lastMajor: number,
): void {
  for (const version of resolved.get(name) ?? []) {
    const major = Number(version.split('.')[0])
    expect(major < firstMajor || major > lastMajor).toBe(true)
  }
}

/** assertAdvisoryFloors applies every floor and unpatched-major rule to a resolved package graph. */
function assertAdvisoryFloors(resolved: Map<string, Set<string>>): void {
  for (const [name, major, minimum] of dependencyFloors) {
    assertFloor(resolved, name, major, minimum)
  }
  for (const [name, firstMajor, lastMajor] of unpatchedMajors) {
    assertNoUnpatchedMajor(resolved, name, firstMajor, lastMajor)
  }
}

const requiredFloors = [
  ['vite', '8.3.3'],
  ['postcss', '8.5.29'],
  ['mermaid', '12.1.0'],
  ['dompurify', '3.4.17'],
  ['katex', '0.19.0'],
  ['vitest', '5.0.3'],
] as const

describe('resolved dependency PR floors', () => {
  it('reads the complete pnpm package-resolution graph', () => {
    expect(packages.size).toBeGreaterThan(0)
  })

  it.each(dependencyFloors)(
    '%s major %s stays at or above %s when present',
    (name, major, minimum) => {
      assertFloor(packages, name, major, minimum)
    },
  )

  it.each(unpatchedMajors)(
    'no %s copy uses unpatched majors %s through %s',
    (name, firstMajor, lastMajor) => {
      assertNoUnpatchedMajor(packages, name, firstMajor, lastMajor)
    },
  )

  it.each(addressedAdvisoryVersions)(
    'rejects vulnerable %s %s and accepts its fix %s',
    (name, vulnerable, patched) => {
      const lockfileWith = (version: string) =>
        readResolvedPackages(`packages:\n  '${name}@${version}': {}\n`)
      expect(() => assertAdvisoryFloors(lockfileWith(vulnerable))).toThrow()
      expect(() => assertAdvisoryFloors(lockfileWith(patched))).not.toThrow()
    },
  )

  it.each(requiredFloors)(
    'requires every %s copy to meet %s',
    (name, minimum) => {
      const versions = [...(packages.get(name) ?? [])]
      expect(versions.length).toBeGreaterThan(0)
      for (const version of versions) {
        expect(version).toMatch(/^\d+\.\d+\.\d+$/)
        expect(compareVersions(version, minimum)).toBeGreaterThanOrEqual(0)
      }
    },
  )
})

describe('lockfile format compatibility', () => {
  it('reads dependencies after the pnpm toolchain document', () => {
    const parsed = readResolvedPackages(`---
packages:
  '@pnpm/exe@12.9.1': {}
---
packages:
  mermaid@12.1.0: {}
  'mermaid@11.14.0': {}
snapshots:
  unrelated@99.0.0: {}
`)
    expect([...parsed.get('mermaid')!]).toEqual(['12.1.0', '11.14.0'])
    expect(parsed.has('unrelated')).toBe(false)
    expect(compareVersions('11.14.0', '12.1.0')).toBeLessThan(0)
  })

  it('supports scoped, quoted, inline, and peer-qualified resolution keys', () => {
    const parsed = readResolvedPackages(`packages:
  '@typescript/typescript6@6.0.2': {}
  vitest@5.0.3(vite@8.3.3):
    resolution: {}
  katex@0.19.0: {}
`)
    expect([...parsed.get('@typescript/typescript6')!]).toEqual(['6.0.2'])
    expect([...parsed.get('vitest')!]).toEqual(['5.0.3'])
    expect([...parsed.get('katex')!]).toEqual(['0.19.0'])
  })

  it.each(['', 'packages: [', 'packages: []', 'snapshots: {}'])(
    'rejects missing or malformed resolution metadata: %s',
    (source) => {
      expect(() => readResolvedPackages(source)).toThrow()
    },
  )
})
