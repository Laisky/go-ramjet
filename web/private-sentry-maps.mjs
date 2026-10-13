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

/** quarantinePrivateMaps validates matching Debug IDs and privately copies mapped JS before removing every public map. */
export async function quarantinePrivateMaps(
  distDirectory,
  expectedChunks,
  release = '',
  buildContextDirectory = path.dirname(path.resolve(distDirectory)),
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
      const mapBytes = await readRegular(mapFile)
      total += mapBytes.length
      if (total > MAX_TOTAL_BYTES) fail()
      // CSS maps carry no Sentry Debug ID and are removed, never published.
      if (!mapFile.endsWith('.js.map')) continue
      const javascript = mapFile.slice(0, -4)
      const name = relativeFile(root, javascript)
      if (!chunks.has(name)) fail()
      const jsBytes = await readRegular(javascript)
      total += jsBytes.length
      if (total > MAX_TOTAL_BYTES) fail()
      const sourceMap = JSON.parse(mapBytes.toString('utf8'))
      const debugId = sourceMap.debug_id ?? sourceMap.debugId
      const jsIds = [
        ...jsBytes
          .toString('utf8')
          .matchAll(
            new RegExp(`(?:^|\n)//# debugId=(${UUID})(?=\r?\n|$)`, 'g'),
          ),
      ]
      if (
        sourceMap.version !== 3 ||
        !Array.isArray(sourceMap.sources) ||
        typeof debugId !== 'string' ||
        !UUID_PATTERN.test(debugId) ||
        (sourceMap.debug_id !== undefined &&
          sourceMap.debugId !== undefined &&
          sourceMap.debug_id !== sourceMap.debugId) ||
        jsIds.length !== 1 ||
        jsIds[0][1] !== debugId ||
        debugIds.has(debugId)
      )
        fail()
      debugIds.add(debugId)
      artifacts.push({
        name,
        mapName: relativeFile(root, mapFile),
        debugId,
        jsBytes,
        mapBytes,
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
        sourceMapSha256: digest(artifact.mapBytes),
      })
    }
    const manifest = {
      schemaVersion: 1,
      release: release || null,
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
      chunks = Object.values(bundle)
        .filter((entry) => entry.type === 'chunk')
        .map((entry) => entry.fileName)
    },
    async closeBundle() {
      const { artifactDirectory } = await quarantinePrivateMaps(
        distDirectory,
        chunks,
        release,
        buildContextDirectory,
      )
      console.info(`Private chat source-map artifact: ${artifactDirectory}`)
    },
  }
}
