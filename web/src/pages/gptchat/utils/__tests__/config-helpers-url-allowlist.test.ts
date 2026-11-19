import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AllModels } from '../../models'
import { DefaultSessionConfig, type SessionConfig } from '../../types'
import { applyUrlOverridesToConfig } from '../config-helpers'

vi.mock('@/utils/storage', () => ({
  kvGet: vi.fn(),
  StorageKeys: {
    SELECTED_SESSION: 'config_selected_session',
  },
}))

/** PollutionProbeKeys are the Object.prototype keys the attack URLs target. */
const PollutionProbeKeys = [
  'sync_key',
  'headers',
  'polluted_flag',
  'polluted_ctor',
  'polluted_nested',
]

/** CustomUrlModel is a model name only a URL can add to AllModels. */
const CustomUrlModel = 'custom-url-allowlist-model'

/**
 * baseConfig returns a fresh session config with a non-default sync key so
 * tests can tell whether a URL changed it.
 */
function baseConfig(): SessionConfig {
  return {
    ...DefaultSessionConfig,
    api_token: 'sk-original',
    sync_key: 'sync-original',
    chat_switch: { ...DefaultSessionConfig.chat_switch },
  }
}

/**
 * applyWithSearch points the page URL at search and applies its overrides
 * to config, returning the override result.
 */
function applyWithSearch(search: string, config: SessionConfig = baseConfig()) {
  window.history.replaceState({}, '', `/chat${search}`)
  return applyUrlOverridesToConfig(config)
}

/**
 * prototypeProbe reports which probe keys a URL managed to write onto
 * Object.prototype.
 */
function prototypeProbe(): string[] {
  return PollutionProbeKeys.filter((key) =>
    Object.prototype.hasOwnProperty.call(Object.prototype, key),
  )
}

