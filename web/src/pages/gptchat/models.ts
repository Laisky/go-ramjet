/**
 * Model constants and categories for GPTChat.
 * Ported from legacy chat.js for compatibility.
 */

// Chat models
export const ChatModelGPT4Turbo = 'gpt-4-turbo'
export const ChatModelGPT41 = 'gpt-4.1'
export const ChatModelGPT41Mini = 'gpt-4.1-mini'
export const ChatModelGPT41Nano = 'gpt-4.1-nano'
export const ChatModelGPT5Dot2 = 'gpt-5.2'
export const ChatModelGPT5Dot2Pro = 'gpt-5.2-pro'
export const ChatModelGPT5Dot1 = 'gpt-5.1'
export const ChatModelGPT5Dot1Codex = 'gpt-5.1-codex'
export const ChatModelGPT5Mini = 'gpt-5-mini'
export const ChatModelGPT5Nano = 'gpt-5-nano'
export const ChatModelGPT5Pro = 'gpt-5-pro'
export const ChatModelGPT4O = 'gpt-4o'
export const ChatModelGPT4OSearch = 'gpt-4o-search-preview'
export const ChatModelGPT4OMini = 'gpt-4o-mini'
export const ChatModelGPT4OMiniSearch = 'gpt-4o-mini-search-preview'
export const ChatModelGPTOSS120B = 'openai/gpt-oss-120b'
export const ChatModelGPTOSS20B = 'openai/gpt-oss-20b'
export const ChatModelGPTO1 = 'o1'
export const ChatModelGPTO3 = 'o3'
export const ChatModelGPTO3Pro = 'o3-pro'
export const ChatModelGPTO3Deepresearch = 'o3-deepresearch'
export const ChatModelGPTO3Mini = 'o3-mini'
export const ChatModelGPTO4Mini = 'o4-mini'
export const ChatModelGPTO4MiniDeepresearch = 'o4-mini-deepresearch'
export const ChatModelDeepSeekChat = 'deepseek-chat'
export const ChatModelDeepSeekResoner = 'deepseek-reasoner'
export const ChatModelClaude45Opus = 'claude-opus-4-5'
export const ChatModelClaude45Sonnet = 'claude-sonnet-4-5'
export const ChatModelClaude45Haiku = 'claude-haiku-4-5'
export const ChatModelGemini25Pro = 'gemini-2.5-pro'
export const ChatModelGemini3Pro = 'gemini-3-pro-preview'
export const ChatModelGemini25Flash = 'gemini-2.5-flash'
export const ChatModelGemini25FlashImage = 'gemini-2.5-flash-image-preview'
export const ChatModelDeepResearch = 'deep-research'
export const ChatModelGroqllama3With70B = 'llama-3.3-70b-versatile'
export const ChatModelGroqLlama4 = 'meta-llama/llama-guard-4-12b'
export const ChatModelQwen332B = 'qwen/qwen3-32b'
export const ChatModelKimiK2 = 'moonshotai/kimi-k2-instruct-0905'

// QA models
export const QAModelBasebit = 'qa-bbt-xego'
export const QAModelSecurity = 'qa-security'
export const QAModelImmigrate = 'qa-immigrate'
export const QAModelCustom = 'qa-custom'
export const QAModelShared = 'qa-shared'

// Image models
export const ImageModelDalle3 = 'dall-e-3'
export const ImageModelGptImage1 = 'gpt-image-1'
export const ImageModelSdxlTurbo = 'sdxl-turbo'
export const ImageModelFluxDev = 'black-forest-labs/flux-dev'
export const ImageModelFluxPro11 = 'black-forest-labs/flux-1.1-pro'
export const ImageModelFluxKontextPro = 'black-forest-labs/flux-kontext-pro'
export const ImageModelFluxProUltra11 = 'black-forest-labs/flux-1.1-pro-ultra'
export const ImageModelFluxSchnell = 'black-forest-labs/flux-schnell'
export const ImageModelImagen3 = 'imagen-3.0-generate-002'
export const ImageModelImagen3Fast = 'imagen-3.0-fast-generate-001'

// Completion models
export const CompletionModelDavinci3 = 'text-davinci-003'

// Default model
export const DefaultModel = ChatModelGPT4OMini

// Model collections
export const ChatModels = [
  ChatModelDeepResearch,
  ChatModelGPT41,
  ChatModelGPT41Mini,
  ChatModelGPT41Nano,
  ChatModelGPT5Dot1,
  ChatModelGPT5Dot2,
  ChatModelGPT5Dot2Pro,
  ChatModelGPT5Dot1Codex,
  ChatModelGPT5Mini,
  ChatModelGPT5Nano,
  ChatModelGPT5Pro,
  ChatModelGPT4O,
  ChatModelGPT4OSearch,
  ChatModelGPT4OMini,
  ChatModelGPT4OMiniSearch,
  ChatModelGPT4Turbo,
  ChatModelGPTOSS120B,
  ChatModelGPTOSS20B,
  ChatModelGPTO1,
  ChatModelGPTO3,
  ChatModelGPTO3Pro,
  ChatModelGPTO3Deepresearch,
  ChatModelGPTO3Mini,
  ChatModelGPTO4Mini,
  ChatModelGPTO4MiniDeepresearch,
  ChatModelDeepSeekChat,
  ChatModelDeepSeekResoner,
  ChatModelClaude45Opus,
  ChatModelClaude45Sonnet,
  ChatModelClaude45Haiku,
  ChatModelGroqllama3With70B,
  ChatModelGroqLlama4,
  ChatModelQwen332B,
  ChatModelKimiK2,
  ChatModelGemini25Pro,
  ChatModelGemini3Pro,
  ChatModelGemini25Flash,
  ChatModelGemini25FlashImage,
]

