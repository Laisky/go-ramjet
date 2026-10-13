import type { Plugin } from 'vite'

/** isPrivateSourceMapBuild returns whether the command and explicit opt-in request private maps. */
export function isPrivateSourceMapBuild(
  command: string,
  optIn?: string,
): boolean

/** privateSentryOptions returns standard plugin options with all external operations disabled. */
export function privateSentryOptions(): {
  authToken: string
  telemetry: false
  silent: true
  sourcemaps: { disable: 'disable-upload' }
  release: { inject: false; create: false; finalize: false }
  errorHandler: () => never
}
/** PrivateArtifactManifest describes private artifact paths, matching Debug IDs and exact checksums. */
export interface PrivateArtifactManifest {
  schemaVersion: number
  release: string | null
  artifacts: Array<{
    javascript: string
    sourceMap: string
    debugId: string
    javascriptSha256: string
    sourceMapSha256: string
  }>
}
/** quarantinePrivateMaps validates dist chunks and returns their private artifact directory and manifest. */
export function quarantinePrivateMaps(
  distDirectory: string,
  expectedChunks: string[],
  release?: string,
  buildContextDirectory?: string,
): Promise<{ artifactDirectory: string; manifest: PrivateArtifactManifest }>
/** privateSentryArtifactPlugin returns the build hook that archives and removes hidden maps. */
export function privateSentryArtifactPlugin(): Plugin
