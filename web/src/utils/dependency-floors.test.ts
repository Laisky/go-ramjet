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
  ['dompurify', 3, '3.4.17'],
  ['katex', 0, '0.19.0'],
  ['vitest', 4, '4.1.11'],
  ['vitest', 5, '5.0.3'],
  ['minimatch', 11, '11.0.1'],
  ['glob', 14, '14.0.2'],
  ['lodash-es', 4, '4.18.0'],
  ['undici', 8, '8.12.2'],
  ['postcss-selector-parser', 6, '6.1.5'],
  ['postcss-selector-parser', 7, '7.1.6'],
  ['browserslist', 4, '4.28.7'],
  ['baseline-browser-mapping', 2, '2.11.0'],
  ['@humanfs/node', 0, '0.16.8'],
  ['@babel/core', 7, '7.29.6'],
  ['zod', 4, '4.6.6'],
] as const

// Advisories that publish no fix for these majors require a newer major instead.
const unpatchedMajors = [
  ['minimatch', 7, 10],
  ['glob', 9, 13],
  ['uuid', 0, 10],
] as const

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
      for (const version of packages.get(name) ?? []) {
        if (!version.startsWith(`${major}.`)) continue
        expect(version).toMatch(/^\d+\.\d+\.\d+$/)
        expect(compareVersions(version, minimum)).toBeGreaterThanOrEqual(0)
      }
    },
  )

  it.each(unpatchedMajors)(
    'no %s copy uses unpatched majors %s through %s',
    (name, firstMajor, lastMajor) => {
      for (const version of packages.get(name) ?? []) {
        const major = Number(version.split('.')[0])
        expect(major < firstMajor || major > lastMajor).toBe(true)
      }
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