describe('applyUrlOverridesToConfig allowlist', () => {
  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  })

  afterEach(() => {
    // A failing run must not poison other tests.
    for (const key of PollutionProbeKeys) {
      delete (Object.prototype as Record<string, unknown>)[key]
    }
    const customIndex = AllModels.indexOf(CustomUrlModel)
    if (customIndex >= 0) {
      AllModels.splice(customIndex, 1)
    }
    window.history.replaceState({}, '', '/')
    vi.restoreAllMocks()
  })

  describe('rejects unsafe paths', () => {
    it('does not pollute Object.prototype through __proto__.sync_key', () => {
      const { config, mutated } = applyWithSearch(
        '?__proto__.sync_key=attacker',
      )
      expect(prototypeProbe()).toEqual([])
      expect(({} as Record<string, unknown>).sync_key).toBeUndefined()
      expect(config.sync_key).toBe('sync-original')
      expect(mutated).toBe(false)
    })

    it('does not pollute Object.prototype through __proto__.headers.x', () => {
      const { mutated } = applyWithSearch(
        '?__proto__.headers.polluted_nested=1',
      )
      expect(prototypeProbe()).toEqual([])
      expect(mutated).toBe(false)
    })

    it('does not pollute Object.prototype through chat_switch.__proto__', () => {
      const { config, mutated } = applyWithSearch(
        '?chat_switch.__proto__.polluted_flag=1',
      )
      expect(prototypeProbe()).toEqual([])
      expect(config.chat_switch).toEqual(DefaultSessionConfig.chat_switch)
      expect(mutated).toBe(false)
    })

    it('does not write a junk constructor through constructor.prototype', () => {
      const { config, mutated } = applyWithSearch(
        '?constructor.prototype.polluted_ctor=1',
      )
      expect(prototypeProbe()).toEqual([])
      expect(Object.hasOwn(config, 'constructor')).toBe(false)
      expect(config.constructor).toBe(Object)
      expect(mutated).toBe(false)
    })

    it.each(['constructor', '__proto__', 'toString', 'hasOwnProperty'])(
      'does not shadow Object.prototype member %s',
      (key) => {
        const { config, mutated } = applyWithSearch(`?${key}=x`)
        expect(Object.hasOwn(config, key)).toBe(false)
        expect(Object.getPrototypeOf(config)).toBe(Object.prototype)
        expect(mutated).toBe(false)
      },
    )

    it.each([
      ['userinfo', 'https://evil:x@provider-a.example'],
      ['a script URL', 'javascript:alert(1)'],
      ['a non-http scheme', 'ftp://provider-a.example'],
      ['a query string', 'https://provider-a.example/v1?x=1'],
      ['a fragment', 'https://provider-a.example/v1#x'],
      ['no URL at all', 'provider-a'],
    ])('rejects an api_base with %s', (_name, value) => {
      const { config, mutated } = applyWithSearch(
        `?api_base=${encodeURIComponent(value)}`,
      )
      expect(config.api_base).toBe(DefaultSessionConfig.api_base)
      expect(mutated).toBe(false)
      expect(window.location.search).toBe('')
    })

    it('keeps api_base a string when a URL addresses a child of it', () => {
      const { config, mutated } = applyWithSearch('?api_base.foo=1')
      expect(config.api_base).toBe(DefaultSessionConfig.api_base)
      expect(mutated).toBe(false)
    })

    it('keeps chat_switch an object when a URL sets it directly', () => {
      const { config, mutated } = applyWithSearch('?chat_switch=x')
      expect(config.chat_switch).toEqual(DefaultSessionConfig.chat_switch)
      expect(mutated).toBe(false)
    })

    it('ignores unknown chat_switch children and deeper paths', () => {
      const { config, mutated } = applyWithSearch(
        '?chat_switch.unknown_flag=1&chat_switch.enable_mcp.deep=1',
      )
      expect(config.chat_switch).toEqual(DefaultSessionConfig.chat_switch)
      expect(mutated).toBe(false)
    })

    it('never sets fields that are missing from DefaultSessionConfig', () => {
      const stored = {
        ...baseConfig(),
        api_token_before_reset: 'sk-previous',
      } as SessionConfig & { api_token_before_reset: string }
      const { config, mutated } = applyWithSearch(
        '?api_token_before_reset=sk-attacker',
        stored,
      )
      expect(
        (config as SessionConfig & { api_token_before_reset: string })
          .api_token_before_reset,
      ).toBe('sk-previous')
      expect(mutated).toBe(false)
    })
  })

  describe('keeps legitimate overrides working', () => {
    it('applies aliases and direct fields, then strips them from the URL', () => {
      const { config, mutated } = applyWithSearch(
        '?api_key=sk-url&model=' +
          CustomUrlModel +
          '&temperature=0.5&n_contexts=all&chat_switch.enable_mcp=1' +
          '&session_name=Shared&token_type=direct&chat_switch.agent_mode=true' +
          '&enable-memory=yes&draw=3&max_tokens=128&api_base=https://example.test' +
          '&prompt=Be+brief&foo=bar#frag',
      )
      expect(mutated).toBe(true)
      expect(config.api_token).toBe('sk-url')
      expect(config.selected_model).toBe(CustomUrlModel)
      expect(AllModels).toContain(CustomUrlModel)
      expect(config.temperature).toBe(0.5)
      expect(config.n_contexts).toBe(31)
      expect(config.max_tokens).toBe(128)
      expect(config.api_base).toBe('https://example.test')
      expect(config.system_prompt).toBe('Be brief')
      expect(config.session_name).toBe('Shared')
      expect(config.token_type).toBe('direct')
      expect(config.chat_switch.enable_mcp).toBe(true)
      expect(config.chat_switch.agent_mode).toBe(true)
      expect(config.chat_switch.enable_memory).toBe(true)
      expect(config.chat_switch.draw_n_images).toBe(3)
      expect(config.sync_key).toBe('sync-original')
      expect(window.location.pathname).toBe('/chat')
      expect(window.location.search).toBe('?foo=bar')
      expect(window.location.hash).toBe('#frag')
    })

    it('stores booleans, not strings, for every boolean field', () => {
      const { config } = applyWithSearch(
        '?chat_switch.agent_mode=0&session_visible=false',
        {
          ...baseConfig(),
          session_visible: true,
          chat_switch: {
            ...DefaultSessionConfig.chat_switch,
            agent_mode: true,
          },
        },
      )
      expect(config.chat_switch.agent_mode).toBe(false)
      expect(config.session_visible).toBe(false)
    })

    it('rebuilds a missing chat_switch from defaults for a switch override', () => {
      const stored = { ...baseConfig() } as Partial<SessionConfig>
      delete stored.chat_switch
      const { config, mutated } = applyWithSearch(
        '?enable_mcp=on',
        stored as SessionConfig,
      )
      expect(mutated).toBe(true)
      expect(config.chat_switch).toEqual({
        ...DefaultSessionConfig.chat_switch,
        enable_mcp: true,
      })
    })

    it('keeps the current value when a typed param cannot be parsed', () => {
      const { config, mutated } = applyWithSearch(
        '?temperature=abc&enable_mcp=maybe&api_key=sk-next',
      )
      expect(mutated).toBe(true)
      expect(config.temperature).toBe(DefaultSessionConfig.temperature)
      expect(config.chat_switch.enable_mcp).toBe(false)
      expect(config.api_token).toBe('sk-next')
      expect(window.location.search).toBe('')
    })

    it('strips protected params without applying them', () => {
      const { config, mutated } = applyWithSearch(
        '?sync_key=evil&mcp_servers.0.url=https://evil.test' +
          '&api_token_reset_required=false&updated_at=9&foo=bar',
      )
      expect(mutated).toBe(false)
      expect(config.sync_key).toBe('sync-original')
      expect(config.mcp_servers).toEqual(DefaultSessionConfig.mcp_servers)
      expect(config.updated_at).toBe(DefaultSessionConfig.updated_at)
      expect(window.location.search).toBe('?foo=bar')
    })

    it('leaves unknown params in the URL when nothing changes', () => {
      const { config, mutated } = applyWithSearch('?foo=bar&ref=home')
      expect(mutated).toBe(false)
      expect(config).toEqual(baseConfig())
      expect(window.location.search).toBe('?foo=bar&ref=home')
    })
  })
})
