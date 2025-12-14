# GPTChat Feature Inventory & Parity Check

## 1. Core Chat Functionality

| Feature | Legacy (`chat.js`) | SPA (`web/src`) | Parity | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Model Selection** | Yes (Huge list) | Yes (`models.ts`) | ✅ | `models.ts` is a direct port. |
| **Streaming Responses** | Yes (SSE) | Yes (`api.ts`) | ✅ | `sendStreamingChatRequest` implements SSE. |
| **Context Management** | Yes (`n_contexts`) | Yes (`use-config.ts`) | ✅ | Configurable in Sidebar. |
| **System Prompt** | Yes | Yes (`use-config.ts`) | ✅ | Configurable in Sidebar. |
| **History Persistence** | Yes (PouchDB) | Yes (`storage.ts`) | ✅ | `storage.ts` ports `libs.js` logic. |
| **Markdown Rendering** | Yes (`marked.js`) | Yes (`react-markdown`) | ✅ | Supports Mermaid & KaTeX. |

## 2. Advanced Features

| Feature | Legacy (`chat.js`) | SPA (`web/src`) | Parity | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Tool Events (MCP)** | Yes (Parsing logic) | Yes (`chat-parser.ts`) | ✅ | Ported and verified. |
| **Reasoning Display** | Yes (Split logic) | Yes (`chat-parser.ts`) | ✅ | Displayed in `ReasoningBlock`. |
| **Image Generation** | Yes (`ChatTaskTypeImage`) | ? | ⚠️ | Models exist, need to verify triggering logic. |
| **File Upload / Vision** | Yes (Upload logic) | ? | ⚠️ | Checking `ChatInput` for upload button. |
| **Prompt Shortcuts** | Yes (`chat-prompts.js`) | Partial | ❌ | Default prompts missing. Only user-saved ones supported. |

## 3. Settings & Configuration

| Feature | Legacy (`chat.js`) | SPA (`web/src`) | Parity | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **API Token** | Yes | Yes | ✅ | In Sidebar. |
| **Parameters** | Yes (Temp, MaxTokens) | Yes | ✅ | In Sidebar. |
| **Theme** | Browser default | `next-themes` | ✅ | Improved with System/Light/Dark toggle. |

## 4. Payment

| Feature | Legacy (`payment.js`) | SPA (`payment.tsx`) | Parity | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Create Intent** | Yes | Yes | ✅ | Backend call exists. |
| **Stripe Elements** | Yes (Credit Card Form) | No | ❌ | SPA only shows clientSecret, no UI. |

## 5. UI/UX

| Feature | Legacy (`chat.js`) | SPA (`web/src`) | Parity | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Confirm Dialogs** | `window.confirm` | `ConfirmDialog` | ✅ | Modernized with Radix UI. |
| **Copy/Delete Msg** | Yes | Yes | ✅ | In `ChatMessage`. |
| **Stop Generation** | Yes | Yes | ✅ | In `ChatInput`. |
| **Auto-scroll** | Yes | Yes | ✅ | In `GPTChatPage`. |

## Action Items for Parity

1.  **Migrate Default Prompts**: convert `chat-prompts.js` to a TypeScript constant and load it if storage is empty.
2.  **Implement Stripe UI**: Add `@stripe/react-stripe-js` and implement the payment form in `payment.tsx`.
3.  **Verify Image/Vision**: Confirm logic for sending image generation requests and handling file uploads.
