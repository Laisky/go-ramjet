import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import {
  loadLegacyChatDeclarations,
  loadLegacyLibs,
} from './fixtures/legacy-chat-script'

// Markup that becomes a live element (with an event-handler attribute) only if
// a session-name sink parses it as HTML instead of treating it as text.
const IMG_PAYLOAD = '<img src="x" onerror="globalThis.__legacyChatXss = 1">'
const ATTR_BREAKOUT_PAYLOAD = `"><img src="x" onerror="globalThis.__legacyChatXss = 1">`

type LegacyLibs = { escapeHtml: (value: unknown) => string }
type SessionConfig = Record<string, unknown> & { session_name?: string }

const legacyLibs = loadLegacyLibs<LegacyLibs>(['escapeHtml'])

/**
 * mountSessionPage renders the legacy session manager and chat skeleton.
 *
 * @param sessionManagerInner - Markup inside `#sessionManager .sessions`.
 * @param tabsInner - Markup inside the chat container's session tabs.
 * @returns The element that legacy code knows as chatContainer.
 */
function mountSessionPage(
  sessionManagerInner = '',
  tabsInner = '',
): HTMLElement {
  document.body.innerHTML = `
    <div id="sessionManager">
      <button class="btn purge"></button>
      <button class="btn new-session"></button>
      <div class="sessions">${sessionManagerInner}</div>
    </div>
    <div id="chatContainer">
      <div class="sessions"><div class="session-tabs">${tabsInner}</div></div>
      <div class="chatManager">
        <div class="conservations"><div class="chats"></div></div>
        <div class="card-footer"><button class="scroll-down"></button></div>
      </div>
    </div>`
  return document.getElementById('chatContainer') as HTMLElement
}

/**
 * sessionItems returns the session list entries for sid in both lists.
 *
 * @param sid - Session ID.
 * @returns The session manager entry and the chat tab entry.
 */
function sessionItems(sid: number): HTMLElement[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>(`.session[data-session="${sid}"]`),
  )
}

/**
 * expectSessionNameAsText asserts that both lists show name for sid as text.
 *
 * @param sid - Session ID.
 * @param name - The exact session name.
 */
