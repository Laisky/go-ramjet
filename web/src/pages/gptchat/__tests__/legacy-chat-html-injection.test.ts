import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadLegacyChatDeclarations } from './fixtures/legacy-chat-script'

// Markup that becomes a live element (with an event-handler attribute) only if
// a sink parses it as HTML instead of treating it as text.
const IMG_PAYLOAD = '<img src="x" onerror="globalThis.__legacyChatXss = 1">'
const ATTR_BREAKOUT_PAYLOAD = `"><img src="x" onerror="globalThis.__legacyChatXss = 1">`

type ChatData = Record<string, unknown> & { chatID: string }

type PageWindow = Window & {
  Prism?: unknown
  MathJax?: unknown
}

/**
 * mountChatManager renders the legacy chat list skeleton and returns its root.
 *
 * @param chatInnerHTML - Static fixture markup for the single chat item.
 * @returns The element that legacy code knows as chatContainer.
 */
function mountChatManager(chatInnerHTML: string): HTMLElement {
  document.body.innerHTML = `
    <div id="chatContainer">
      <div class="chatManager"><div class="conservations"><div class="chats">
        <div id="chat-1">${chatInnerHTML}</div>
      </div></div></div>
    </div>`
  return document.getElementById('chatContainer') as HTMLElement
}

afterEach(() => {
  document.body.innerHTML = ''
  const page = window as PageWindow
  delete page.Prism
  delete page.MathJax
})

describe('legacy chat.js renderAfterAiResp info line (alert #89)', () => {
  type RenderApi = {
    renderAfterAiResp: (
      chatData: ChatData,
      saveStorage?: boolean,
    ) => Promise<void>
  }

  let chatContainer: HTMLElement
  let apiToken: string

  /**
   * loadRender evaluates renderAfterAiResp with inert page stubs.
   *
   * @returns The evaluated renderAfterAiResp function.
   */
  function loadRender(): RenderApi {
    return loadLegacyChatDeclarations<RenderApi>(['renderAfterAiResp'], {
      chatContainer,
      muRenderAfterAiRespForChatID: {},
      splitReasoningContentForUI: () => ({ thinking: '', toolEvents: [] }),
      renderHTML: async (markdown: string) => markdown,
      renderToolEventsHTML: () => '',
      latestRenderHTMLResult: '',
      ChatTaskStatusDone: 'done',
      getChatSessionConfig: async () => ({ api_token: apiToken }),
      libs: { EnableTooltipsEverywhere: () => {} },
      scrollToChat: () => {},
      saveChats2Storage: async () => {},
      setChatData: async () => {},
      bindImageOperationInAiResp: () => {},
      addOperateBtnBelowAiResponse: async () => {},
      restoreUserInputDefaultStateAndFocus: () => {},
    })
  }

  beforeEach(() => {
    chatContainer = mountChatManager('<div class="ai-response"></div>')
    apiToken = 'FREETIER-placeholder'
    const page = window as PageWindow
    page.Prism = { highlightAllUnder: () => {} }
    page.MathJax = { Hub: { Queue: () => {} } }
  })

  it('renders a model name taken from ?chatmodel= as text, not markup', async () => {
    const { renderAfterAiResp } = loadRender()

    await renderAfterAiResp({ chatID: 'chat-1', model: IMG_PAYLOAD })

    const info = chatContainer.querySelector('.ai-response > div.info')
    expect(info?.querySelector('img')).toBeNull()
    expect(info?.querySelector('i.model')?.textContent).toBe(IMG_PAYLOAD)
  })

  it('renders a stored cost value as text, not markup', async () => {
    apiToken = 'laisky-placeholder'
    const { renderAfterAiResp } = loadRender()

    await renderAfterAiResp({
      chatID: 'chat-1',
      model: 'gpt-test',
      costUsd: IMG_PAYLOAD,
    })

    const info = chatContainer.querySelector('.ai-response > div.info')
    expect(info?.querySelector('img')).toBeNull()
    expect(info?.querySelector('i.cost')?.textContent).toBe(`$${IMG_PAYLOAD}`)
  })

  it('keeps the info line markup identical for ordinary values', async () => {
    apiToken = 'laisky-placeholder'
    const { renderAfterAiResp } = loadRender()

    await renderAfterAiResp({
      chatID: 'chat-1',
      model: 'gpt-test',
      costUsd: '0.01',
    })
    await renderAfterAiResp({
      chatID: 'chat-1',
      model: 'gpt-test',
      costUsd: '0.01',
    })

    const infos = chatContainer.querySelectorAll('.ai-response > div.info')
    expect(infos).toHaveLength(1)
    expect(infos[0].outerHTML).toBe(
      '<div class="info"><i class="model">gpt-test</i><i class="cost">$0.01</i></div>',
    )
  })

  it('renders an empty model slot when the model is missing', async () => {
    const { renderAfterAiResp } = loadRender()

    await renderAfterAiResp({ chatID: 'chat-1' })

    expect(
      chatContainer.querySelector('.ai-response > div.info')?.outerHTML,
    ).toBe('<div class="info"><i class="model"></i></div>')
  })
})

