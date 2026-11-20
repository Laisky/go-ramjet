import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { loadLegacyChatDeclarations } from './fixtures/legacy-chat-script'

type PageWindow = Window & { bootstrap?: unknown; chatPrompts?: unknown }

type PromptManagerApi = {
  setupPromptManager: () => Promise<void>
  readonly systemPromptModalCallback?: (evt: Event) => Promise<void>
}

type Shortcut = { title: string; description: string }

/**
 * mountPromptManager renders the elements setupPromptManager binds to.
 *
 * @returns The element that legacy code knows as configContainer.
 */
function mountPromptManager(): HTMLElement {
  document.body.innerHTML = `
    <div id="config">
      <div class="prompt-shortcuts"><span class="badge" data-prompt="p"></span></div>
      <div class="system-prompt">
        <textarea class="input"></textarea>
        <i class="bi save-prompt"></i>
        <i class="bi open-prompt-market"></i>
      </div>
    </div>
    <div id="save-system-prompt" class="modal">
      <div class="modal-body">
        <input class="title">
        <textarea class="user-input"></textarea>
      </div>
    </div>
    <div id="prompt-market" class="modal">
      <div class="prompt-labels"></div>
      <textarea class="prompt-content"></textarea>
      <input class="prompt-title">
      <div class="modal-body"><button class="save"></button></div>
    </div>`
  return document.getElementById('config') as HTMLElement
}

// CodeQL alert #88 flags the save-prompt modal prefill. A textarea never
// parses markup into elements, so it is not XSS, but writing the prompt with
// innerHTML decoded entities and stopped updating the textarea once its value
// had been set, so these tests pin the prefill as an exact value copy.
describe('legacy chat.js save-prompt modal prefill (alert #88)', () => {
  let configContainer: HTMLElement
  let api: PromptManagerApi
  let saved: Shortcut[]

  /**
   * openSaveModal types prompt into the system prompt input and clicks the
   * star icon that opens the save-prompt modal.
   *
   * @param prompt - The system prompt text to save.
   * @returns The modal's description textarea.
   */
  function openSaveModal(prompt: string): HTMLTextAreaElement {
    const promptInput = configContainer.querySelector(
      '.system-prompt .input',
    ) as HTMLTextAreaElement
    promptInput.value = prompt
    configContainer
      .querySelector<HTMLElement>('.system-prompt .bi.save-prompt')
      ?.click()
    return document.querySelector(
      '#save-system-prompt textarea.user-input',
    ) as HTMLTextAreaElement
  }

  beforeEach(async () => {
    configContainer = mountPromptManager()
    saved = []
    const page = window as PageWindow
    page.bootstrap = {
      Modal: class {
        show() {}
        hide() {}
      },
    }
    page.chatPrompts = []
    api = loadLegacyChatDeclarations<PromptManagerApi>(
      ['setupPromptManager'],
      {
        configContainer,
        loadPromptShortcutsFromStorage: async () => [],
        appendPromptShortcut: async (shortcut: Shortcut) => {
          saved.push(shortcut)
        },
        OpenaiChatStaticContext: async () => {},
        libs: {
          evtTarget: (evt: Event) => evt.currentTarget || evt.target,
        },
      },
      ['systemPromptModalCallback'],
    )
    await api.setupPromptManager()
  })

  afterEach(() => {
    document.body.innerHTML = ''
    const page = window as PageWindow
    delete page.bootstrap
    delete page.chatPrompts
  })

  it('copies the prompt verbatim, keeping entities and markup as text', async () => {
    const prompt = `Use &amp;amp; and &lt;tag&gt; literally. </textarea><img src="x" onerror="globalThis.__legacyChatXss = 1"><script>globalThis.__legacyChatXss = 1</script>`

    const textarea = openSaveModal(prompt)

    await vi.waitFor(() => expect(textarea.value).toBe(prompt))
    expect(textarea.children).toHaveLength(0)
    expect(document.querySelector('#save-system-prompt img, script')).toBeNull()
    expect(
      (globalThis as { __legacyChatXss?: number }).__legacyChatXss,
    ).toBeUndefined()
  })

  it('prefills the current prompt again after a previous save', async () => {
    const title = document.querySelector(
      '#save-system-prompt input.title',
    ) as HTMLInputElement

    const first = openSaveModal('first prompt')
    await vi.waitFor(() => expect(first.value).toBe('first prompt'))
    title.value = 'First'
    await api.systemPromptModalCallback?.(new Event('click'))
    expect(saved).toEqual([{ title: 'First', description: 'first prompt' }])
    expect(first.value).toBe('')

    const second = openSaveModal('second prompt')

    await vi.waitFor(() => expect(second.value).toBe('second prompt'))
  })
})
