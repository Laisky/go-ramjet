export const robotIcon = '🤖️';

// export const ChatModelTurbo35 = 'gpt-3.5-turbo';
// export const ChatModelTurbo35V1106 = 'gpt-3.5-turbo-1106';
// export const ChatModelTurbo35V0125 = 'gpt-3.5-turbo-0125';
// export const ChatModelTurbo35_16K = "gpt-3.5-turbo-16k";
// export const ChatModelTurbo35_0613 = "gpt-3.5-turbo-0613";
// export const ChatModelTurbo35_0613_16K = "gpt-3.5-turbo-16k-0613";
// export const ChatModelGPT4 = "gpt-4";
export const ChatModelGPT4Turbo = 'gpt-4-turbo';
export const ChatModelGPT4O = 'gpt-4o';
export const ChatModelGPT4OMini = 'gpt-4o-mini';
export const ChatModelGPTO1Preview = 'o1-preview';
// export const ChatModelGPTO1 = 'o1';
// export const ChatModelGPTO1Mini = 'o1-mini';
export const ChatModelGPTO3Mini = 'o3-mini';
export const ChatModelDeepSeekChat = 'deepseek-chat';
export const ChatModelDeepSeekResoner = 'deepseek-reasoner';
// export const ChatModelDeepSeekCoder = 'deepseek-coder';
// export const ChatModelGPT4Turbo1106 = 'gpt-4-1106-preview';
// export const ChatModelGPT4Turbo0125 = 'gpt-4-0125-preview';
// export const ChatModelGPT4Vision = 'gpt-4-vision-preview';
// export const ChatModelClaude1 = 'claude-instant-1';
// export const ChatModelClaude2 = 'claude-2';
export const ChatModelClaude3Opus = 'claude-3-opus';
// export const ChatModelClaude3Sonnet = 'claude-3-sonnet';
export const ChatModelClaude35Sonnet = 'claude-3.5-sonnet';
// export const ChatModelClaude35Sonnet8K = 'claude-3.5-sonnet-8k';
// export const ChatModelClaude3Haiku = 'claude-3-haiku';
export const ChatModelClaude35Haiku = 'claude-3.5-haiku';
// export const ChatModelGPT4_0613 = "gpt-4-0613";
// export const ChatModelGPT4_32K = "gpt-4-32k";
// export const ChatModelGPT4_0613_32K = "gpt-4-32k-0613";
// export const ChatModelGeminiPro = 'gemini-pro';
// export const ChatModelGeminiProVision = 'gemini-pro-vision';
export const ChatModelGemini2Flash = 'gemini-2.0-flash';
export const ChatModelGemini2FlashThinking = 'gemini-2.0-flash-thinking';
// export const ChatModelGroqLlama2With70B4K = 'llama2-70b-4096';
// export const ChatModelGroqMixtral8x7B32K = 'mixtral-8x7b-32768';
export const ChatModelGroqGemma2With9B = 'gemma2-9b-it';
export const ChatModelDeepResearch = 'deep-research';
// export const ChatModelGroqllama3With8B = 'llama-3.1-8b-instant';
export const ChatModelGroqllama3With70B = 'llama-3.3-70b-versatile';
// export const ChatModelGroqllama3With405B = 'llama-3.1-405b-instruct';
export const QAModelBasebit = 'qa-bbt-xego';
export const QAModelSecurity = 'qa-security';
export const QAModelImmigrate = 'qa-immigrate';
export const QAModelCustom = 'qa-custom';
export const QAModelShared = 'qa-shared';
export const CompletionModelDavinci3 = 'text-davinci-003';
// export const ImageModelDalle2 = 'dall-e-2';
export const ImageModelDalle3 = 'dall-e-3';
export const ImageModelSdxlTurbo = 'sdxl-turbo';
// export const ImageModelFluxPro = 'flux-pro';
export const ImageModelFluxDev = 'flux-dev';
export const ImageModelFluxPro11 = 'flux-1.1-pro';
export const ImageModelFluxProUltra11 = 'flux-1.1-pro-ultra';
export const ImageModelFluxSchnell = 'flux-schnell';
// export const ImageModelImg2Img = 'img-to-img';

export const DefaultModel = ChatModelGPT4OMini;

// casual chat models