function expectSessionNameAsText(sid: number, name: string) {
  expect(document.querySelector('img')).toBeNull()
  const items = sessionItems(sid)
  expect(items).toHaveLength(2)
  items.forEach((item) => {
    expect(item.querySelector('.col')?.textContent).toBe(name)
    expect(item.dataset.name).toBe(name)
  })
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('legacy chat.js session names are rendered as text', () => {
  let configs: Record<number, SessionConfig>
  let kvKeys: string[]

  beforeEach(() => {
    configs = {}
    kvKeys = []
  })

  /**
   * pageScope returns the page globals the session manager code references.
   *
   * @param chatContainer - Root element of the chat page.
   * @returns Free variables for loadLegacyChatDeclarations.
   */
  function pageScope(chatContainer: HTMLElement) {
    return {
      chatContainer,
      libs: {
        escapeHtml: legacyLibs.escapeHtml,
        evtTarget: (evt: Event) => evt.currentTarget || evt.target,
        KvList: async () => kvKeys,
        KvGet: async () => [],
        KvSet: async () => {},
      },
      KvKeyPrefixSessionHistory: 'chat_user_session_',
      KvKeyPrefixSessionConfig: 'chat_user_config_',
      kvSessionKey: (sid: number) => `chat_user_session_${sid}`,
      activeSessionID: async () => 1,
      getChatSessionConfig: async (sid: number) => configs[sid],
      saveChatSessionConfig: async (sconfig: SessionConfig, sid: number) => {
        configs[sid] = sconfig
      },
      sessionChatHistory: async () => [],
      newSessionConfig: () => ({}),
      changeSession: async () => {},
      clearSessionHandler: () => {},
      scrollChatToDown: () => {},
      listenSessionSwitch: () => {},
      bindSessionEditBtn: () => {},
      bindSessionDeleteBtn: () => {},
      bindSessionDuplicateBtn: () => {},
      updateConfigFromSessionConfig: async () => {},
      showalert: () => {},
    }
  }

  it.each([IMG_PAYLOAD, ATTR_BREAKOUT_PAYLOAD])(
    'restores the stored session name %s as text',
    async (name) => {
      const chatContainer = mountSessionPage()
      configs[1] = { session_name: name }
      kvKeys = ['chat_user_session_1']
      const { setupSessionManager } = loadLegacyChatDeclarations<{
        setupSessionManager: () => Promise<void>
      }>(['setupSessionManager'], pageScope(chatContainer))

      await setupSessionManager()

      expectSessionNameAsText(1, name)
      expect(
        sessionItems(1)[0].querySelector('.list-group-item.active'),
      ).not.toBeNull()
    },
  )

  it('falls back to the session ID when the session has no name', async () => {
    const chatContainer = mountSessionPage()
    configs[3] = {}
    kvKeys = ['chat_user_session_3']
    const { setupSessionManager } = loadLegacyChatDeclarations<{
      setupSessionManager: () => Promise<void>
    }>(['setupSessionManager'], pageScope(chatContainer))

    await setupSessionManager()

    expectSessionNameAsText(3, '3')
  })

  it('writes a renamed session name as text', async () => {
    const item = (withControls: boolean) => `
      <div class="list-group session" data-session="1" data-name="1">
        <button type="button" class="list-group-item"><div class="col">1</div>
        ${withControls ? '<i class="bi bi-pencil-square"></i>' : ''}</button>
      </div>`
    const chatContainer = mountSessionPage(item(true), item(false))
    configs[1] = {}
    const { bindSessionEditBtn } = loadLegacyChatDeclarations<{
      bindSessionEditBtn: () => void
    }>(['bindSessionEditBtn'], {
      ...pageScope(chatContainer),
      SingleInputModal: (
        _title: string,
        _label: string,
        callback: (value: string) => Promise<void>,
      ) => callback(IMG_PAYLOAD),
    })

    bindSessionEditBtn()
    document
      .querySelector<HTMLElement>('#sessionManager .bi-pencil-square')
      ?.click()

    await expect.poll(() => configs[1].session_name).toBe(IMG_PAYLOAD)
    expect(document.querySelector('img')).toBeNull()
    const [managerItem, tabItem] = sessionItems(1)
    expect(managerItem.querySelector('.col')?.textContent).toBe(IMG_PAYLOAD)
    expect(tabItem.querySelector('.col')?.textContent).toBe(IMG_PAYLOAD)
    // Only the session manager entry keeps the name as a data attribute.
    expect(managerItem.dataset.name).toBe(IMG_PAYLOAD)
  })

  it('renders a duplicated session name as text', async () => {
    const chatContainer = mountSessionPage(`
      <div class="list-group session" data-session="1" data-name="x">
        <button type="button" class="list-group-item"><div class="col">x</div>
        <i class="bi bi-files"></i></button>
      </div>`)
    configs[1] = { session_name: ATTR_BREAKOUT_PAYLOAD }
    kvKeys = ['chat_user_session_1']
    const { bindSessionDuplicateBtn } = loadLegacyChatDeclarations<{
      bindSessionDuplicateBtn: () => void
    }>(['bindSessionDuplicateBtn'], pageScope(chatContainer))

    bindSessionDuplicateBtn()
    document.querySelector<HTMLElement>('#sessionManager .bi-files')?.click()

    await expect.poll(() => sessionItems(2).length).toBe(2)
    expectSessionNameAsText(2, `${ATTR_BREAKOUT_PAYLOAD}_copy`)
  })

  it.each([IMG_PAYLOAD, ATTR_BREAKOUT_PAYLOAD])(
    'asks to delete the session named %s as text',
    async (name) => {
      const item = (sid: number) => `
        <div class="list-group session" data-session="${sid}">
          <button type="button" class="list-group-item"><div class="col"></div>
          <i class="bi bi-trash col-auto"></i></button>
        </div>`
      const chatContainer = mountSessionPage(`${item(1)}${item(2)}`)
      // data-name holds the decoded name, exactly as the session list leaves it.
      const [target] = sessionItems(2)
      target.dataset.name = name
      document.body.insertAdjacentHTML(
        'beforeend',
        '<div id="deleteCheckModal"><h5 class="modal-title"></h5></div>',
      )
      let shown = false
      const { bindSessionDeleteBtn } = loadLegacyChatDeclarations<{
        bindSessionDeleteBtn: () => void
      }>(
        ['ConfirmModal', 'bindSessionDeleteBtn'],
        {
          ...pageScope(chatContainer),
          ShowSpinner: () => {},
          HideSpinner: () => {},
          deleteCheckModal: {
            show: () => {
              shown = true
            },
          },
        },
        ['deleteCheckCallback'],
      )

      bindSessionDeleteBtn()
      target.querySelector<HTMLElement>('.bi-trash')?.click()

      await expect.poll(() => shown).toBe(true)
      expect(document.querySelector('img')).toBeNull()
      expect(
        document.querySelector('#deleteCheckModal .modal-title')?.textContent,
      ).toBe(`Are you sure to delete session "${name}"?`)
    },
  )
})
