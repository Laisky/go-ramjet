import assert from 'node:assert/strict'
import * as fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { afterEach, test } from 'node:test'
import { createHash } from 'node:crypto'
import {
  isPrivateSourceMapBuild,
  privateSentryArtifactPlugin,
  privateSentryOptions,
  quarantinePrivateMaps,
} from './private-sentry-maps.mjs'

const DEBUG_ID = '12345678-1234-1234-1234-123456789abc'
const PRIVATE_MARKER = 'synthetic-private-source-content'
const FAILURE = 'Private browser source-map validation failed.'
const ownedDirectories = new Set()

/** cleanup removes only temporary fixture and artifact directories owned by these tests. */
afterEach(async function cleanup() {
  for (const directory of ownedDirectories) {
    assert.ok(path.dirname(directory) === path.resolve(os.tmpdir()))
    assert.match(
      path.basename(directory),
      /^(?:chat-map-fixture-|go-ramjet-sentry-maps-)/,
    )
    await fs.rm(directory, { recursive: true, force: true })
  }
  ownedDirectories.clear()
})

/** fixture creates synthetic JS/map inputs, never loading real sources or credentials. */
async function fixture() {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'chat-map-fixture-'))
  ownedDirectories.add(root)
  const dist = path.join(root, 'dist')
  await fs.mkdir(path.join(dist, 'assets'), { recursive: true })
  const javascript = `console.log('synthetic');\n//# debugId=${DEBUG_ID}\n`
  const sourceMap = JSON.stringify({
    version: 3,
    sources: ['synthetic.ts'],
    sourcesContent: [PRIVATE_MARKER],
    names: [],
    mappings: '',
    debug_id: DEBUG_ID,
  })
  await fs.writeFile(path.join(dist, 'assets/main.js'), javascript)
  await fs.writeFile(path.join(dist, 'assets/main.js.map'), sourceMap)
  return { root, dist, javascript, sourceMap }
}

/** expectFailure verifies fixed diagnostics and removes no files outside the fixture. */
async function expectFailure(dist, chunks = ['assets/main.js'], release = '') {
  await assert.rejects(quarantinePrivateMaps(dist, chunks, release), {
    message: FAILURE,
  })
}

test('plugin disables all upload, release and telemetry operations even with inherited settings', () => {
  const options = privateSentryOptions()
  assert.equal(options.authToken, '')
  assert.equal(options.telemetry, false)
  assert.equal(options.silent, true)
  assert.deepEqual(options.sourcemaps, { disable: 'disable-upload' })
  assert.deepEqual(options.release, {
    inject: false,
    create: false,
    finalize: false,
  })
  assert.throws(options.errorHandler, { message: FAILURE })
  const plugin = privateSentryArtifactPlugin()
  assert.equal(plugin.apply, 'build')
  assert.equal(plugin.enforce, 'post')
})

test('archives matching JS and hidden maps outside dist with checksums and private permissions', async () => {
  const { dist, javascript, sourceMap } = await fixture()
  const release = 'go-ramjet@' + 'a'.repeat(40)
  const { artifactDirectory, manifest } = await quarantinePrivateMaps(
    dist,
    ['assets/main.js'],
    release,
  )
  ownedDirectories.add(artifactDirectory)
  assert.equal(path.dirname(artifactDirectory), path.resolve(os.tmpdir()))
  assert.equal((await fs.stat(artifactDirectory)).mode & 0o777, 0o700)
  assert.equal(
    (await fs.stat(path.join(artifactDirectory, 'manifest.json'))).mode & 0o777,
    0o600,
  )
  assert.deepEqual(
    JSON.parse(
      await fs.readFile(path.join(artifactDirectory, 'manifest.json'), 'utf8'),
    ),
    manifest,
  )
  assert.equal(manifest.release, release)
  assert.equal(manifest.artifacts.length, 1)
  assert.equal(manifest.artifacts[0].debugId, DEBUG_ID)
  assert.equal(
    manifest.artifacts[0].javascriptSha256,
    createHash('sha256').update(javascript).digest('hex'),
  )
  assert.equal(
    manifest.artifacts[0].sourceMapSha256,
    createHash('sha256').update(sourceMap).digest('hex'),
  )
  assert.equal(
    await fs.readFile(path.join(artifactDirectory, 'assets/main.js'), 'utf8'),
    javascript,
  )
  assert.equal(
    await fs.readFile(
      path.join(artifactDirectory, 'assets/main.js.map'),
      'utf8',
    ),
    sourceMap,
  )
  assert.equal(
    await fs.readFile(path.join(dist, 'assets/main.js'), 'utf8'),
    javascript,
  )
  assert.deepEqual(await fs.readdir(path.join(dist, 'assets')), ['main.js'])
  assert.ok(!javascript.includes(PRIVATE_MARKER))
})

