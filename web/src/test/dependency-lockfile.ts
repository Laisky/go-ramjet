import { parseAllDocuments } from 'yaml'

/** isRecord checks whether a decoded YAML value is a mapping. */
function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** readResolvedPackages reads every package-resolution document in a pnpm lockfile. */
export function readResolvedPackages(source: string): Map<string, Set<string>> {
  const result = new Map<string, Set<string>>()
  const documents = parseAllDocuments(source)
  if (documents.length === 0)
    throw new Error('The dependency lockfile is empty')

  for (const document of documents) {
    if (document.errors.length > 0) throw document.errors[0]
    const graph: unknown = document.toJS()
    if (!isRecord(graph) || !isRecord(graph.packages)) {
      throw new Error('A lockfile document has no package-resolution mapping')
    }
    for (const key of Object.keys(graph.packages)) {
      const match = /^(.+)@(\d+\.\d+\.\d+(?:-[\da-zA-Z.-]+)?)(?:\(.*\))?$/.exec(
        key,
      )
      if (!match) continue
      const [, name, version] = match
      const versions = result.get(name) ?? new Set<string>()
      versions.add(version)
      result.set(name, versions)
    }
  }
  return result
}
