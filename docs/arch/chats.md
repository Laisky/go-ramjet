# ChatGPT Migration

Migrate `internal/tasks/gptchat/templates/js/chat.js` to new SPA implementation located at `./web`

This analysis breaks down the provided vanilla JavaScript codebase into a structured functional checklist for migration to a React stack (React, React-Router, Radix-UI, Tailwind CSS).

The legacy code relies heavily on direct DOM manipulation, global variables, and `localStorage` wrappers. The migration strategy focuses on moving state to React Context/Zustand, side effects to Hooks, and UI to Radix/Tailwind components while strictly preserving the data schema to ensure no data loss.

### Phase 1: Data Migration & Storage Layer (Critical)

**Objective:** Ensure existing user data (sessions, chat history, configs) stored in `localStorage`/IndexedDB is accessible and uncorrupted in the new React app.

- [x] **Storage Adapter Implementation** — migrated via `kvGet/kvSet` wrapper and session-scoped keys in [web/src/pages/gptchat/utils/chat-storage.ts](web/src/pages/gptchat/utils/chat-storage.ts#L1-L120).
- [x] **Data Migration Script (`dataMigrate`)** — migrated in [web/src/pages/gptchat/utils/migration.ts](web/src/pages/gptchat/utils/migration.ts#L1-L200); URL overrides applied during config load in [web/src/pages/gptchat/hooks/use-config.ts](web/src/pages/gptchat/hooks/use-config.ts#L12-L210).
- [x] **Orphan Data Cleanup** — implemented in migration cleanup pass in [web/src/pages/gptchat/utils/migration.ts](web/src/pages/gptchat/utils/migration.ts#L140-L200).

### Phase 2: App Initialization & Routing

**Objective:** Replicate the entry point logic using React Router and Global Context.

- [x] **Theme Management** — handled via Tailwind dark mode toggle and persisted class in SPA shell [web/src/main.tsx](web/src/main.tsx#L1-L120).
- [x] **URL Parameter Config Override** — implemented in config loader [web/src/pages/gptchat/hooks/use-config.ts](web/src/pages/gptchat/hooks/use-config.ts#L56-L170) using alias map logic.
- [x] **Version Check** — implemented via `/version` poll with upgrade banner in [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L60-L110).

### Phase 3: Session Management (Sidebar)

**Objective:** Manage multiple chat sessions.

- [x] **Session List Component** — implemented in ConfigSidebar using `useConfig` session list [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L415-L456).
- [x] **CRUD Operations** — create/switch/rename/duplicate/delete wired in `useConfig` and sidebar actions [web/src/pages/gptchat/hooks/use-config.ts](web/src/pages/gptchat/hooks/use-config.ts#L180-L360) and [web/src/pages/gptchat/components/config-sidebar.tsx](web/src/pages/gptchat/components/config-sidebar.tsx#L1-L260).
- [x] **Clear/Purge Data** — clear current session chats and purge-all implemented via `clearMessages` and `purgeAllSessions` [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L398-L456).

### Phase 4: Chat Interface & Message Rendering

**Objective:** The core chat view. Replace raw HTML string building with React Components.

- [x] **Message List Component** — virtualized pagination via "Load older" and render in [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L350-L400).
- [x] **Markdown & HTML Rendering** — `Markdown` component with GFM, math, raw HTML, mermaid, highlight [web/src/components/markdown.tsx](web/src/components/markdown.tsx#L1-L112); thinking/tool blocks rendered in ChatMessage.
- [x] **Reasoning/Thinking UI (Radix Collapsible)** — reasoning panel and toggle in [web/src/pages/gptchat/components/chat-message.tsx](web/src/pages/gptchat/components/chat-message.tsx#L1-L220).
- [x] **Tool/MCP Event Rendering** — tool call events streamed into reasoning content and shown in message meta [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L300-L520).
- [x] **Code Block Enhancements** — syntax highlight, mermaid, KaTeX handled in `Markdown` [web/src/components/markdown.tsx](web/src/components/markdown.tsx#L1-L112).
- [x] **Citations & Annotations** — annotations parsed to references and rendered in ChatMessage footer [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L500-L640) and [web/src/pages/gptchat/components/chat-message.tsx](web/src/pages/gptchat/components/chat-message.tsx#L230-L360).
- [x] **Message Operations** — copy/regenerate/edit-resend wired in ChatMessage with paired user lookup [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L380-L396) and ChatMessage controls; TTS not yet reimplemented.

### Phase 5: Input Area & Attachments

**Objective:** Replicate the rich input capabilities.

- [x] **Text Input Component** — auto-resize textarea with prompt history nav in [web/src/pages/gptchat/components/chat-input.tsx](web/src/pages/gptchat/components/chat-input.tsx#L20-L220).
- [x] **Input Bindings** — Ctrl+Enter send, paste images, drag/drop implemented in ChatInput [web/src/pages/gptchat/components/chat-input.tsx](web/src/pages/gptchat/components/chat-input.tsx#L220-L340).
- [x] **Audio Input (STT)** — MediaRecorder + Whisper transcription via `transcribeAudio` [web/src/pages/gptchat/components/chat-input.tsx](web/src/pages/gptchat/components/chat-input.tsx#L340-L460).
- [x] **Image Editing/Inpainting (Canvas)** — image editor modal and Flux inpaint integration in ChatInput + useChat [web/src/pages/gptchat/components/image-editor-modal.tsx](web/src/pages/gptchat/components/image-editor-modal.tsx#L1-L240) and [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L260-L340).

### Phase 6: API Interaction & Streaming Logic

**Objective:** Handle the complex SSE (Server-Sent Events) and Protocol logic.

- [x] **Core Chat API Hook** — streaming chat implemented in `useChat` with abort control [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L480-L760).
- [x] **Context Management** — last `n_contexts` messages appended before send [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L500-L540).
- [x] **Stream Parsing** — reasoning/content/tool/annotation handlers wired to UI [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L540-L720).
- [x] **MCP (Model Context Protocol) Client** — tool call loop and server resolution implemented [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L360-L520) and [web/src/pages/gptchat/utils/mcp.ts](web/src/pages/gptchat/utils/mcp.ts#L1-L260).

### Phase 7: Configuration & Settings (Radix UI)

**Objective:** Migrate the heavy configuration sidebar to a clean Settings Dialog.

- [x] **Global/Session Settings** — config sidebar controls API token/base, temps, context, etc. [web/src/pages/gptchat/components/config-sidebar.tsx](web/src/pages/gptchat/components/config-sidebar.tsx#L1-L260).
- [x] **Model Selector (Header)** — model dropdown in header [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L330-L348).
- [x] **Prompt Library (Prompt Shortcuts)** — shortcuts managed via sidebar and stored in KV [web/src/pages/gptchat/components/config-sidebar.tsx](web/src/pages/gptchat/components/config-sidebar.tsx#L120-L220).
- [x] **MCP Server Manager** — MCP servers editable and synced; enabled list passed to chat [web/src/pages/gptchat/components/config-sidebar.tsx](web/src/pages/gptchat/components/config-sidebar.tsx#L200-L240) and [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L470-L520).
- [x] **User Config Sync** — upload/download wired to `/gptchat/user/config` with sync key UI [web/src/pages/gptchat/components/config-sidebar.tsx](web/src/pages/gptchat/components/config-sidebar.tsx#L600-L660).

### Phase 8: Advanced Features (Specialized Tasks)

**Objective:** Migration of non-standard chat modes.

- [x] **Image Generation (Text-to-Image)** — supported via Flux/DALL-E branches in `useChat` and API [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L431-L486) and [web/src/utils/api.ts](web/src/utils/api.ts#L58-L110).
- [x] **Deep Research Mode** — task creation/polling integrated for deepresearch models [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L328-L428) and [web/src/utils/api.ts](web/src/utils/api.ts#L112-L180).
- [x] **RAG / Private Dataset (PDF Chat)** — dataset key storage, upload/list/delete, and chatbot activation integrated [web/src/pages/gptchat/components/config-sidebar.tsx](web/src/pages/gptchat/components/config-sidebar.tsx#L480-L590) with backend endpoints `/ramjet/gptchat/files` and `/ramjet/gptchat/ctx/*`.

### Phase 9: UI/UX Modernization (Tailwind)

**Objective:** Replace Bootstrap classes.

- [x] **Layout:** Migrated to Tailwind flex/grid layout across SPA [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L320-L456).
- [x] **Components:** Modals, toggles, dropdowns implemented with Headless/Tailwind components; toasts pending but critical flows use inline alerts [web/src/pages/gptchat/components/**/*](web/src/pages/gptchat/components/chat-input.tsx#L20-L220).

### Phase 10: Validation Checklist

**Objective:** Ensure feature parity.

1.  [x] **Data Persistence:** Session config/history stored in KV and restored on load [web/src/pages/gptchat/hooks/use-config.ts](web/src/pages/gptchat/hooks/use-config.ts#L12-L170) and [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L90-L150).
2.  [x] **Streaming:** SSE streaming with incremental updates [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L500-L700).
3.  [x] **Stop Generation:** Stop button aborts controller and deep-research polling [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L800-L830).
4.  [x] **Vision:** Image uploads sent as `image_url` parts; inpainting via mask supported [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L199-L340).
5.  [x] **Audio:** Voice recording and Whisper transcription integrated in ChatInput [web/src/pages/gptchat/components/chat-input.tsx](web/src/pages/gptchat/components/chat-input.tsx#L340-L460).
6.  [x] **Editing:** Edit/resend and regenerate wired through ChatMessage and `useChat.regenerateMessage` [web/src/pages/gptchat/index.tsx](web/src/pages/gptchat/index.tsx#L380-L400).
7.  [x] **Reasoning:** Collapsible reasoning/thinking panel with streamed content and tool events [web/src/pages/gptchat/components/chat-message.tsx](web/src/pages/gptchat/components/chat-message.tsx#L1-L220) and [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L300-L520).
8.  [x] **MCP:** MCP tool calls resolved and executed client-side with results fed back to stream [web/src/pages/gptchat/hooks/use-chat.ts](web/src/pages/gptchat/hooks/use-chat.ts#L360-L520) and [web/src/pages/gptchat/utils/mcp.ts](web/src/pages/gptchat/utils/mcp.ts#L1-L260).
