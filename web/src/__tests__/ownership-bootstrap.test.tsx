import { webcrypto } from 'node:crypto'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const bootstrap = vi.hoisted(() => {
  const observations: Array<{
    stage: string
    url: string
    session: string | null
    cvSession: string | null
  }> = []
  const observe = (stage: string) => {
    observations.push({
      stage,
      url:
        window.location.pathname +
        window.location.search +
        window.location.hash,
      session: window.localStorage.getItem('gptchat_ownership_sso_token'),
      cvSession: window.localStorage.getItem('cv_sso_token'),
    })
  }
  const render = vi.fn(() => observe('render'))
  const createRoot = vi.fn(() => {
    observe('createRoot')
    return { render }
  })
  return { observations, observe, createRoot, render }
})

vi.mock('react-dom/client', () => ({
  default: { createRoot: bootstrap.createRoot },
  createRoot: bootstrap.createRoot,
}))
vi.mock('@/site/site-meta', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/site/site-meta')>()
  return {
    ...actual,
    bootstrapSite: () => {
      bootstrap.observe('bootstrapSite')
      return actual.bootstrapSite()
    },
  }
})

describe('actual SPA code callback bootstrap', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    vi.stubEnv('VITE_SITE_ID', '')
    vi.stubGlobal('crypto', webcrypto)
    bootstrap.observations.length = 0
    localStorage.clear()
    sessionStorage.clear()
    document.head.innerHTML = '<meta name="ramjet-site" content="chat">'
    document.body.innerHTML = '<div id="root"></div>'
    delete document.documentElement.dataset.site
    history.replaceState(null, '', '/gptchat?tab=data#datasets')
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.unstubAllEnvs()
  })

  it('exchanges once before metadata and React mounting, preserving return path and separate CV session', async () => {
    const { beginCodeSignIn } = await import('@/lib/sso-code')
    const login = new URL(await beginCodeSignIn('chat'))
    const callback = new URL(login.searchParams.get('redirect_to')!)
    localStorage.setItem('cv_sso_token', 'independent-cv-session')
    history.replaceState(
      null,
      '',
      '/?sso_code=single-use&sso_state=' +
        callback.searchParams.get('sso_state'),
    )
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        expect(location.search).toBe('')
        expect(bootstrap.observations).toHaveLength(0)
        return new Response(
          JSON.stringify({
            access_token: 'original.synthetic.session',
            token_type: 'Bearer',
          }),
        )
      }),
    )
    await (
      await import('@/main')
    ).ready
    expect(bootstrap.observations.map(({ stage }) => stage)).toEqual([
      'bootstrapSite',
      'createRoot',
      'render',
    ])
    for (const observed of bootstrap.observations) {
      expect(observed.url).toBe('/gptchat?tab=data#datasets')
      expect(observed.session).toBe('original.synthetic.session')
      expect(observed.cvSession).toBe('independent-cv-session')
    }
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it.each(['chat', 'cv', 'default'])(
    'scrubs and rejects legacy tokens before any app mounts on %s',
    async (site) => {
      document
        .querySelector('meta[name="ramjet-site"]')!
        .setAttribute('content', site)
      localStorage.setItem('gptchat_ownership_sso_token', 'existing-session')
      history.replaceState(null, '', '/?sso_token=forged&state=legacy')
      vi.stubGlobal('fetch', vi.fn())
      await (
        await import('@/main')
      ).ready
      expect(bootstrap.render).toHaveBeenCalledTimes(1)
      for (const observed of bootstrap.observations) {
        expect(observed.url).toBe('/')
        expect(observed.session).toBe('existing-session')
      }
      expect(fetch).not.toHaveBeenCalled()
    },
  )
})
