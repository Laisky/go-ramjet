/**
 * Model constants and categories for GPTChat.
 * Ported from legacy chat.js for compatibility.
 */

// Chat models
// export const ChatModelGPT4Turbo = 'gpt-4-turbo'
export const ChatModelGPT41 = 'gpt-4.1'
// export const ChatModelGPT41Mini = 'gpt-4.1-mini'
// export const ChatModelGPT41Nano = 'gpt-4.1-nano'
export const ChatModelGPT6Astra = 'gpt-6-astra'
export const ChatModelGPT5Dot6 = 'gpt-5.6'
export const ChatModelGPT5Dot6Sol = 'gpt-5.6-sol'
export const ChatModelGPT5Dot6Terra = 'gpt-5.6-terra'
export const ChatModelGPT5Dot6Luna = 'gpt-5.6-luna'
// export const ChatModelGPT5Dot1 = 'gpt-5.1'
// export const ChatModelGPT5Dot1Codex = 'gpt-5.1-codex'
export const ChatModelGPT5Dot3Codex = 'gpt-5.3-codex'
// export const ChatModelGPT5Dot4Mini = 'gpt-5.4-mini'
// export const ChatModelGPT5Dot4Nano = 'gpt-5.4-nano'
// export const ChatModelGPT5Pro = 'gpt-5-pro'
export const ChatModelGPT4OMini = 'gpt-4o-mini'
export const ChatModelGPTOSS120B = 'openai/gpt-oss-120b'
export const ChatModelGPTOSS20B = 'openai/gpt-oss-20b'
export const ChatModelGPTO1 = 'o1'
export const ChatModelGPTO3 = 'o3'
export const ChatModelGPTO3Pro = 'o3-pro'
export const ChatModelGPTO3Deepresearch = 'o3-deep-research'
export const ChatModelGPTO3Mini = 'o3-mini'
export const ChatModelGPTO4Mini = 'o4-mini'
export const ChatModelGPTO4MiniDeepresearch = 'o4-mini-deep-research'
export const ChatModelDeepV4Flash = 'deepseek-v4-flash'
export const ChatModelDeepSeekV4Pro = 'deepseek-v4-pro'
// export const ChatModelClaude47Opus = 'claude-opus-4-7'
export const ChatModelClaude48Opus = 'claude-opus-4-8'
export const ChatModelClaudeOpus5 = 'claude-opus-5'
export const ChatModelClaudeFable51 = 'claude-fable-5-1'
export const ChatModelClaudeMythos51 = 'claude-mythos-5-1'
export const ChatModelClaudeSonnet5 = 'claude-sonnet-5'
export const ChatModelClaude45Haiku = 'claude-haiku-4-5'
// export const ChatModelGemini25Pro = 'gemini-2.5-pro'
export const ChatModelGemini3dot1Pro = 'gemini-3.1-pro-preview'
export const ChatModelGemini35FlashLite = 'gemini-3.5-flash-lite'
export const ChatModelGemini38Flash = 'gemini-3.8-flash'
export const ChatModelGemini31FlashImage = 'gemini-3.1-flash-image-preview'
export const ChatModelGemini3ProImage = 'gemini-3-pro-image'
export const ChatModelDeepResearch = 'deep-research'
export const ChatModelLlama33With70B =
  '@cf/meta/llama-3.3-70b-instruct-fp8-fast'
