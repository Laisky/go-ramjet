import { constants } from 'node:fs'
import * as fs from 'node:fs/promises'
import { createHash } from 'node:crypto'
import os from 'node:os'
import path from 'node:path'

const FAILURE = 'Private browser source-map validation failed.'
const UUID = '[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}'
const UUID_PATTERN = new RegExp(`^${UUID}$`)
const MAX_FILES = 4096
const MAX_FILE_BYTES = 64 * 1024 * 1024
const MAX_TOTAL_BYTES = 512 * 1024 * 1024

/** fail stops the build using a fixed diagnostic without source content. */
function fail() {
  throw new Error(FAILURE)
}

/** assertDirectory rejects a symbolic-link root without restricting unrelated OS directory aliases. */
async function assertDirectory(directory) {
  const stat = await fs.lstat(directory)
  if (!stat.isDirectory() || stat.isSymbolicLink()) fail()
}

/** hasPublicMapReference inspects comment tokens while ignoring quoted JS/CSS text. */
function hasPublicMapReference(source) {
  const tokens =
    /("(?:\\[\s\S]|[^"\\])*"|'(?:\\[\s\S]|[^'\\])*'|`(?:\\[\s\S]|[^`\\])*`)|(\/\/[^\r\n]*|\/\*[\s\S]*?\*\/)/g
  for (const token of source.matchAll(tokens)) {
    if (
      token[2] &&
      /^(?:\/\/|\/\*)\s*[#@]\s*sourceMappingURL\s*=/.test(token[2])
    )
      return true
  }
  return false
}

/** collectFiles walks bounded regular files without following symbolic links. */
async function collectFiles(root) {
  await assertDirectory(root)
  const files = []
  let visited = 0
  const walk = async (directory, depth) => {
    if (depth > 16) fail()
    const entries = await fs.readdir(directory, { withFileTypes: true })
    for (const entry of entries) {
      if (++visited > MAX_FILES) fail()
      const filename = path.join(directory, entry.name)
      const stat = await fs.lstat(filename)
      if (stat.isSymbolicLink()) fail()
      if (stat.isDirectory()) await walk(filename, depth + 1)
      else if (stat.isFile()) files.push(filename)
      else fail()
    }
  }
  await walk(root, 0)
  return files.sort()
}

/** readRegular reads a size-bounded file through a descriptor that rejects symlinks. */
async function readRegular(filename) {
  const handle = await fs.open(
    filename,
    constants.O_RDONLY | constants.O_NOFOLLOW,
  )
  try {
    const stat = await handle.stat()
    if (!stat.isFile() || stat.size > MAX_FILE_BYTES) fail()
    const bytes = await handle.readFile()
    if (bytes.length > MAX_FILE_BYTES) fail()
    return bytes
  } finally {
    await handle.close()
  }
}

/** relativeFile returns a bounded, portable path contained in the dist tree. */
function relativeFile(root, filename) {
  const relative = path.relative(root, filename)
  if (
    !relative ||
    relative.startsWith('..') ||
    path.isAbsolute(relative) ||
    relative.length > 512 ||
    /[\r\n\\]/.test(relative)
  )
    fail()
  return relative.split(path.sep).join('/')
}

