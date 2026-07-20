/**
 * Model constants and categories for GPTChat.
 * Ported from legacy chat.js for compatibility.
 */

// Chat models
// export const ChatModelGPT4Turbo = 'gpt-4-turbo'
export const ChatModelGPT41 = 'gpt-4.1'
// export const ChatModelGPT41Mini = 'gpt-4.1-mini'
// export const ChatModelGPT41Nano = 'gpt-4.1-nano'
export const ChatModelGPT5Dot5 = 'gpt-5.5'
export const ChatModelGPT5Dot6Sol = 'gpt-5.6-sol'
export const ChatModelGPT5Dot6Terra = 'gpt-5.6-terra'
export const ChatModelGPT5Dot6Luna = 'gpt-5.6-luna'
// export const ChatModelGPT5Dot1 = 'gpt-5.1'
// export const ChatModelGPT5Dot1Codex = 'gpt-5.1-codex'
export const ChatModelGPT5Dot3Codex = 'gpt-5.3-codex'
// export const ChatModelGPT5Dot4Mini = 'gpt-5.4-mini'
// export const ChatModelGPT5Dot4Nano = 'gpt-5.4-nano'
// export const ChatModelGPT5Pro = 'gpt-5-pro'
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
export const ChatModelDeepV4Flash = 'deepseek-v4-flash'
export const ChatModelDeepSeekV4Pro = 'deepseek-v4-pro'
export const ChatModelClaude47Opus = 'claude-opus-4-7'
export const ChatModelClaude48Opus = 'claude-opus-4-8'
export const ChatModelClaudeFable5 = 'claude-fable-5'
export const ChatModelClaudeMythos5 = 'claude-mythos-5'
export const ChatModelClaude46Sonnet = 'claude-sonnet-4-6'
export const ChatModelClaude45Haiku = 'claude-haiku-4-5'
// export const ChatModelGemini25Pro = 'gemini-2.5-pro'
export const ChatModelGemini3dot1Pro = 'gemini-3.1-pro-preview'
export const ChatModelGemini31FlashLite = 'gemini-3.1-flash-lite-preview'
export const ChatModelGemini31FlashImage = 'gemini-3.1-flash-image-preview'
export const ChatModelGemini3ProImage = 'gemini-3-pro-image'
export const ChatModelDeepResearch = 'deep-research'
export const ChatModelGroqllama3With70B = 'llama-3.3-70b-versatile'
export const ChatModelGroqLlama4 = 'meta-llama/llama-guard-4-12b'
export const ChatModelQwen332B = 'qwen/qwen3-32b'
export const ChatModelKimiK3 = 'kimi-k3'
export const ChatModelGlm5Dot2 = 'glm-5.2'
export const ChatModelGrok4Dot5 = 'grok-4.5'

// QA models
export const QAModelBasebit = 'qa-bbt-xego'
export const QAModelSecurity = 'qa-security'
export const QAModelImmigrate = 'qa-immigrate'
export const QAModelCustom = 'qa-custom'
export const QAModelShared = 'qa-shared'

// Image models
export const ImageModelDalle3 = 'dall-e-3'
// export const ImageModelGptImage1 = 'gpt-image-1'
export const ImageModelGptImage1Mini = 'gpt-image-1-mini'
// export const ImageModelGptImage1dot5 = 'gpt-image-1.5'
export const ImageModelGptImageLatest = 'chatgpt-image-latest'
// export const ImageModelSdxlTurbo = 'sdxl-turbo'
export const ImageModelFluxDev = 'black-forest-labs/flux-dev'
export const ImageModelFluxPro2 = 'black-forest-labs/flux-2-pro'
export const ImageModelFluxKontextPro = 'black-forest-labs/flux-kontext-pro'
// export const ImageModelFluxProUltra11 = 'black-forest-labs/flux-1.1-pro-ultra'
export const ImageModelFluxSchnell = 'black-forest-labs/flux-schnell'
export const ImageModelImagen4 = 'imagen-4.0-fast-generate-001'
export const ImageModelImagen4Fast = 'imagen-4.0-fast-generate-001'