export const ChatModelLlamaPromptGuard2 = 'meta-llama/llama-prompt-guard-2-86m'
export const ChatModelQwen36With27B = 'qwen/qwen3.6-27b'
export const ChatModelKimiK3 = 'kimi-k3'
export const ChatModelGlm5Dot3 = 'glm-5.3'
export const ChatModelGrok4Dot6 = 'grok-4.6'

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
export const ImageModelGptImage2 = 'gpt-image-2'
export const ImageModelGptImageLatest = 'chatgpt-image-latest'
// export const ImageModelSdxlTurbo = 'sdxl-turbo'
export const ImageModelFluxDev = 'black-forest-labs/flux-dev'
export const ImageModelFluxPro2 = 'black-forest-labs/flux-2-pro'
export const ImageModelFluxKontextPro = 'black-forest-labs/flux-kontext-pro'
// export const ImageModelFluxProUltra11 = 'black-forest-labs/flux-1.1-pro-ultra'
export const ImageModelFluxSchnell = 'black-forest-labs/flux-schnell'
export const ImageModelImagen4 = 'imagen-4.0-generate-001'
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
  ChatModelGPT6Astra,
  ChatModelGPT5Dot6,
  ChatModelGPT5Dot6Sol,
  ChatModelGPT5Dot6Terra,
  ChatModelGPT5Dot6Luna,
  // ChatModelGPT5Dot1Codex,
  ChatModelGPT5Dot3Codex,
  // ChatModelGPT5Dot4Mini,
  // ChatModelGPT5Dot4Nano,
  // ChatModelGPT5Pro,
  ChatModelGPT4OMini,
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
  // ChatModelClaude47Opus,
  ChatModelClaude48Opus,
  ChatModelClaudeOpus5,
  ChatModelClaudeFable51,
  ChatModelClaudeMythos51,
  ChatModelClaudeSonnet5,
  ChatModelClaude45Haiku,
  ChatModelLlama33With70B,
  ChatModelLlamaPromptGuard2,
  ChatModelQwen36With27B,
  ChatModelKimiK3,
  ChatModelGlm5Dot3,
  ChatModelGrok4Dot6,
  // ChatModelGemini25Pro,
  ChatModelGemini3dot1Pro,
  ChatModelGemini35FlashLite,
  ChatModelGemini38Flash,
  ChatModelGemini31FlashImage,
  ChatModelGemini3ProImage,
]

export const VisionModels = [
  // ChatModelGPT4Turbo,
  ChatModelGPT41,
  // ChatModelGPT41Mini,
  // ChatModelGPT41Nano,
  // ChatModelGPT5Dot1,
  ChatModelGPT6Astra,
  ChatModelGPT5Dot6,
  ChatModelGPT5Dot6Sol,
  ChatModelGPT5Dot6Terra,
  ChatModelGPT5Dot6Luna,
  // ChatModelGPT5Dot1Codex,
  ChatModelGPT5Dot3Codex,
  // ChatModelGPT5Dot4Mini,
  // ChatModelGPT5Dot4Nano,
  // ChatModelGPT5Pro,
  ChatModelGPT4OMini,
  // ChatModelGemini25Pro,
  ChatModelGemini3dot1Pro,
  ChatModelGemini35FlashLite,
  ChatModelGemini38Flash,
  ChatModelGemini31FlashImage,
  ChatModelGemini3ProImage,
  // ChatModelClaude47Opus,
  ChatModelClaude48Opus,
  ChatModelClaudeOpus5,
  ChatModelClaudeFable51,
  ChatModelClaudeMythos51,
  ChatModelClaudeSonnet5,
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
  ImageModelGptImage2,
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
  ImageModelGptImage2,
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
  ChatModelLlama33With70B,
  ChatModelLlamaPromptGuard2,
  ChatModelQwen36With27B,
  ChatModelGPT4OMini,
  // ChatModelGPT41Nano,
  // ChatModelGPT5Dot4Nano,
  ChatModelGPTOSS120B,
  ChatModelGPTOSS20B,
  ChatModelDeepV4Flash,
  ChatModelGemini35FlashLite,
  ChatModelGemini38Flash,
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
    ChatModelGPTOSS120B,
    ChatModelGPTOSS20B,
    ChatModelGPT41,
    // ChatModelGPT41Mini,
    // ChatModelGPT41Nano,
    // ChatModelGPT5Dot1,
    ChatModelGPT6Astra,
    ChatModelGPT5Dot6,
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
    // ChatModelClaude47Opus,
    ChatModelClaude48Opus,
    ChatModelClaudeOpus5,
    ChatModelClaudeFable51,
    ChatModelClaudeMythos51,
    ChatModelClaudeSonnet5,
  ],
  Google: [
    // ChatModelGemini25Pro,
    ChatModelGemini3dot1Pro,
    ChatModelGemini35FlashLite,
    ChatModelGemini38Flash,
    ChatModelGemini31FlashImage,
    ChatModelGemini3ProImage,
  ],
  Deepseek: [ChatModelDeepV4Flash, ChatModelDeepSeekV4Pro],
  Others: [
    ChatModelDeepResearch,
    ChatModelLlama33With70B,
    ChatModelLlamaPromptGuard2,
    ChatModelQwen36With27B,
    ChatModelKimiK3,
    ChatModelGlm5Dot3,
    ChatModelGrok4Dot6,
  ],
  Image: [
    ImageModelDalle3,
    // ImageModelGptImage1,
    ImageModelGptImage1Mini,
    ImageModelGptImage2,
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