/** digest returns the SHA-256 checksum of the exact artifact bytes. */
function digest(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

/** isPrivateSourceMapBuild enables only an explicit private production build request. */
export function isPrivateSourceMapBuild(command, optIn) {
  return command === 'build' && optIn === 'true'
}

/** privateSentryOptions prevents uploads, release mutations, and plugin telemetry. */
export function privateSentryOptions() {
  return {
    authToken: '',
    telemetry: false,
    silent: true,
    sourcemaps: { disable: 'disable-upload' },
    release: { inject: false, create: false, finalize: false },
    errorHandler: fail,
  }
}

/** isWithin returns whether a resolved candidate is inside or equal to a directory. */
function isWithin(directory, candidate) {
  const relative = path.relative(directory, candidate)
  return (
    relative === '' ||
    (!relative.startsWith('..' + path.sep) &&
      relative !== '..' &&
      !path.isAbsolute(relative))
  )
}

/** preparePrivateSourceMap validates the official JS footer and adds absent Debug ID metadata only to private map bytes. */
function preparePrivateSourceMap(jsBytes, originalMapBytes) {
  const javascript = jsBytes.toString('utf8')
  const footers = [
    ...javascript.matchAll(/(?:^|\n)\/\/# debugId=([^\r\n]*)(?=\r?\n|$)/g),
  ]
  if (footers.length !== 1 || !UUID_PATTERN.test(footers[0][1])) fail()
  const debugId = footers[0][1]
  const assignments = [
    ...javascript.matchAll(/\b_sentryDebugIdIdentifier\s*=/g),
  ]
  const runtimeMarkers = [
    ...javascript.matchAll(
      /\b_sentryDebugIdIdentifier\s*=\s*(["'`])([^"'`\r\n]*)\1/g,
    ),
  ]
  if (
    assignments.length !== runtimeMarkers.length ||
    runtimeMarkers.length > 1 ||
    runtimeMarkers.some((marker) => marker[2] !== 'sentry-dbid-' + debugId)
  )
    fail()
  const sourceMap = JSON.parse(originalMapBytes.toString('utf8'))
  if (sourceMap.version !== 3 || !Array.isArray(sourceMap.sources)) fail()
  const existingFields = ['debug_id', 'debugId'].filter((field) =>
    Object.hasOwn(sourceMap, field),
  )
  if (existingFields.some((field) => sourceMap[field] !== debugId)) fail()
  const repairedMetadata = existingFields.length === 0
  if (repairedMetadata) {
    // Official @sentry/bundler-plugins 10.76.2 core/debug-id-upload.js
    // setDebugIdOnSourceMap assigns both fields; preserve every other parsed field.
    sourceMap.debug_id = debugId
    sourceMap.debugId = debugId
  }
  const mapBytes = repairedMetadata
    ? Buffer.from(JSON.stringify(sourceMap) + '\n', 'utf8')
    : originalMapBytes
  if (mapBytes.length > MAX_FILE_BYTES) fail()
  return { debugId, mapBytes, repairedMetadata }
}

/** chunkCoverage returns bounded coverage counts without forwarding arbitrary metadata. */
function chunkCoverage(mappedChunks, supplied) {
  const coverage = supplied ?? {
    generatedChunks: mappedChunks,
    mappedChunks,
    unmappedChunks: 0,
  }
  const {
    generatedChunks,
    mappedChunks: recordedMapped,
    unmappedChunks,
  } = coverage
  if (
    ![generatedChunks, recordedMapped, unmappedChunks].every(
      (count) => Number.isInteger(count) && count >= 0 && count <= MAX_FILES,
    ) ||
    recordedMapped !== mappedChunks ||
    generatedChunks !== recordedMapped + unmappedChunks
  )
    fail()
  return { generatedChunks, mappedChunks: recordedMapped, unmappedChunks }
}

/** quarantinePrivateMaps validates matching Debug IDs and privately copies mapped JS before removing every public map. */
export async function quarantinePrivateMaps(
  distDirectory,
  expectedChunks,
  release = '',
  buildContextDirectory = path.dirname(path.resolve(distDirectory)),
  suppliedCoverage,
) {
  const root = path.resolve(distDirectory)
  let mapFiles = []
  try {
    const files = await collectFiles(root)
    mapFiles = files.filter((filename) => filename.endsWith('.map'))
    if (
      !mapFiles.length ||
      !Array.isArray(expectedChunks) ||
      !expectedChunks.length
    )
      fail()
    if (release && !/^go-ramjet@[a-f0-9]{7,40}$/.test(release)) fail()
    const chunks = new Set(expectedChunks)
    if (chunks.size !== expectedChunks.length || chunks.size > MAX_FILES) fail()
    const coverage = chunkCoverage(chunks.size, suppliedCoverage)
    for (const chunk of chunks) {
      if (typeof chunk !== 'string' || !chunk.endsWith('.js')) fail()
      const filename = path.resolve(root, chunk)
      if (
        relativeFile(root, filename) !== chunk ||
        !files.includes(filename + '.map')
      )
        fail()
    }

    // Hidden maps must never produce a public sourceMappingURL, including public JS/CSS.
    let scannedBytes = 0
    for (const filename of files.filter((file) => /\.(?:js|css)$/.test(file))) {
      const bytes = await readRegular(filename)
      scannedBytes += bytes.length
      if (
        scannedBytes > MAX_TOTAL_BYTES ||
        hasPublicMapReference(bytes.toString('utf8'))
      )
        fail()
    }

    const artifacts = []
    const debugIds = new Set()
    let total = 0
    for (const mapFile of mapFiles) {
      const originalMapBytes = await readRegular(mapFile)
      total += originalMapBytes.length
      if (total > MAX_TOTAL_BYTES) fail()
      // CSS maps carry no Sentry Debug ID and are removed, never published.
      if (!mapFile.endsWith('.js.map')) continue
      const javascript = mapFile.slice(0, -4)
      const name = relativeFile(root, javascript)
      if (!chunks.has(name)) fail()
      const jsBytes = await readRegular(javascript)
      total += jsBytes.length
      if (total > MAX_TOTAL_BYTES) fail()
      const { debugId, mapBytes, repairedMetadata } = preparePrivateSourceMap(
        jsBytes,
        originalMapBytes,
      )
      if (debugIds.has(debugId)) fail()
      debugIds.add(debugId)
      artifacts.push({
        name,
        mapName: relativeFile(root, mapFile),
        debugId,
        jsBytes,
        mapBytes,
        originalMapBytes,
        repairedMetadata,
      })
    }
    if (artifacts.length !== chunks.size) fail()

    // Resolve OS directory aliases without rejecting unrelated symlink ancestry.
    const servedDirectory = await fs.realpath(root)
    const buildContext = await fs.realpath(buildContextDirectory)
    const temporaryDirectory = await fs.realpath(os.tmpdir())
    if (
      isWithin(servedDirectory, temporaryDirectory) ||
      isWithin(buildContext, temporaryDirectory)
    )
      fail()
    const artifactDirectory = await fs.mkdtemp(
      path.join(temporaryDirectory, 'go-ramjet-sentry-maps-'),
    )
    if (
      isWithin(servedDirectory, artifactDirectory) ||
      isWithin(buildContext, artifactDirectory)
    )
      fail()
    await fs.chmod(artifactDirectory, 0o700)
    const entries = []
    for (const artifact of artifacts) {
      for (const [name, bytes] of [
        [artifact.name, artifact.jsBytes],
        [artifact.mapName, artifact.mapBytes],
      ]) {
        const destination = path.join(artifactDirectory, name)
        await fs.mkdir(path.dirname(destination), {
          recursive: true,
          mode: 0o700,
        })
        await fs.writeFile(destination, bytes, { flag: 'wx', mode: 0o600 })
      }
      entries.push({
        javascript: artifact.name,
        sourceMap: artifact.mapName,
        debugId: artifact.debugId,
        javascriptSha256: digest(artifact.jsBytes),
        originalMapSha256: digest(artifact.originalMapBytes),
        sourceMapSha256: digest(artifact.mapBytes),
      })
    }
    const manifest = {
      schemaVersion: 1,
      release: release || null,
      coverage,
      repairedMetadata: artifacts.filter(
        (artifact) => artifact.repairedMetadata,
      ).length,
      artifacts: entries,
    }
    await fs.writeFile(
      path.join(artifactDirectory, 'manifest.json'),
      JSON.stringify(manifest, null, 2) + '\n',
      { flag: 'wx', mode: 0o600 },
    )
    return { artifactDirectory, manifest }
  } catch {
    fail()
  } finally {
    // A failed opt-in build must not leave collected source maps in the serving tree.
    let cleanupFailed = false
    for (const filename of mapFiles) {
      try {
        await fs.unlink(filename)
      } catch {
        cleanupFailed = true
      }
    }
    if (cleanupFailed) fail()
  }
}

/** privateSentryArtifactPlugin archives actual Vite chunks after Debug ID injection. */
export function privateSentryArtifactPlugin() {
  let distDirectory
  let buildContextDirectory
  let chunks = []
  let coverage
  let release = ''
  return {
    name: 'chat-private-sentry-artifacts',
    enforce: 'post',
    apply: 'build',
    configResolved(config) {
      distDirectory = path.resolve(config.root, config.build.outDir)
      buildContextDirectory = path.resolve(config.root, '..')
      release = process.env.VITE_CHAT_SENTRY_RELEASE || ''
    },
    generateBundle(_options, bundle) {
      const generated = Object.values(bundle).filter(
        (entry) => entry.type === 'chunk',
      )
      // Mapless facade chunks are coverage gaps, not uploadable JS/map pairs.
      chunks = generated
        .filter((entry) => {
          const mapName = entry.sourcemapFileName ?? entry.fileName + '.map'
          if (bundle[mapName]?.type !== 'asset') return false
          if (mapName !== entry.fileName + '.map') fail()
          return true
        })
        .map((entry) => entry.fileName)
      coverage = {
        generatedChunks: generated.length,
        mappedChunks: chunks.length,
        unmappedChunks: generated.length - chunks.length,
      }
    },
    async closeBundle() {
      const { artifactDirectory } = await quarantinePrivateMaps(
        distDirectory,
        chunks,
        release,
        buildContextDirectory,
        coverage,
      )
      console.info(`Private chat source-map artifact: ${artifactDirectory}`)
    },
  }
}