export const ChatModels = [
    ChatModelDeepResearch,
    // ChatModelTurbo35,
    // ChatModelTurbo35V1106,
    // ChatModelTurbo35V0125,
    // ChatModelGPT4,
    ChatModelGPT4Turbo,
    ChatModelGPT4O,
    ChatModelGPT4OMini,
    ChatModelGPTO1Preview,
    // ChatModelGPTO1,
    // ChatModelGPTO1Mini,
    ChatModelGPTO3Mini,
    ChatModelDeepSeekChat,
    ChatModelDeepSeekResoner,
    // ChatModelDeepSeekCoder,
    // ChatModelGPT4Turbo1106,
    // ChatModelGPT4Turbo0125,
    // ChatModelClaude1,
    // ChatModelClaude2,
    ChatModelClaude3Opus,
    ChatModelClaude35Sonnet,
    // ChatModelClaude35Sonnet8K,
    // ChatModelClaude3Haiku,
    ChatModelClaude35Haiku,
    // ChatModelGroqLlama2With70B4K,
    // ChatModelGroqMixtral8x7B32K,
    ChatModelGroqGemma2With9B,
    ChatModelGroqllama3With70B,
    // ChatModelGroqllama3With8B,
    // ChatModelGroqllama3With405B,
    // ChatModelGPT4Vision,
    // ChatModelGeminiPro,
    // ChatModelGeminiProVision,
    ChatModelGemini2Flash,
    ChatModelGemini2FlashThinking
    // ChatModelTurbo35_16K,
    // ChatModelTurbo35_0613,
    // ChatModelTurbo35_0613_16K,
    // ChatModelGPT4_0613,
    // ChatModelGPT4_32K,
    // ChatModelGPT4_0613_32K,
];
export const VisionModels = [
    ChatModelGPT4Turbo,
    ChatModelGPT4O,
    ChatModelGPT4OMini,
    // ChatModelGeminiProVision,
    ChatModelGemini2Flash,
    ChatModelGemini2FlashThinking,
    ChatModelClaude3Opus,
    ChatModelClaude35Sonnet,
    // ChatModelClaude35Sonnet8K,
    // ChatModelClaude3Haiku,
    ChatModelClaude35Haiku,
    ChatModelGPTO1Preview,
    ChatModelGemini2Flash,
    // ImageModelSdxlTurbo,
    // ImageModelImg2Img
    // ImageModelFluxPro,
    ImageModelFluxPro11,
    ImageModelFluxProUltra11,
    ImageModelFluxDev
];
export const QaModels = [
    QAModelBasebit,
    QAModelSecurity,
    QAModelImmigrate,
    QAModelCustom,
    QAModelShared
];
export const ImageModels = [
    ImageModelDalle3,
    ImageModelSdxlTurbo,
    // ImageModelFluxPro,
    ImageModelFluxPro11,
    ImageModelFluxDev,
    ImageModelFluxProUltra11,
    ImageModelFluxSchnell
    // ImageModelImg2Img
];
export const CompletionModels = [
    CompletionModelDavinci3
];
export const FreeModels = [
    // ChatModelGroqLlama2With70B4K,
    // ChatModelGroqMixtral8x7B32K,
    ChatModelGroqGemma2With9B,
    ChatModelGroqllama3With70B,
    // ChatModelGroqllama3With8B,
    // ChatModelGroqllama3With405B,
    // ChatModelTurbo35,
    ChatModelGPT4OMini,
    ChatModelDeepSeekChat,
    // ChatModelDeepSeekCoder,
    // ChatModelTurbo35V0125,
    // ChatModelGeminiPro,
    // ChatModelGeminiProVision,
    ChatModelGemini2Flash,
    QAModelBasebit,
    QAModelSecurity,
    QAModelImmigrate,
    ImageModelSdxlTurbo
    // ImageModelImg2Img
];
export const AllModels = [].concat(
    ChatModels,
    QaModels,
    ImageModels,
    CompletionModels
);

// Kv Keys
export const KvKeyPinnedMaterials = 'config_api_pinned_materials';
export const KvKeyAllowedModels = 'config_chat_models';
export const KvKeyCustomDatasetPassword = 'config_chat_dataset_key';
export const KvKeyPromptShortCuts = 'config_prompt_shortcuts';
export const KvKeyPrefixSessionHistory = 'chat_user_session_';
export const KvKeyPrefixSessionConfig = 'chat_user_config_';
export const KvKeyPrefixSelectedSession = 'config_selected_session';
export const KvKeySyncKey = 'config_sync_key';
export const KvKeyVersionDate = 'config_version_date';
export const KvKeyUserInfo = 'config_user_info';
export const KvKeyChatData = 'chat_data_'; // ${KvKeyChatData}${role}_${chatID}

// Other Constants (Roles, Task Types, etc.) - Keep these here as well
export const RoleHuman = 'user';
export const RoleSystem = 'system';
export const RoleAI = 'assistant';

export const ChatTaskTypeChat = 'chat';
export const ChatTaskTypeImage = 'image';
export const ChatTaskTypeDeepResearch = 'deepresearch';

export const ChatTaskStatusWaiting = 'waiting';
export const ChatTaskStatusProcessing = 'processing';
export const ChatTaskStatusDone = 'done';