describe('legacy chat.js reloadAiResp human prompt (alert #86)', () => {
  type ReloadApi = {
    reloadAiResp: (
      chatID: string,
      overwriteSendChat2Server?: () => Promise<void>,
    ) => Promise<void>
  }

  let selectedModel: string
  let storedChatData: Record<string, unknown>

  /**
   * loadReload evaluates reloadAiResp against the mounted chat list.
   *
   * @param chatContainer - Root element that contains the chat item.
   * @returns The evaluated reloadAiResp function.
   */
  function loadReload(chatContainer: HTMLElement): ReloadApi {
    return loadLegacyChatDeclarations<ReloadApi>(['reloadAiResp'], {
      chatContainer,
      muRenderAfterAiRespForChatID: {},
      OpenaiSelectedModel: async () => selectedModel,
      getChatData: async () => storedChatData,
      setChatData: async () => {},
      RoleAI: 'assistant',
      robotIcon: '🤖️',
      ChatTaskStatusWaiting: 'waiting',
      deleteBtnHandler: () => {},
      bindEditHumanInput: () => {},
      sendChat2Server: async () => {},
    })
  }

  beforeEach(() => {
    selectedModel = 'gpt-test'
    storedChatData = { taskType: 'chat' }
  })

  it('treats an edited prompt from the textarea as text, not markup', async () => {
    const chatContainer = mountChatManager(
      '<div class="role-human"><textarea></textarea></div>',
    )
    const textarea = chatContainer.querySelector(
      'textarea',
    ) as HTMLTextAreaElement
    textarea.value = IMG_PAYLOAD
    const { reloadAiResp } = loadReload(chatContainer)

    await reloadAiResp('chat-1', async () => {})

    const pre = chatContainer.querySelector(
      '#chat-1 .role-human .text-start pre',
    )
    expect(pre?.querySelector('img')).toBeNull()
    expect(pre?.textContent).toBe(IMG_PAYLOAD)
  })

  it('keeps the existing escaped prompt unchanged on a plain reload', async () => {
    const chatContainer = mountChatManager(
      '<div class="role-human"><div class="text-start"><pre>a &lt;b&gt; &amp; c</pre></div></div>',
    )
    const { reloadAiResp } = loadReload(chatContainer)

    await reloadAiResp('chat-1', async () => {})

    const pre = chatContainer.querySelector(
      '#chat-1 .role-human .text-start pre',
    )
    expect(pre?.innerHTML).toBe('a &lt;b&gt; &amp; c')
    expect(pre?.textContent).toBe('a <b> & c')
  })

  it('keeps model and task type attributes inert', async () => {
    selectedModel = ATTR_BREAKOUT_PAYLOAD
    storedChatData = { taskType: ATTR_BREAKOUT_PAYLOAD }
    const chatContainer = mountChatManager(
      '<div class="role-human"><textarea>hello</textarea></div>',
    )
    const { reloadAiResp } = loadReload(chatContainer)

    await reloadAiResp('chat-1', async () => {})

    const ai = chatContainer.querySelector(
      '#chat-1 .role-ai .ai-response',
    ) as HTMLElement
    expect(chatContainer.querySelector('#chat-1 img')).toBeNull()
    expect(ai.dataset.model).toBe(ATTR_BREAKOUT_PAYLOAD)
    expect(ai.dataset.taskType).toBe(ATTR_BREAKOUT_PAYLOAD)
    expect(ai.dataset.taskStatus).toBe('waiting')
    expect(ai.querySelector('.placeholder-glow')).not.toBeNull()
  })

  it('still rebinds the human controls and sends the request', async () => {
    const chatContainer = mountChatManager(
      '<div class="role-human"><textarea>hello</textarea></div>',
    )
    const send = vi.fn(async () => {})
    const { reloadAiResp } = loadReload(chatContainer)

    await reloadAiResp('chat-1', send)

    expect(send).toHaveBeenCalledTimes(1)
    expect(
      chatContainer
        .querySelector('#chat-1 .role-human')
        ?.getAttribute('data-chatid'),
    ).toBe('chat-1')
    expect(
      chatContainer.querySelector('#chat-1 .role-human .control .bi-trash'),
    ).not.toBeNull()
  })
})