// Completion models
export const CompletionModelDavinci3 = 'text-davinci-003'

// Default model
export const DefaultModel = ChatModelGPT4OMini

// Model collections
export const ChatModels = [
  ChatModelDeepResearch,
  ChatModelGPT41,
  // ChatModelGPT41Mini,
  // ChatModelGPT41Nano,
  // ChatModelGPT5Dot1,
  ChatModelGPT5Dot5,
  ChatModelGPT5Dot5,
  ChatModelGPT5Dot6Sol,
  ChatModelGPT5Dot6Terra,
  ChatModelGPT5Dot6Luna,
  // ChatModelGPT5Dot1Codex,
  ChatModelGPT5Dot3Codex,
  // ChatModelGPT5Dot4Mini,
  // ChatModelGPT5Dot4Nano,
  // ChatModelGPT5Pro,
  ChatModelGPT4OSearch,
  ChatModelGPT4OMini,
  ChatModelGPT4OMiniSearch,
  // ChatModelGPT4Turbo,
  ChatModelGPTOSS120B,
  ChatModelGPTOSS20B,
  ChatModelGPTO1,
  ChatModelGPTO3,
  ChatModelGPTO3Pro,
  ChatModelGPTO3Deepresearch,
  ChatModelGPTO3Mini,
  ChatModelGPTO4Mini,
  ChatModelGPTO4MiniDeepresearch,
  ChatModelDeepV4Flash,
  ChatModelDeepSeekV4Pro,
  ChatModelClaude47Opus,
  ChatModelClaude48Opus,
  ChatModelClaudeFable5,
  ChatModelClaudeMythos5,
  ChatModelClaude46Sonnet,
  ChatModelClaude45Haiku,
  ChatModelGroqllama3With70B,
  ChatModelGroqLlama4,
  ChatModelQwen332B,
  ChatModelKimiK3,
  ChatModelGlm5Dot2,
  ChatModelGrok4Dot5,
  // ChatModelGemini25Pro,
  ChatModelGemini3dot1Pro,
  ChatModelGemini31FlashLite,
  ChatModelGemini31FlashImage,
  ChatModelGemini3ProImage,
]

export const VisionModels = [
  // ChatModelGPT4Turbo,
  ChatModelGPT41,
  // ChatModelGPT41Mini,
  // ChatModelGPT41Nano,
  // ChatModelGPT5Dot1,
  ChatModelGPT5Dot5,
  ChatModelGPT5Dot6Sol,
  ChatModelGPT5Dot6Terra,
  ChatModelGPT5Dot6Luna,
  // ChatModelGPT5Dot1Codex,
  ChatModelGPT5Dot3Codex,
  // ChatModelGPT5Dot4Mini,
  // ChatModelGPT5Dot4Nano,
  // ChatModelGPT5Pro,
  ChatModelGPT4OSearch,
  ChatModelGPT4OMini,
  ChatModelGPT4OMiniSearch,
  // ChatModelGemini25Pro,
  ChatModelGemini3dot1Pro,
  ChatModelGemini31FlashLite,
  ChatModelGemini31FlashImage,
  ChatModelGemini3ProImage,
  ChatModelClaude47Opus,
  ChatModelClaude48Opus,
  ChatModelClaude48Opus,
  ChatModelClaudeFable5,
  ChatModelClaudeMythos5,
  ChatModelClaude46Sonnet,
  ChatModelClaude45Haiku,
  ChatModelGPTO1,
  ChatModelGPTO3,
  ChatModelGPTO3Pro,
  ChatModelGPTO3Deepresearch,
  ImageModelFluxPro2,
  ImageModelFluxKontextPro,
  // ImageModelFluxProUltra11,
  ImageModelFluxDev,
  // ImageModelGptImage1,
  ImageModelGptImage1Mini,
  // ImageModelGptImage1dot5,
  ImageModelGptImageLatest,
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
  // ImageModelGptImage1,
  ImageModelGptImage1Mini,
  // ImageModelGptImage1dot5,
  ImageModelGptImageLatest,
  // ImageModelSdxlTurbo,
  ImageModelFluxPro2,
  ImageModelFluxKontextPro,
  ImageModelFluxDev,
  // ImageModelFluxProUltra11,
  ImageModelFluxSchnell,
  ImageModelImagen4,
  ImageModelImagen4Fast,
]

