import { readFileSync } from 'node:fs'
import path from 'node:path'

/**
 * LEGACY_CHAT_SCRIPT_PATH is the retained legacy GPTChat script that
 * internal/tasks/gptchat/templates/js/js.go embeds into the Go binary. It is a
 * classic browser script with top-level declarations and no exports, so tests
 * load individual declarations from the shipped text instead of importing it.
 * The path is resolved from this file, so vitest may run from any directory.
 */
export const LEGACY_CHAT_SCRIPT_PATH = path.resolve(
  import.meta.dirname,
  '../../../../../../internal/tasks/gptchat/templates/js/chat.js',
)

let cachedSource: string | undefined

/**
 * readLegacyChatScript returns the legacy chat.js source text, reading it from
 * disk once per test file.
 *
 * @returns The full, unmodified source of chat.js.
 */
export function readLegacyChatScript(): string {
  cachedSource ??= readFileSync(LEGACY_CHAT_SCRIPT_PATH, 'utf8')
  return cachedSource
}

/**
 * extractTopLevelDeclaration returns the source text of one top-level
 * `function`, `async function`, or `const` declaration in chat.js.
 *
 * chat.js is formatted so that top-level declarations start at column 0, a
 * function ends at the first following line that is exactly `}`, and a const
 * ends at the first unindented line that ends with `;`.
 *
 * @param source - The chat.js source text.
 * @param name - The declared identifier to extract.
 * @returns The declaration text, including its closing line.
 * @throws Error when the declaration cannot be found or is unterminated.
 */
export function extractTopLevelDeclaration(
  source: string,
  name: string,
): string {
  const lines = source.split('\n')
  const fnHead = new RegExp(`^(async )?function ${name}\\(`)
  const constHead = new RegExp(`^const ${name} =`)
  const start = lines.findIndex(
    (line) => fnHead.test(line) || constHead.test(line),
  )
  if (start < 0) {
    throw new Error(`legacy chat.js has no top-level declaration ${name}`)
  }

  const isFunction = fnHead.test(lines[start])
  for (let end = start; end < lines.length; end++) {
    const line = lines[end]
    const closes = isFunction
      ? line === '}'
      : line.trimEnd().endsWith(';') && (end === start || !/^\s/.test(line))
    if (closes) {
      return lines.slice(start, end + 1).join('\n')
    }
  }

  throw new Error(`legacy chat.js declaration ${name} is unterminated`)
}

/**
 * loadLegacyChatDeclarations evaluates the named chat.js declarations in strict
 * mode with the given free variables bound, mimicking the page globals the
 * legacy script expects.
 *
 * @param names - Top-level declarations to load, in dependency order.
 * @param scope - Free variables (stubs) the declarations reference by name.
 * @param variables - Page-level `let` variables the declarations assign, such
 *   as modal callbacks; each starts undefined and is exposed as a getter.
 * @returns An object mapping each requested name to its evaluated value, plus
 *   a live getter for every name in variables.
 */
export function loadLegacyChatDeclarations<T extends Record<string, unknown>>(
  names: string[],
  scope: Record<string, unknown> = {},
  variables: string[] = [],
): T {
  const source = readLegacyChatScript()
  const body = names
    .map((name) => extractTopLevelDeclaration(source, name))
    .join('\n\n')
  const declared = variables.map((name) => `let ${name};`).join('\n')
  const exported = [
    ...names,
    ...variables.map((name) => `get ${name}() { return ${name}; }`),
  ].join(', ')
  const factory = new Function(
    ...Object.keys(scope),
    `'use strict';\n${declared}\n${body}\nreturn { ${exported} };`,
  )
  return factory(...Object.values(scope)) as T
}
