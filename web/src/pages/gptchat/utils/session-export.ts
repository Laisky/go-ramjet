import { kvGet } from '@/utils/storage'
import { sanitizeChatMessageData } from '../hooks/chat-storage'
import type {
  ChatMessageData,
  SessionConfig,
  SessionHistoryItem,
} from '../types'
import { getChatDataKey, getSessionHistoryKey } from './chat-storage'
import { getSessionConfigKey } from './config-helpers'

/**
 * Escapes special characters for XML.
 */
function escapeXml(unsafe: unknown): string {
  if (unsafe === undefined || unsafe === null) return ''
  const str = String(unsafe)
  return str.replace(/[<>&'"]/g, (c) => {
    switch (c) {
      case '<':
        return '&lt;'
      case '>':
        return '&gt;'
      case '&':
        return '&amp;'
      case "'":
        return '&apos;'
      case '"':
        return '&quot;'
      default:
        return c
    }
  })
}

/**
 * Formats a message as an XML string.
 */
function formatMessageAsXml(msg: ChatMessageData): string {
  let xml = '    <message>\n'
  xml += `      <chat_id>${escapeXml(msg.chatID)}</chat_id>\n`
  xml += `      <role>${escapeXml(msg.role)}</role>\n`
  if (msg.timestamp) {
    xml += `      <time>${escapeXml(new Date(msg.timestamp).toISOString())}</time>\n`
    xml += `      <timestamp>${msg.timestamp}</timestamp>\n`
  }
  if (msg.model) {
    xml += `      <model>${escapeXml(msg.model)}</model>\n`
  }
  if (msg.costUsd !== undefined) {
    xml += `      <cost_usd>${msg.costUsd}</cost_usd>\n`
  }
  if (msg.requestid) {
    xml += `      <request_id>${escapeXml(msg.requestid)}</request_id>\n`
  }
  xml += `      <content>${escapeXml(msg.content)}</content>\n`
  if (msg.reasoningContent) {
    xml += `      <reasoning_content>${escapeXml(msg.reasoningContent)}</reasoning_content>\n`
  }
  if (msg.error) {
    xml += `      <error>${escapeXml(msg.error)}</error>\n`
  }

  if (msg.attachments && msg.attachments.length > 0) {
    xml += '      <attachments>\n'
    for (const attach of msg.attachments) {
      xml += '        <attachment>\n'
      xml += `          <filename>${escapeXml(attach.filename)}</filename>\n`
      xml += `          <type>${escapeXml(attach.type)}</type>\n`
      if (attach.contentB64) {
        xml += `          <content_b64>${attach.contentB64}</content_b64>\n`
      }
      if (attach.url) {
        xml += `          <url>${escapeXml(attach.url)}</url>\n`
      }
      xml += '        </attachment>\n'
    }
    xml += '      </attachments>\n'
  }

  if (msg.annotations && msg.annotations.length > 0) {
    xml += '      <annotations>\n'
    for (const anno of msg.annotations) {
      xml += '        <annotation>\n'
      xml += `          <type>${escapeXml(anno.type)}</type>\n`
      // Serialize entire annotation for extensibility
      xml += `          <details>${escapeXml(JSON.stringify(anno))}</details>\n`
      xml += '        </annotation>\n'
    }
    xml += '      </annotations>\n'
  }

  xml += '    </message>\n'
  return xml
}

/**
 * Exports a session and its messages to an XML file.
 */
export async function exportSessionToXml(
  sessionId: number,
  sessionName: string,
) {
  try {
    // 1. Get session config
    const configKey = getSessionConfigKey(sessionId)
    const config = await kvGet<SessionConfig>(configKey)

    // 2. Get session history
    const historyKey = getSessionHistoryKey(sessionId)
    const history = (await kvGet<SessionHistoryItem[]>(historyKey)) || []

    // 3. Fetch all messages
    const messages: ChatMessageData[] = []
    const seenChatIds = new Set<string>()

    for (const item of history) {
      if (seenChatIds.has(item.chatID)) continue
      seenChatIds.add(item.chatID)

      const userData = await kvGet<ChatMessageData>(
        getChatDataKey(item.chatID, 'user'),
      )
      const assistantData = await kvGet<ChatMessageData>(
        getChatDataKey(item.chatID, 'assistant'),
      )

      if (userData && typeof userData === 'object') {
        messages.push(sanitizeChatMessageData(userData))
      }
      if (assistantData && typeof assistantData === 'object') {
        messages.push(sanitizeChatMessageData(assistantData))
      }
    }

    // 4. Construct XML
    let xml = '<?xml version="1.0" encoding="UTF-8"?>\n'
    xml += `<session id="${sessionId}" name="${escapeXml(sessionName)}">\n`

    // Export config section
    if (config) {
      xml += '  <config>\n'
      xml += `    <system_prompt>${escapeXml(config.system_prompt)}</system_prompt>\n`
      xml += `    <selected_model>${escapeXml(config.selected_model)}</selected_model>\n`
      xml += `    <selected_chat_model>${escapeXml(config.selected_chat_model)}</selected_chat_model>\n`
      xml += `    <selected_draw_model>${escapeXml(config.selected_draw_model)}</selected_draw_model>\n`
      xml += `    <temperature>${config.temperature}</temperature>\n`
      xml += `    <max_tokens>${config.max_tokens}</max_tokens>\n`
      xml += `    <n_contexts>${config.n_contexts}</n_contexts>\n`
      xml += '  </config>\n'
    }

    // Export messages section
    xml += '  <messages>\n'
    for (const msg of messages) {
      xml += formatMessageAsXml(msg)
    }
    xml += '  </messages>\n'
    xml += '</session>'

    // 5. Trigger download
    const blob = new Blob([xml], { type: 'application/xml' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    const safeName = sessionName.replace(/[^a-z0-9]/gi, '_').toLowerCase()
    link.href = url
    link.download = `session_${sessionId}_${safeName}_${new Date().getTime()}.xml`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    URL.revokeObjectURL(url)
  } catch (err) {
    console.error('Failed to export session:', err)
    alert('Failed to export session. Please try again.')
  }
}
