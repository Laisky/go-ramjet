import { describe, expect, it } from 'vitest'

import lockfile from '../../pnpm-lock.yaml?raw'

const packageSection = lockfile.split('\npackages:\n')[1]?.split('\nsnapshots:\n')[0]

/**
 * compareVersions compares stable numeric versions and returns a negative value
 * when actual is older than minimum, zero when equal, and a positive value otherwise.
 */
function compareVersions(actual: string, minimum: string): number {
  const left = actual.split('.').map(Number)
  const right = minimum.split('.').map(Number)
  for (let index = 0; index < 3; index += 1) {
    const difference = left[index] - right[index]
    if (difference !== 0) {
      return difference
    }
  }
  return 0
}

const dependencyFloors = [
  ['ajv', 6, '6.14.0'],
  ['minimatch', 3, '3.1.5'],
  ['flatted', 3, '3.4.2'],
  ['picomatch', 4, '4.0.4'],
  ['brace-expansion', 1, '1.1.13'],
  ['vite', 8, '8.0.5'],
  ['postcss', 8, '8.5.10'],
  ['mermaid', 11, '11.15.0'],
  ['uuid', 11, '11.1.1'],
  ['dompurify', 3, '3.4.1'],
] as const

describe('resolved dependency PR floors', () => {
  it('reads the pnpm package-resolution section', () => {
    expect(packageSection).toBeDefined()
    expect(packageSection?.length).toBeGreaterThan(0)
  })

  it.each(dependencyFloors)(
    '%s major %i stays at or above %s when present',
    (name, major, minimum) => {
      expect(packageSection).toBeDefined()
      const pattern = new RegExp(
        `^  ['"]?${name}@(${major}\\.\\d+\\.\\d+)['"]?:$`,
        'gm',
      )
      const versions = Array.from(
        (packageSection ?? '').matchAll(pattern),
        (match) => match[1],
      )
      // A removed transitive dependency is also a valid resolution of its PR.
      // Direct dependencies must remain present so an empty match cannot pass.
      if (['vite', 'postcss', 'mermaid', 'dompurify'].includes(name)) {
        expect(versions.length).toBeGreaterThan(0)
      }
      for (const version of versions) {
        expect(
          compareVersions(version, minimum),
          `${name}@${version} regresses the minimum ${minimum}`,
        ).toBeGreaterThanOrEqual(0)
      }
    },
  )
})