describe('legacy chat.js appendPromptShortcut badge (alert #87)', () => {
  type ShortcutApi = {
    appendPromptShortcut: (
      shortcut: { title: string; description: string },
      storage?: boolean,
    ) => Promise<void>
  }

  let configContainer: HTMLElement
  let stored: unknown[]
  const editHandler = vi.fn()

  /**
   * loadShortcuts evaluates appendPromptShortcut with an in-memory KV.
   *
   * @returns The evaluated appendPromptShortcut function.
   */
  function loadShortcuts(): ShortcutApi {
    return loadLegacyChatDeclarations<ShortcutApi>(['appendPromptShortcut'], {
      configContainer,
      loadPromptShortcutsFromStorage: async () => [...stored],
      KvKeyPromptShortCuts: 'config_prompt_shortcuts',
      libs: {
        KvGet: async () => stored,
        KvSet: async (_key: string, value: unknown[]) => {
          stored = value
        },
        evtTarget: (evt: Event) => evt.currentTarget || evt.target,
      },
      EditFavSystemPromptHandler: editHandler,
      ConfirmModal: () => {},
      OpenaiChatStaticContext: async () => {},
    })
  }

  beforeEach(() => {
    document.body.innerHTML = `
      <div id="config">
        <div class="prompt-shortcuts"></div>
        <div class="system-prompt"><textarea class="input"></textarea></div>
      </div>`
    configContainer = document.getElementById('config') as HTMLElement
    stored = []
    editHandler.mockReset()
  })

  it('renders a saved shortcut title as text, not markup', async () => {
    const { appendPromptShortcut } = loadShortcuts()

    await appendPromptShortcut(
      { title: IMG_PAYLOAD, description: 'desc' },
      true,
    )

    const badge = configContainer.querySelector(
      '.prompt-shortcuts .badge',
    ) as HTMLElement
    expect(badge.querySelector('img')).toBeNull()
    expect(badge.querySelector('i.title')?.textContent).toBe(IMG_PAYLOAD)
    expect(badge.dataset.title).toBe(IMG_PAYLOAD)
    expect(stored).toEqual([{ title: IMG_PAYLOAD, description: 'desc' }])
  })

  it('keeps the badge markup and handlers for ordinary titles', async () => {
    const { appendPromptShortcut } = loadShortcuts()

    await appendPromptShortcut({
      title: 'Translator',
      description: 'translate',
    })

    const badge = configContainer.querySelector(
      '.prompt-shortcuts .badge',
    ) as HTMLElement
    expect(badge.innerHTML).toBe(
      '<i class="title">Translator</i>  <i class="bi bi-pencil-square"></i><i class="bi bi-trash"></i>',
    )
    badge.querySelector<HTMLElement>('i.bi-pencil-square')?.click()
    expect(editHandler).toHaveBeenCalledTimes(1)

    badge.click()
    const promptInput = configContainer.querySelector(
      '.system-prompt .input',
    ) as HTMLTextAreaElement
    await vi.waitFor(() => expect(promptInput.value).toBe('translate'))
  })
})