test('rejects mismatched Debug IDs and erases public maps on failure', async () => {
  const { dist, sourceMap } = await fixture()
  await fs.writeFile(
    path.join(dist, 'assets/main.js.map'),
    sourceMap.replace(DEBUG_ID, 'abcdefab-1234-1234-1234-123456789abc'),
  )
  await expectFailure(dist)
  assert.deepEqual(await fs.readdir(path.join(dist, 'assets')), ['main.js'])
})

test('rejects sourceMappingURL in emitted or copied public JS', async () => {
  const { dist, javascript } = await fixture()
  await fs.writeFile(
    path.join(dist, 'copied.js'),
    javascript + '\n//# sourceMappingURL=private.map\n',
  )
  await expectFailure(dist)
  assert.ok(
    !(await fs.readdir(path.join(dist, 'assets'))).includes('main.js.map'),
  )
})

test('rejects public CSS mapping URLs', async () => {
  const { dist } = await fixture()
  await fs.writeFile(
    path.join(dist, 'assets/main.css'),
    'body {}\n/*# sourceMappingURL=private.map */',
  )
  await expectFailure(dist)
})

test('removes CSS maps while keeping copied public JS that has no source map', async () => {
  const { dist } = await fixture()
  await fs.writeFile(path.join(dist, 'assets/main.css.map'), '{}')
  await fs.writeFile(
    path.join(dist, 'worklet.js'),
    '/* synthetic public worklet */',
  )
  const { artifactDirectory, manifest } = await quarantinePrivateMaps(dist, [
    'assets/main.js',
  ])
  ownedDirectories.add(artifactDirectory)
  assert.equal(manifest.artifacts.length, 1)
  assert.deepEqual(await fs.readdir(path.join(dist, 'assets')), ['main.js'])
})

test('rejects map symlinks without reading or deleting their outside target', async () => {
  const { root, dist } = await fixture()
  const outside = path.join(root, 'outside.map')
  await fs.writeFile(outside, PRIVATE_MARKER)
  await fs.unlink(path.join(dist, 'assets/main.js.map'))
  await fs.symlink(outside, path.join(dist, 'assets/main.js.map'))
  await expectFailure(dist)
  assert.equal(await fs.readFile(outside, 'utf8'), PRIVATE_MARKER)
})

test('rejects symlinked dist roots and nested directories', async () => {
  const { root, dist } = await fixture()
  await fs.symlink(dist, path.join(root, 'alias'))
  await expectFailure(path.join(root, 'alias'))
  await fs.symlink(root, path.join(dist, 'assets', 'nested-alias'))
  await expectFailure(dist)
})

test('rejects chunk path traversal rather than archiving outside files', async () => {
  const { dist } = await fixture()
  await expectFailure(dist, ['../outside.js'])
})

test('requires maps for all actual emitted chunks', async () => {
  const { dist } = await fixture()
  await fs.writeFile(path.join(dist, 'assets/unmapped.js'), 'synthetic')
  await expectFailure(dist, ['assets/main.js', 'assets/unmapped.js'])
})

test('rejects duplicate chunk paths and duplicate Debug IDs', async () => {
  const { dist, javascript, sourceMap } = await fixture()
  await fs.writeFile(path.join(dist, 'assets/other.js'), javascript)
  await fs.writeFile(path.join(dist, 'assets/other.js.map'), sourceMap)
  await expectFailure(dist, ['assets/main.js', 'assets/other.js'])
  const second = await fixture()
  await expectFailure(second.dist, ['assets/main.js', 'assets/main.js'])
})

test('rejects malformed maps and missing or repeated JS Debug ID comments', async () => {
  for (const kind of [
    'invalid-json',
    'version',
    'sources',
    'conflicting-ids',
    'missing',
    'repeated',
  ]) {
    const { dist, javascript, sourceMap } = await fixture()
    if (kind === 'invalid-json')
      await fs.writeFile(path.join(dist, 'assets/main.js.map'), PRIVATE_MARKER)
    if (kind === 'version')
      await fs.writeFile(
        path.join(dist, 'assets/main.js.map'),
        sourceMap.replace('"version":3', '"version":2'),
      )
    if (kind === 'sources')
      await fs.writeFile(
        path.join(dist, 'assets/main.js.map'),
        sourceMap.replace('"sources":["synthetic.ts"]', '"sources":null'),
      )
    if (kind === 'conflicting-ids')
      await fs.writeFile(
        path.join(dist, 'assets/main.js.map'),
        JSON.stringify({
          ...JSON.parse(sourceMap),
          debugId: 'abcdefab-1234-1234-1234-123456789abc',
        }),
      )
    if (kind === 'missing')
      await fs.writeFile(path.join(dist, 'assets/main.js'), 'synthetic')
    if (kind === 'repeated')
      await fs.writeFile(
        path.join(dist, 'assets/main.js'),
        javascript + `//# debugId=${DEBUG_ID}\n`,
      )
    await expectFailure(dist)
  }
})

