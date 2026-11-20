import { afterEach, describe, expect, it } from 'vitest'

import { loadLegacyChatDeclarations } from './fixtures/legacy-chat-script'

type McpServer = Record<string, unknown> & { url: string; api_key: string }

type SessionConfig = Record<string, unknown> & {
  chat_switch?: Record<string, unknown>
  mcp_servers?: McpServer[]
}

const STORED_MCP_URL = 'https://mcp.example.test'

type UrlConfigApi = {
  applyUrlConfigOverrides: (sconfig: SessionConfig) => boolean
  UrlParamAliasMap: Map<string, string>
  UrlConfigBooleanFields: Set<string>
  UrlConfigIntegerFields: Set<string>
  UrlConfigFloatFields: Set<string>
}

// Markup that becomes a live element if a session-name sink parses it.
const IMG_PAYLOAD = '<img src="x" onerror="globalThis.__legacyChatXss = 1">'

// normalizeUrlParamKey lowercases keys, so the marker must be lowercase too.
const POLLUTION_MARKER = 'legacychatpolluted'

/**
 * loadUrlConfigApi loads the legacy URL override helpers from chat.js.
 *
 * @param allModels - Mutable stand-in for the page-level AllModels list.
 * @returns The evaluated applyUrlConfigOverrides function.
 */
function loadUrlConfigApi(allModels: string[] = []): UrlConfigApi {
  return loadLegacyChatDeclarations<UrlConfigApi>(
    [
      'UrlConfigBooleanFields',
      'UrlConfigIntegerFields',
      'UrlConfigFloatFields',
      'UrlParamAliasMap',
      'UrlSettableConfigPaths',
      'normalizeUrlParamKey',
      'parseBooleanParamValue',
      'parseIntegerParamValue',
      'parseFloatParamValue',
      'getNestedConfigValue',
      'isUnsafeConfigPathSegment',
      'isUrlScalarConfigValue',
      'isUrlSettableConfigPath',
      'setNestedConfigValue',
      'coerceConfigValue',
      'applyUrlConfigOverrides',
    ],
    { DefaultModel: 'default-model', AllModels: allModels },
  )
}

/**
 * newSessionConfig returns a session config shaped like the legacy defaults.
 *
 * @returns A fresh plain-object session config.
 */
function newSessionConfig(): SessionConfig {
  return {
    api_token: 'FREETIER-placeholder',
    api_base: 'https://api.example.test',
    selected_model: 'default-model',
    temperature: 1,
    max_tokens: 500,
    n_contexts: 6,
    system_prompt: '',
    chat_switch: { all_in_one: false, enable_mcp: false },
    mcp_bootstrapped: true,
    mcp_servers: [
      {
        id: 'srv-1',
        name: 'Default MCP',
        url: STORED_MCP_URL,
        api_key: 'mcp-key-placeholder',
        enabled: true,
        tools: [],
      },
    ],
  }
}

/**
 * visit points the jsdom location at the given query string.
 *
 * @param search - Query string including the leading `?`.
 */
function visit(search: string) {
  window.history.replaceState({}, '', `/gptchat${search}`)
}

