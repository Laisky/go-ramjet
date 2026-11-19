import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { loadLegacyChatDeclarations } from './fixtures/legacy-chat-script'

type RoundTripApi = {
  reloadAiResp: (chatID: string) => Promise<void>
  appendImg2UserInput: (
    chatID: string,
    imgDataBase64: string,
    imgName: string,
  ) => Promise<void>
}

// userPromptEnhance is the first thing sendChat2Server does with the prompt
// it read back from the page; the stub records it and then aborts the request
// so the test needs no network, model, or streaming stubs.
const STOP_AFTER_PROMPT = new Error('stop after the prompt is captured')

// A prompt whose HTML serialization differs from its text in every way that
// matters: tags, a literal entity, and bare special characters.
const PROMPT_WITH_MARKUP = '<div>a &amp; b</div> & <b> "quoted" 1 < 2'

/**
 * mountHumanChat renders one chat item whose human side holds innerHTML.
 *
 * @param humanInnerHTML - Static fixture markup inside the .role-human node.
 * @returns The element that legacy code knows as chatContainer.
 */
function mountHumanChat(humanInnerHTML: string): HTMLElement {
  document.body.innerHTML = `
    <div id="chatContainer">
      <div class="chatManager"><div class="conservations"><div class="chats">
        <div id="chat-1"><div class="role-human">${humanInnerHTML}</div></div>
      </div></div></div>
    </div>`
  return document.getElementById('chatContainer') as HTMLElement
}

describe('legacy chat.js prompt round trip through the page (alert #86)', () => {
  let sentPrompts: string[]
  let savedChats: Array<Record<string, unknown>>

  /**
   * loadRoundTrip evaluates the real reloadAiResp -> sendChat2Server path and
   * appendImg2UserInput against the mounted chat item.
   *
   * @param chatContainer - Root element that contains the chat item.
   * @returns The evaluated functions.
   */
  function loadRoundTrip(chatContainer: HTMLElement): RoundTripApi {
    return loadLegacyChatDeclarations<RoundTripApi>(
      ['reloadAiResp', 'sendChat2Server', 'appendImg2UserInput'],
      {
        chatContainer,
        muRenderAfterAiRespForChatID: {},
        ensureChatPromptElements: () => {},
        OpenaiSelectedModel: async () => 'gpt-test',
        getChatData: async () => ({ taskType: 'chat' }),
        setChatData: async () => {},
        saveChats2Storage: async (chatData: Record<string, unknown>) => {
          savedChats.push(chatData)
        },
        userPromptEnhance: async (prompt: string) => {
          sentPrompts.push(prompt)
          throw STOP_AFTER_PROMPT
        },
        RoleAI: 'assistant',
        RoleHuman: 'user',
        robotIcon: '🤖️',
        ChatTaskStatusWaiting: 'waiting',
        deleteBtnHandler: () => {},
        bindEditHumanInput: () => {},
      },
    )
  }

  beforeEach(() => {
    sentPrompts = []
    savedChats = []
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('sends exactly the edited text when an edited prompt is reloaded', async () => {
    const chatContainer = mountHumanChat('<textarea></textarea>')
    const textarea = chatContainer.querySelector(
      'textarea',
    ) as HTMLTextAreaElement
    textarea.value = PROMPT_WITH_MARKUP
    const { reloadAiResp } = loadRoundTrip(chatContainer)

    await expect(reloadAiResp('chat-1')).rejects.toBe(STOP_AFTER_PROMPT)

    expect(sentPrompts).toEqual([PROMPT_WITH_MARKUP])
    expect(
      chatContainer.querySelector('#chat-1 .role-human .text-start pre')
        ?.textContent,
    ).toBe(PROMPT_WITH_MARKUP)
  })

  it('sends exactly the displayed text when a prompt is reloaded unedited', async () => {
    // append2Chats renders stored prompts through libs.escapeHtml.
    const chatContainer = mountHumanChat(
      '<div class="text-start"><pre>&lt;div&gt;a &amp;amp; b&lt;/div&gt; &amp; &lt;b&gt; "quoted" 1 &lt; 2</pre></div>',
    )
    const { reloadAiResp } = loadRoundTrip(chatContainer)

    await expect(reloadAiResp('chat-1')).rejects.toBe(STOP_AFTER_PROMPT)

    expect(sentPrompts).toEqual([PROMPT_WITH_MARKUP])
  })

  it('stores the prompt text, not its HTML, when pinning an image', async () => {
    const chatContainer = mountHumanChat(
      '<div class="text-start"><pre>&lt;div&gt;a &amp;amp; b&lt;/div&gt; &amp; &lt;b&gt; "quoted" 1 &lt; 2</pre></div>',
    )
    const { appendImg2UserInput } = loadRoundTrip(chatContainer)

    await appendImg2UserInput('chat-1', 'AAAA', 'pinned.png')

    expect(savedChats).toHaveLength(1)
    expect(savedChats[0].content).toBe(PROMPT_WITH_MARKUP)
    expect(savedChats[0].role).toBe('user')
  })
})
