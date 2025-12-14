# ChatGPT Migration

Migrate `internal/tasks/gptchat/templates/js/chat.js` to new SPA implementation located at `./web`

This analysis breaks down the provided vanilla JavaScript codebase into a structured functional checklist for migration to a React stack (React, React-Router, Radix-UI, Tailwind CSS).

The legacy code relies heavily on direct DOM manipulation, global variables, and `localStorage` wrappers. The migration strategy focuses on moving state to React Context/Zustand, side effects to Hooks, and UI to Radix/Tailwind components while strictly preserving the data schema to ensure no data loss.

### Phase 1: Data Migration & Storage Layer (Critical)
**Objective:** Ensure existing user data (sessions, chat history, configs) stored in `localStorage`/IndexedDB is accessible and uncorrupted in the new React app.

*   [ ] **Storage Adapter Implementation**
    *   Replicate the `window.libs.KvGet/Set` logic. If `window.libs` is an external dependency, wrap it in a React Hook or Service.
    *   **Reference:** Usage throughout (e.g., Lines 678, 1789).
*   [ ] **Data Migration Script (`dataMigrate`)**
    *   Port the logic that moves config from `localStorage` keys to Session KV storage.
    *   Ensure overrides from URL parameters are applied during initialization.
    *   **Reference:** Lines 759-866 (`dataMigrate`), Lines 871-889 (`removeOrphanChatData`).
*   [ ] **Orphan Data Cleanup**
    *   Implement the cleanup logic to remove chat data chunks that no longer belong to valid sessions.
    *   **Reference:** Lines 895-947 (`removeOrphanChatData`).

### Phase 2: App Initialization & Routing
**Objective:** Replicate the entry point logic using React Router and Global Context.

*   [ ] **Theme Management**
    *   Port `setupDarkMode` to a Context Provider or Hook.
    *   **Reference:** Lines 885-889.
*   [ ] **URL Parameter Config Override**
    *   Use `useSearchParams` to parse URL params (e.g., `api_key`, `model`, `system_prompt`) and update the session config on load.
    *   **Reference:** Lines 524-648 (`applyUrlConfigOverrides`, `UrlParamAliasMap`).
*   [ ] **Version Check**
    *   Port `checkUpgrade` to a `useEffect` hook. Use a Radix UI Toast or Dialog for the update prompt.
    *   **Reference:** Lines 1163-1188.

### Phase 3: Session Management (Sidebar)
**Objective:** Manage multiple chat sessions.

*   [ ] **Session List Component**
    *   Fetch and display sessions from `KvKeyPrefixSessionHistory`.
    *   Implement "Active Session" logic.
    *   **Reference:** Lines 1509-1598 (`setupSessionManager`).
*   [ ] **CRUD Operations**
    *   **Create:** New Session button (Line 1563).
    *   **Read:** Load session history (Lines 1419-1428).
    *   **Update:** Rename session (Line 1673 `bindSessionEditBtn`).
    *   **Delete:** Delete session (Line 1698 `bindSessionDeleteBtn`).
    *   **Duplicate:** Duplicate session (Line 1735 `bindSessionDuplicateBtn`).
*   [ ] **Clear/Purge Data**
    *   Implement `clearSessionHandler` to wipe history or all data.
    *   **Reference:** Lines 1621-1669.

### Phase 4: Chat Interface & Message Rendering
**Objective:** The core chat view. Replace raw HTML string building with React Components.

*   [ ] **Message List Component**
    *   Render list of messages (User/AI/System).
    *   Implement "Load More" (Virtualization or Pagination) logic.
    *   **Reference:** Lines 1450-1490 (`loadMoreHistory`, `renderChatBatch`).
*   [ ] **Markdown & HTML Rendering**
    *   Replace `libs.Markdown2HTML` and direct `innerHTML` injection with `react-markdown` or similar safe renderer.
    *   **Critical:** Support custom `<thinking>` blocks and tool outputs.
    *   **Reference:** Lines 2187-2197 (`renderHTML`).
*   [ ] **Reasoning/Thinking UI (Radix Collapsible)**
    *   Parse and render "Chain of Thought" data. Hide `thinking` content behind a collapsible toggle.
    *   **Reference:** Lines 30-40 (`splitReasoningStage`), Lines 2259-2276 (Rendering logic).
*   [ ] **Tool/MCP Event Rendering**
    *   Render tool calls (e.g., "exec MCP tool", "tool error") distinct from chat content.
    *   **Reference:** Lines 103-117 (`renderToolEventsHTML`).
*   [ ] **Code Block Enhancements**
    *   Syntax Highlighting (PrismJS/Shiki).
    *   Mermaid.js diagram support.
    *   MathJax/KaTeX support.
    *   **Reference:** Lines 2288-2316.
*   [ ] **Citations & Annotations**
    *   Parse `url_citation` from AI response and render footnotes/reference list.
    *   **Reference:** Lines 2623-2679 (`parseAnnotationsAsRef`).
*   [ ] **Message Operations**
    *   Copy (Text/Raw), Text-to-Speech (TTS), Reload/Regenerate.
    *   **Reference:** Lines 2359-2465 (`addOperateBtnBelowAiResponse`).

### Phase 5: Input Area & Attachments
**Objective:** Replicate the rich input capabilities.