export const VisionModels = [
  ChatModelGPT4Turbo,
  ChatModelGPT41,
  ChatModelGPT41Mini,
  ChatModelGPT41Nano,
  ChatModelGPT5Dot1,
  ChatModelGPT5Dot2,
  ChatModelGPT5Dot2Pro,
  ChatModelGPT5Dot1Codex,
  ChatModelGPT5Mini,
  ChatModelGPT5Nano,
  ChatModelGPT5Pro,
  ChatModelGPT4O,
  ChatModelGPT4OSearch,
  ChatModelGPT4OMini,
  ChatModelGPT4OMiniSearch,
  ChatModelGemini25Pro,
  ChatModelGemini3Pro,
  ChatModelGemini25Flash,
  ChatModelGemini25FlashImage,
  ChatModelClaude45Opus,
  ChatModelClaude45Sonnet,
  ChatModelClaude45Haiku,
  ChatModelGPTO1,
  ChatModelGPTO3,
  ChatModelGPTO3Pro,
  ChatModelGPTO3Deepresearch,
  ImageModelFluxPro11,
  ImageModelFluxKontextPro,
  ImageModelFluxProUltra11,
  ImageModelFluxDev,
  ImageModelGptImage1,
]

export const QaModels = [
  QAModelBasebit,
  QAModelSecurity,
  QAModelImmigrate,
  QAModelCustom,
  QAModelShared,
]

export const ImageModels = [
  ImageModelDalle3,
  ImageModelGptImage1,
  ImageModelSdxlTurbo,
  ImageModelFluxPro11,
  ImageModelFluxKontextPro,
  ImageModelFluxDev,
  ImageModelFluxProUltra11,
  ImageModelFluxSchnell,
  ImageModelImagen3,
  ImageModelImagen3Fast,
]

export const CompletionModels = [CompletionModelDavinci3]

export const FreeModels = [
  ChatModelGroqllama3With70B,
  ChatModelGroqLlama4,
  ChatModelQwen332B,
  ChatModelGPT4OMini,
  ChatModelGPTOSS120B,
  ChatModelGPTOSS20B,
  ChatModelDeepSeekChat,
  ChatModelGemini25Flash,
  QAModelBasebit,
  QAModelSecurity,
  QAModelImmigrate,
  ImageModelSdxlTurbo,
]

export const AllModels = [
  ...ChatModels,
  ...QaModels,
  ...ImageModels,
  ...CompletionModels,
]

// Model categories for UI grouping
export const ModelCategories: Record<string, string[]> = {
  OpenAI: [
    ChatModelGPT4OMini,
    ChatModelGPT4O,
    ChatModelGPT4OMiniSearch,
    ChatModelGPTOSS120B,
    ChatModelGPTOSS20B,
    ChatModelGPT4OSearch,
    ChatModelGPT41,
    ChatModelGPT41Mini,
    ChatModelGPT41Nano,
    ChatModelGPT5Dot1,
    ChatModelGPT5Dot2,
    ChatModelGPT5Dot2Pro,
    ChatModelGPT5Dot1Codex,
    ChatModelGPT5Mini,
    ChatModelGPT5Nano,
    ChatModelGPT5Pro,
    ChatModelGPT4Turbo,
    ChatModelGPTO1,
    ChatModelGPTO3,
    ChatModelGPTO3Mini,
    ChatModelGPTO3Pro,
    ChatModelGPTO3Deepresearch,
    ChatModelGPTO4Mini,
    ChatModelGPTO4MiniDeepresearch,
  ],
  Anthropic: [
    ChatModelClaude45Haiku,
    ChatModelClaude45Opus,
    ChatModelClaude45Sonnet,
  ],
  Google: [
    ChatModelGemini25Pro,
    ChatModelGemini3Pro,
    ChatModelGemini25Flash,
    ChatModelGemini25FlashImage,
  ],
  Deepseek: [ChatModelDeepSeekChat, ChatModelDeepSeekResoner],
  Others: [
    ChatModelDeepResearch,
    ChatModelGroqllama3With70B,
    ChatModelGroqLlama4,
    ChatModelQwen332B,
    ChatModelKimiK2,
  ],
  Image: [
    ImageModelDalle3,
    ImageModelGptImage1,
    ImageModelSdxlTurbo,
    ImageModelFluxDev,
    ImageModelFluxPro11,
    ImageModelFluxKontextPro,
    ImageModelFluxProUltra11,
    ImageModelFluxSchnell,
    ImageModelImagen3,
    ImageModelImagen3Fast,
  ],
}

// Helper functions
export function isChatModel(model: string): boolean {
  return ChatModels.includes(model)
}

export function isQaModel(model: string): boolean {
  return QaModels.includes(model)
}

export function isImageModel(model: string): boolean {
  return ImageModels.includes(model)
}

export function isCompletionModel(model: string): boolean {
  return CompletionModels.includes(model)
}

export function isVisionModel(model: string): boolean {
  return VisionModels.includes(model)
}

export function isFreeModel(model: string): boolean {
  return FreeModels.includes(model)
}

/**
 * Get the category for a model
 */
export function getModelCategory(model: string): string | undefined {
  for (const [category, models] of Object.entries(ModelCategories)) {
    if (models.includes(model)) {
      return category
    }
  }
  return undefined
}