export const CompletionModels = [CompletionModelDavinci3]

export const FreeModels = [
  ChatModelGroqllama3With70B,
  ChatModelGroqLlama4,
  ChatModelQwen332B,
  ChatModelGPT4OMini,
  // ChatModelGPT41Nano,
  // ChatModelGPT5Dot4Nano,
  ChatModelGPTOSS120B,
  ChatModelGPTOSS20B,
  ChatModelDeepV4Flash,
  ChatModelGemini31FlashLite,
  QAModelBasebit,
  QAModelSecurity,
  QAModelImmigrate,
  // ImageModelSdxlTurbo,
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
    ChatModelGPT4OMiniSearch,
    ChatModelGPTOSS120B,
    ChatModelGPTOSS20B,
    ChatModelGPT4OSearch,
    ChatModelGPT41,
    // ChatModelGPT41Mini,
    // ChatModelGPT41Nano,
    // ChatModelGPT5Dot1,
    ChatModelGPT5Dot5,
    ChatModelGPT5Dot6Sol,
    ChatModelGPT5Dot6Terra,
    ChatModelGPT5Dot6Luna,
    // ChatModelGPT5Dot1Codex,
    ChatModelGPT5Dot3Codex,
    // ChatModelGPT5Dot4Mini,
    // ChatModelGPT5Dot4Nano,
    // ChatModelGPT5Pro,
    // ChatModelGPT4Turbo,
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
    ChatModelClaude47Opus,
    ChatModelClaude48Opus,
    ChatModelClaudeFable5,
    ChatModelClaudeMythos5,
    ChatModelClaude46Sonnet,
  ],
  Google: [
    // ChatModelGemini25Pro,
    ChatModelGemini3dot1Pro,
    ChatModelGemini31FlashLite,
    ChatModelGemini31FlashImage,
    ChatModelGemini3ProImage,
  ],
  Deepseek: [ChatModelDeepV4Flash, ChatModelDeepSeekV4Pro],
  Others: [
    ChatModelDeepResearch,
    ChatModelGroqllama3With70B,
    ChatModelGroqLlama4,
    ChatModelQwen332B,
    ChatModelKimiK3,
    ChatModelGlm5Dot2,
    ChatModelGrok4Dot5,
  ],
  Image: [
    ImageModelDalle3,
    // ImageModelGptImage1,
    ImageModelGptImage1Mini,
    // ImageModelGptImage1dot5,
    ImageModelGptImageLatest,
    // ImageModelSdxlTurbo,
    ImageModelFluxDev,
    ImageModelFluxPro2,
    ImageModelFluxKontextPro,
    // ImageModelFluxProUltra11,
    ImageModelFluxSchnell,
    ImageModelImagen4,
    ImageModelImagen4Fast,
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
 * isModelAllowed reports whether the current user may select and use the model.
 */
export function isModelAllowed(
  model: string,
  allowedModels?: string[],
): boolean {
  if (!allowedModels || allowedModels.length === 0) {
    return true
  }

  if (allowedModels.includes('*')) {
    return true
  }

  return allowedModels.includes(model)
}

/**
 * getFirstAllowedModel returns the first permitted model from an ordered list.
 */
export function getFirstAllowedModel(
  models: string[],
  allowedModels?: string[],
): string | undefined {
  return models.find((model) => isModelAllowed(model, allowedModels))
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