*   [ ] **Text Input Component**
    *   Auto-resizing textarea (Lines 2750-2790).
    *   Prompt History navigation (Arrow Up/Down).
    *   **Reference:** Lines 330-369 (`navigateChatPromptHistory`).
*   [ ] **Input Bindings**
    *   Ctrl+Enter to send.
    *   Paste handler for images (Clipboard API).
    *   Drag & drop file upload.
    *   **Reference:** Lines 1342-1378 (`bindChatInputDragDrop`), Lines 2955-3004 (`filePasteHandler`).
*   [ ] **Audio Input (STT)**
    *   Implement "Hold to Record" or Toggle Record using `MediaRecorder`.
    *   Send audio to Whisper API (`/oneapi/v1/audio/transcriptions`).
    *   **Reference:** Lines 2877-2950 (`bindTalkBtnHandler`).
*   [ ] **Image Editing/Inpainting (Canvas)**
    *   Replicate the Canvas modal for drawing masks on images.
    *   **Reference:** Lines 971-1053 (`showImageEditModal`).

### Phase 6: API Interaction & Streaming Logic
**Objective:** Handle the complex SSE (Server-Sent Events) and Protocol logic.

*   [ ] **Core Chat API Hook**
    *   Replicate `sendChat2Server` logic.
    *   Handle `fetch` for non-streaming and `SSE` for streaming.
    *   **Reference:** Lines 1794-2200.
*   [ ] **Context Management**
    *   Implement `getLastNChatMessages` to gather context, including images and system prompts.
    *   **Reference:** Lines 1794-1845.
*   [ ] **Stream Parsing**
    *   Port `parseChatResp` to handle various provider formats (OpenAI, Anthropic, Gemini).
    *   Handle "Reasoning" chunks vs "Content" chunks vs "Tool" chunks.
    *   **Reference:** Lines 1851-1896.
*   [ ] **MCP (Model Context Protocol) Client**
    *   Implement the client-side tool execution loop.
    *   Detect tool calls -> Execute client-side fetch -> Send results back to LLM.
    *   **Reference:** Lines 2056-2115 (Tool Loop), Lines 3862-3982 (`callMCPTool`).

### Phase 7: Configuration & Settings (Radix UI)
**Objective:** Migrate the heavy configuration sidebar to a clean Settings Dialog.

*   [ ] **Global/Session Settings**
    *   Inputs for API Token, Base URL, Temperature, Context Limit, etc.
    *   **Reference:** Lines 4252-4416 (`setupConfig`).
*   [ ] **Model Selector (Header)**
    *   Dropdown for selecting models. Group by vendor (OpenAI, Anthropic, etc.).
    *   **Reference:** Lines 1243-1279.
*   [ ] **Prompt Library (Prompt Shortcuts)**
    *   UI to Save/Load/Edit system prompts.
    *   **Reference:** Lines 4528-4663 (`setupPromptManager`).
*   [ ] **MCP Server Manager**
    *   UI to Add/Edit/Sync MCP servers.
    *   **Reference:** Lines 4070-4246 (`setupMCPManager`).
*   [ ] **User Config Sync**
    *   Implement Upload/Download config to server.
    *   **Reference:** Lines 4457-4519.

### Phase 8: Advanced Features (Specialized Tasks)
**Objective:** Migration of non-standard chat modes.

*   [ ] **Image Generation (Text-to-Image)**
    *   Handle Flux, DALL-E, etc.
    *   **Reference:** Lines 1957-2009 (Switch case for models).
*   [ ] **Deep Research Mode**
    *   Handle the specific task type `ChatTaskTypeDeepResearch`.
    *   Poll for results (`fetchDeepResearchResultBackground`).
    *   **Reference:** Lines 1604-1621.
*   [ ] **RAG / Private Dataset (PDF Chat)**
    *   File upload and "Build Bot" flow.
    *   **Reference:** Lines 4700-4965 (`setupPrivateDataset`).

### Phase 9: UI/UX Modernization (Tailwind)
**Objective:** Replace Bootstrap classes.

*   [ ] **Layout:** Replace Bootstrap Grid (`row`, `col`) with Tailwind Flexbox/Grid.
*   [ ] **Components:**
    *   `modal` -> Radix UI `Dialog`.
    *   `dropdown-menu` -> Radix UI `DropdownMenu`.
    *   `tooltip` -> Radix UI `Tooltip`.
    *   `collapse` -> Radix UI `Collapsible`.
    *   `toast`/`alert` -> Radix UI `Toast` or `Sonner`.

### Phase 10: Validation Checklist
**Objective:** Ensure feature parity.

1.  [ ] **Data Persistence:** Does reloading the page retain the current session and chat history?
2.  [ ] **Streaming:** Do messages stream in character-by-character?
3.  [ ] **Stop Generation:** Does the stop button successfully kill the SSE connection?
4.  [ ] **Vision:** Can I upload an image and ask a question about it?
5.  [ ] **Audio:** Can I record voice and get a text response?
6.  [ ] **Editing:** Can I edit a previous user message and regenerate the response tree?
7.  [ ] **Reasoning:** Do "o1/r1" style models show a collapsible "Thinking" block?
8.  [ ] **MCP:** Can the client connect to a configured MCP server and execute a tool?
