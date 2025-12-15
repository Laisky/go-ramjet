import { DefaultModel, ImageModelFluxDev } from './models'
/**
 * Types and interfaces for GPTChat.
 */

export interface ChatMessageData {
  chatID: string
  role: 'user' | 'assistant' | 'system'
  content: string
  model?: string
  reasoningContent?: string
  timestamp?: number
  attachments?: ChatAttachment[]
  annotations?: Annotation[]
  references?: ChatReference[]
}

export interface ChatAttachment {
  filename: string
  contentB64?: string
  cacheKey?: string
  url?: string
  type: 'image' | 'file'
}

export interface Annotation {
  type: string
  url_citation?: {
    url: string
    title?: string
  }
  [key: string]: unknown
}

export interface ChatReference {
  index: number
  url: string
  title?: string
}

export interface SessionConfig {
  api_token: string
  token_type: 'proxy' | 'direct'
  api_base: string
  selected_model: string
  selected_chat_model: string
  selected_draw_model: string
  system_prompt: string
  n_contexts: number
  max_tokens: number
  temperature: number
  presence_penalty: number
  frequency_penalty: number
  chat_switch: ChatSwitch
  mcp_servers?: McpServerConfig[]
  sync_key?: string
  session_name?: string
}

export interface ChatSwitch {
  disable_https_crawler: boolean
  all_in_one: boolean
  enable_talk: boolean
  enable_mcp: boolean
  draw_n_images: number
}

export interface McpTool {
  name: string
  description?: string
  input_schema?: Record<string, unknown>
}

export interface McpServerConfig {
  id: string
  name: string
  url: string
  api_key?: string
  enabled: boolean
  tools?: McpTool[]
  enabled_tool_names?: string[]
}

export interface SessionHistoryItem {
  chatID: string
  role: 'user' | 'assistant'
  content: string
  model?: string
  timestamp?: number
}

export interface PromptShortcut {
  name: string
  prompt: string
}

// Default configuration values
export const DefaultSessionConfig: SessionConfig = {
  api_token: '',
  token_type: 'proxy',
  api_base: 'https://api.openai.com',
  selected_model: DefaultModel,
  selected_chat_model: DefaultModel,
  selected_draw_model: ImageModelFluxDev,
  system_prompt:
    "The following is a conversation with Chat-GPT, an AI created by OpenAI. The AI is helpful, creative, clever, and very friendly, it's mainly focused on solving coding problems, so it likely provide code example whenever it can and every code block is rendered as markdown. However, it also has a sense of humor and can talk about anything. Please answer user's last question, and if possible, reference the context as much as you can.",
  n_contexts: 6,
  max_tokens: 4000,
  temperature: 1,
  presence_penalty: 0,
  frequency_penalty: 0,
  chat_switch: {
    disable_https_crawler: false,
    all_in_one: false,
    enable_talk: false,
    enable_mcp: false,
    draw_n_images: 1,
  },
  mcp_servers: [
    {
      id: 'default-mcp',
      name: 'Default MCP',
      url: 'https://mcp.laisky.com',
      enabled: true,
      tools: [],
    },
  ],
  sync_key: '',
  session_name: 'Chat Session 1',
}

// Roles
export const RoleHuman = 'user' as const
export const RoleAI = 'assistant' as const
export const RoleSystem = 'system' as const

// Task types
export const ChatTaskTypeChat = 'chat' as const
export const ChatTaskTypeImage = 'image' as const
export const ChatTaskTypeDeepResearch = 'deepresearch' as const

// Task status
export const ChatTaskStatusWaiting = 'waiting' as const
export const ChatTaskStatusProcessing = 'processing' as const
export const ChatTaskStatusDone = 'done' as const

export interface UserConfig {
  user_name: string
  token: string
  openai_token: string
  image_token: string
  is_free: boolean
  is_admin: boolean
  allowed_models: string[]
  no_limit_expensive_models: boolean
  api_base: string
  image_url: string
}