describe('legacy chat.js applyUrlConfigOverrides', () => {
  afterEach(() => {
    delete (Object.prototype as Record<string, unknown>)[POLLUTION_MARKER]
    window.history.replaceState({}, '', '/')
  })

  it.each([
    `?__proto__.${POLLUTION_MARKER}=yes`,
    `?chat_switch.__proto__.${POLLUTION_MARKER}=yes`,
    `?constructor.prototype.${POLLUTION_MARKER}=yes`,
    `?chat_switch.constructor.prototype.${POLLUTION_MARKER}=yes`,
  ])('does not pollute Object.prototype from %s', (search) => {
    const { applyUrlConfigOverrides } = loadUrlConfigApi()
    const sconfig = newSessionConfig()
    visit(search)

    applyUrlConfigOverrides(sconfig)

    expect(
      Object.prototype.hasOwnProperty.call(Object.prototype, POLLUTION_MARKER),
    ).toBe(false)
    expect(({} as Record<string, unknown>)[POLLUTION_MARKER]).toBeUndefined()
    expect(Object.prototype.hasOwnProperty.call(sconfig, 'constructor')).toBe(
      false,
    )
    expect(
      Object.prototype.hasOwnProperty.call(sconfig.chat_switch, 'constructor'),
    ).toBe(false)
  })

  it('ignores the inherited constructor root key and keeps it in the URL', () => {
    const { applyUrlConfigOverrides } = loadUrlConfigApi()
    const sconfig = newSessionConfig()
    visit('?constructor=hijacked')

    expect(applyUrlConfigOverrides(sconfig)).toBe(false)

    expect(Object.prototype.hasOwnProperty.call(sconfig, 'constructor')).toBe(
      false,
    )
    expect(window.location.search).toBe('?constructor=hijacked')
  })

  it('still applies supported overrides and strips them from the URL', () => {
    const allModels: string[] = []
    const { applyUrlConfigOverrides } = loadUrlConfigApi(allModels)
    const sconfig = newSessionConfig()
    visit(
      '?model=custom-model&temperature=0.5&max_tokens=900&enable_mcp=yes&unknown=kept',
    )

    expect(applyUrlConfigOverrides(sconfig)).toBe(true)

    expect(sconfig.selected_model).toBe('custom-model')
    expect(sconfig.temperature).toBe(0.5)
    expect(sconfig.max_tokens).toBe(900)
    expect(sconfig.chat_switch).toEqual({
      all_in_one: false,
      enable_mcp: true,
    })
    expect(allModels).toEqual(['custom-model'])
    expect(window.location.search).toBe('?unknown=kept')
  })

  it.each([
    ['lacks it', undefined],
    ['stores it as an array', []],
  ])('creates chat_switch when the stored config %s', (_case, stored) => {
    const { applyUrlConfigOverrides } = loadUrlConfigApi()
    const sconfig = newSessionConfig()
    if (stored === undefined) {
      delete sconfig.chat_switch
    } else {
      sconfig.chat_switch = stored as unknown as Record<string, unknown>
    }
    visit('?all_in_one=true')

    expect(applyUrlConfigOverrides(sconfig)).toBe(true)

    expect(sconfig.chat_switch).toStrictEqual({ all_in_one: true })
    expect(window.location.search).toBe('')
  })

  // A link is untrusted input. Nested paths would let it re-target a stored
  // MCP server (whose api_key is then sent to the new URL), replace whole
  // objects, or truncate arrays, so only scalar settings and single
  // chat_switch flags may be set.
  it.each([
    '?mcp_servers.0.url=https://attacker.example',
    '?mcp_servers=replaced',
    '?mcp_servers.length=0',
    '?mcp_servers.length=not-a-number',
    '?chat_switch=replaced',
    '?chat_switch.nested.flag=on',
    '?system_prompt.length=1',
  ])('leaves stored objects and arrays untouched for %s', (search) => {
    const { applyUrlConfigOverrides } = loadUrlConfigApi()
    const sconfig = newSessionConfig()
    const before = structuredClone(sconfig)
    visit(search)

    expect(applyUrlConfigOverrides(sconfig)).toBe(false)

    expect(sconfig).toEqual(before)
    expect(sconfig.mcp_servers?.[0].url).toBe(STORED_MCP_URL)
    expect(window.location.search).toBe(search)
  })

  // Only the documented alias targets are URL-settable, like the React app's
  // explicit UrlSettableConfigPaths. Other own scalars such as session_name
  // (own once a session is renamed or duplicated, and rendered by the session
  // list) or internal flags must not be reachable from a link.
  it.each([
    `?session_name=${encodeURIComponent(IMG_PAYLOAD)}`,
    '?mcp_bootstrapped=false',
    '?chat_switch.custom_flag=on',
  ])('ignores %s, which is not a documented URL setting', (search) => {
    const { applyUrlConfigOverrides } = loadUrlConfigApi()
    const sconfig = { ...newSessionConfig(), session_name: 'work' }
    const before = structuredClone(sconfig)
    visit(search)

    expect(applyUrlConfigOverrides(sconfig)).toBe(false)

    expect(sconfig).toEqual(before)
    expect(window.location.search).toBe(search)
  })

  it('still applies every documented alias from a link', () => {
    const api = loadUrlConfigApi()
    const linkValue = (field: string) => {
      if (api.UrlConfigBooleanFields.has(field)) return 'true'
      if (api.UrlConfigIntegerFields.has(field)) return '7'
      if (api.UrlConfigFloatFields.has(field)) return '0.25'
      return 'from-link'
    }
    const expected = (field: string) => {
      if (api.UrlConfigBooleanFields.has(field)) return true
      if (api.UrlConfigIntegerFields.has(field)) return 7
      if (api.UrlConfigFloatFields.has(field)) return 0.25
      return 'from-link'
    }

    expect(api.UrlParamAliasMap.size).toBeGreaterThan(40)
    for (const [alias, target] of api.UrlParamAliasMap) {
      const sconfig: SessionConfig = {
        ...newSessionConfig(),
        token_type: 'proxy',
        presence_penalty: 0,
        frequency_penalty: 0,
        chat_switch: {
          all_in_one: false,
          disable_https_crawler: false,
          enable_talk: false,
          enable_mcp: false,
          draw_n_images: 1,
        },
      }
      const field = target.split('.').at(-1) as string
      visit(`?${alias}=${linkValue(field)}`)

      expect(api.applyUrlConfigOverrides(sconfig), alias).toBe(true)

      const [root, leaf] = target.split('.')
      const actual = leaf
        ? (sconfig[root] as Record<string, unknown>)[leaf]
        : sconfig[root]
      expect(actual, alias).toBe(expected(field))
      expect(window.location.search, alias).toBe('')
    }
  })
})
