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

const pendingState = 'a'.repeat(64)

describe('actual SPA ownership callback bootstrap', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    vi.stubEnv('VITE_SITE_ID', '')
    bootstrap.observations.length = 0
    localStorage.clear()
    sessionStorage.clear()
    document.head.innerHTML = '<meta name="ramjet-site" content="default">'
    document.body.innerHTML = '<div id="root"></div>'
    delete document.documentElement.dataset.site
    history.replaceState(null, '', '/gptchat')
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        throw new Error('No network is expected before app mounting.')
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.unstubAllEnvs()
  })

  it.each(['/gptchat', '/gptchat/', '/GPTCHAT'])(
    'admits a matching callback at %s before metadata and React mounting',
    async (path) => {
      sessionStorage.setItem(
        'gptchat_ownership_sso_pending_state',
        pendingState,
      )
      localStorage.setItem('cv_sso_token', 'independent-cv-session')
      history.replaceState(
        null,
        '',
        path +
          '?tab=data&sso_token=original.synthetic.session&state=' +
          pendingState +
          '#datasets',
      )
      await import('@/main')
      expect(bootstrap.observations.map(({ stage }) => stage)).toEqual([
        'bootstrapSite',
        'createRoot',
        'render',
      ])
      for (const observed of bootstrap.observations) {
        expect(observed.url).toBe(path + '?tab=data#datasets')
        expect(observed.session).toBe('original.synthetic.session')
        expect(observed.cvSession).toBe('independent-cv-session')
      }
      expect(
        sessionStorage.getItem('gptchat_ownership_sso_pending_state'),
      ).toBeNull()
      expect(bootstrap.createRoot).toHaveBeenCalledWith(
        document.getElementById('root'),
      )
      expect(bootstrap.render).toHaveBeenCalledTimes(1)
      expect(fetch).not.toHaveBeenCalled()
    },
  )

  it('rejects and scrubs a mismatched callback before mounting without replacing the established session', async () => {
    localStorage.setItem('gptchat_ownership_sso_token', 'existing-session')
    sessionStorage.setItem('gptchat_ownership_sso_pending_state', pendingState)
    history.replaceState(
      null,
      '',
      '/gptchat?tab=data&sso_token=untrusted-session&state=' + 'b'.repeat(64),
    )
    await import('@/main')
    for (const observed of bootstrap.observations) {
      expect(observed.url).toBe('/gptchat?tab=data')
      expect(observed.session).toBe('existing-session')
    }
    expect(bootstrap.observations.map(({ stage }) => stage)).toEqual([
      'bootstrapSite',
      'createRoot',
      'render',
    ])
    expect(
      sessionStorage.getItem('gptchat_ownership_sso_pending_state'),
    ).toBeNull()
    expect(fetch).not.toHaveBeenCalled()
  })

  it('admits the chat site landing route because SiteLanding mounts GPTChat there', async () => {
    document
      .querySelector('meta[name="ramjet-site"]')!
      .setAttribute('content', 'chat')
    sessionStorage.setItem('gptchat_ownership_sso_pending_state', pendingState)
    history.replaceState(
      null,
      '',
      '/?sso_token=root-chat-session&state=' + pendingState,
    )
    await import('@/main')
    for (const observed of bootstrap.observations) {
      expect(observed.url).toBe('/')
      expect(observed.session).toBe('root-chat-session')
    }
    expect(bootstrap.render).toHaveBeenCalledTimes(1)
  })

  it.each([
    { path: '/cv', site: 'default' },
    { path: '/', site: 'cv' },
    { path: '/cv', site: 'chat' },
    { path: '/', site: 'default' },
    { path: '/gptchat-other', site: 'chat' },
    { path: '/tasks/gptchat', site: 'default' },
  ])(
    'leaves another application callback untouched at $path on $site',
    async ({ path, site }) => {
      document
        .querySelector('meta[name="ramjet-site"]')!
        .setAttribute('content', site)
      localStorage.setItem('cv_sso_token', 'existing-cv-session')
      sessionStorage.setItem(
        'gptchat_ownership_sso_pending_state',
        pendingState,
      )
      const callback =
        path + '?sso_token=other-app-session&state=' + pendingState
      history.replaceState(null, '', callback)
      await import('@/main')
      for (const observed of bootstrap.observations) {
        expect(observed.url).toBe(callback)
        expect(observed.session).toBeNull()
        expect(observed.cvSession).toBe('existing-cv-session')
      }
      expect(
        sessionStorage.getItem('gptchat_ownership_sso_pending_state'),
      ).toBe(pendingState)
      expect(bootstrap.render).toHaveBeenCalledTimes(1)
      expect(fetch).not.toHaveBeenCalled()
    },
  )
})