test('bounds map size and never reports malformed submitted release content', async () => {
  const { dist } = await fixture()
  await fs.truncate(path.join(dist, 'assets/main.js.map'), 64 * 1024 * 1024 + 1)
  await expectFailure(dist)
  const second = await fixture()
  await expectFailure(
    second.dist,
    ['assets/main.js'],
    PRIVATE_MARKER + '\nunsafe',
  )
})

test('requires at least one matched map and bounds directory depth', async () => {
  const { dist } = await fixture()
  await fs.unlink(path.join(dist, 'assets/main.js.map'))
  await expectFailure(dist)
  const second = await fixture()
  await fs.mkdir(
    path.join(second.dist, ...Array.from({ length: 17 }, () => 'nested')),
    { recursive: true },
  )
  await expectFailure(second.dist)
})

test('default and development builds cannot opt into private map plugin side effects', () => {
  for (const optIn of [undefined, '', 'false', 'TRUE', '1']) {
    assert.equal(isPrivateSourceMapBuild('build', optIn), false)
  }
  assert.equal(isPrivateSourceMapBuild('serve', 'true'), false)
  assert.equal(isPrivateSourceMapBuild('build', 'true'), true)
})

test('does not mistake ordinary JS regex/string names for public mapping directives', async () => {
  const { dist, javascript } = await fixture()
  await fs.writeFile(
    path.join(dist, 'assets/main.js'),
    'const pattern = /sourceMappingURL=/;\nconst marker = "//# sourceMappingURL=private.map";\n' +
      javascript,
  )
  const { artifactDirectory } = await quarantinePrivateMaps(dist, [
    'assets/main.js',
  ])
  ownedDirectories.add(artifactDirectory)
})

test('private map correlation accepts an existing short release identifier', async () => {
  const { dist } = await fixture()
  const { artifactDirectory, manifest } = await quarantinePrivateMaps(
    dist,
    ['assets/main.js'],
    'go-ramjet@abcdef0',
  )
  ownedDirectories.add(artifactDirectory)
  assert.equal(manifest.release, 'go-ramjet@abcdef0')
})

test('rejects TMPDIR inside served dist before copying maps and cleans failed-build maps', async () => {
  const { dist } = await fixture()
  const temporaryDirectory = path.join(dist, 'temporary')
  await fs.mkdir(temporaryDirectory)
  const originalTmpdir = os.tmpdir
  os.tmpdir = () => temporaryDirectory
  try {
    await expectFailure(dist)
    assert.deepEqual(await fs.readdir(temporaryDirectory), [])
    assert.deepEqual(await fs.readdir(path.join(dist, 'assets')), ['main.js'])
  } finally {
    os.tmpdir = originalTmpdir
  }
})

test('rejects a private temp directory elsewhere inside the checkout build context', async () => {
  const { root, dist } = await fixture()
  const temporaryDirectory = path.join(root, 'temporary')
  await fs.mkdir(temporaryDirectory)
  const originalTmpdir = os.tmpdir
  os.tmpdir = () => temporaryDirectory
  try {
    await assert.rejects(
      quarantinePrivateMaps(dist, ['assets/main.js'], '', root),
      { message: FAILURE },
    )
    assert.deepEqual(await fs.readdir(temporaryDirectory), [])
    assert.deepEqual(await fs.readdir(path.join(dist, 'assets')), ['main.js'])
  } finally {
    os.tmpdir = originalTmpdir
  }
})

test('permits an OS temporary directory alias whose resolved artifact location is outside the checkout', async () => {
  const { root, dist } = await fixture()
  const originalTmpdir = os.tmpdir
  const alias = path.join(root, 'os-temporary-alias')
  await fs.symlink(originalTmpdir(), alias)
  os.tmpdir = () => alias
  try {
    const { artifactDirectory } = await quarantinePrivateMaps(
      dist,
      ['assets/main.js'],
      '',
      root,
    )
    ownedDirectories.add(artifactDirectory)
    assert.equal(
      path.dirname(artifactDirectory),
      await fs.realpath(originalTmpdir()),
    )
    assert.deepEqual(await fs.readdir(path.join(dist, 'assets')), ['main.js'])
  } finally {
    os.tmpdir = originalTmpdir
  }
})
