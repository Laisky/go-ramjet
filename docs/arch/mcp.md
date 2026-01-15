# Technical Design Document: MCP Remote Server Support

## Menu

- [Technical Design Document: MCP Remote Server Support](#technical-design-document-mcp-remote-server-support)
  - [Menu](#menu)
  - [1. Architecture Overview](#1-architecture-overview)
  - [2. Data Structure](#2-data-structure)
  - [3. Implementation Details](#3-implementation-details)
    - [Part 1: HTML Updates](#part-1-html-updates)
    - [Part 2: JavaScript Logic](#part-2-javascript-logic)
    - [Summary of UX Improvements](#summary-of-ux-improvements)
    - [Important Note on Tool Execution](#important-note-on-tool-execution)

## 1. Architecture Overview

To achieve the "Easy-to-use" priority, we will integrate MCP support directly into the existing Session Configuration. This allows different chat sessions to use different sets of tools.

**Data Flow:**

1.  **Storage:** We will add an `mcp_servers` array to the `SessionConfig` object (stored in IndexedDB via `libs.KvSet`).
2.  **Configuration UI:** A new section in the settings sidebar to Add, Edit, Delete, and Sync MCP servers.
3.  **Fetching Tools:** A function to query the remote MCP URL (assuming a standard endpoint like `/v1/tools` or `/tools/list`) and store the resulting tool definitions.
4.  **Inference:** Modify `sendChat2Server` to inject the cached tool definitions into the OpenAI-compatible API request.

## 2. Data Structure

We will modify `newSessionConfig()` to include:

```javascript
mcp_servers: [
  {
    id: 'uuid...',
    name: 'My Weather Server',
    url: 'https://mcp.example.com',
    api_key: 'sk-...',
    enabled: true,
    tools: [], // Cached tool definitions fetched from remote
  },
];
```

## 3. Implementation Details

I have broken the implementation down into **HTML Updates** (for the UI) and **JavaScript Updates** (for the logic).

### Part 1: HTML Updates

Add the following HTML snippets to your `index.html` (or wherever the `hiddenChatConfigSideBar` and Modals reside).

**1. Add MCP Section to Settings Sidebar**
Find the `div` inside `#hiddenChatConfigSideBar` (Config Sidebar) and add this section (e.g., below the "System Prompt" section):

```html
<!-- MCP Server Management Section -->
<div class="mb-3 mcp-manager">
  <label class="form-label d-flex justify-content-between">
    MCP Servers
    <i class="bi bi-plus-circle add-mcp-server" style="cursor: pointer;" title="Add MCP Server"></i>
  </label>
  <div class="list-group mcp-server-list">
    <!-- Servers will be rendered here by JS -->
  </div>
  <div class="form-text">Remote MCP servers provide tools/functions for the AI.</div>
</div>
```

**2. Add MCP Edit Modal**
Add this new Modal HTML at the bottom of your page (near other modals like `#singleInputModal`):

```html
<!-- Modal: Add/Edit MCP Server -->
<div class="modal fade" id="modal-mcp-edit" tabindex="-1" aria-hidden="true">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Configure MCP Server</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
      </div>
      <div class="modal-body">
        <input type="hidden" class="mcp-id" />
        <div class="mb-3">
          <label class="form-label">Server Name</label>
          <input type="text" class="form-control mcp-name" placeholder="e.g. Weather Tools" />
        </div>
        <div class="mb-3">
          <label class="form-label">MCP URL</label>
          <input type="text" class="form-control mcp-url" placeholder="https://mcp-server.com" />
        </div>
        <div class="mb-3">
          <label class="form-label">API Key (Optional)</label>
          <input type="password" class="form-control mcp-key" placeholder="sk-..." />
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary btn-save-mcp">Save & Sync</button>
      </div>
    </div>
  </div>
</div>
```

### Part 2: JavaScript Logic

Add the following code to your JavaScript file.

**1. Update `newSessionConfig`**
Modify the existing function to initialize the `mcp_servers` array.

```javascript
function newSessionConfig() {
  return {
    // ... existing fields ...
    api_token: 'FREETIER-' + libs.RandomString(32),
    // ... existing fields ...
    chat_switch: {
      all_in_one: false,
      disable_https_crawler: true,
      enable_google_search: false,
      enable_talk: false, // Ensure this exists based on your code
      draw_n_images: 1,
    },
    // NEW: Add this field
    mcp_servers: [],
  };
}
```

**2. MCP Management Logic**
Copy this entire block of functions into your script (e.g., before `setupChatJs`). This handles the UI, fetching tools, and saving config.

```javascript
/**
 * MCP Server Manager Logic
 */
async function setupMCPManager() {
  const container = document.querySelector('.mcp-manager');
  const listContainer = container.querySelector('.mcp-server-list');
  const modalEle = document.getElementById('modal-mcp-edit');
  const modal = new window.bootstrap.Modal(modalEle);

  // Render List from Session Config
  const renderList = async () => {
    const sconfig = await getChatSessionConfig();
    const servers = sconfig.mcp_servers || [];

    listContainer.innerHTML = '';
    if (servers.length === 0) {
      listContainer.innerHTML = '<div class="text-muted small fst-italic">No servers configured.</div>';
      return;
    }

    servers.forEach((server) => {
      const toolCount = server.tools ? server.tools.length : 0;
      const statusColor = server.enabled ? 'text-success' : 'text-secondary';
      const item = document.createElement('div');
      item.className = 'list-group-item d-flex justify-content-between align-items-center';
      item.innerHTML = `
                <div class="d-flex align-items-center overflow-hidden">
                    <div class="form-check form-switch me-2">
                        <input class="form-check-input mcp-enable-switch" type="checkbox" ${
                          server.enabled ? 'checked' : ''
                        } data-id="${server.id}">
                    </div>
                    <div class="text-truncate" style="max-width: 130px;" title="${libs.sanitizeHTML(server.url)}">
                        <strong>${libs.sanitizeHTML(server.name)}</strong>
                        <div class="small text-muted" style="font-size: 0.75rem;">${toolCount} tools</div>
                    </div>
                </div>
                <div class="btn-group btn-group-sm">
                    <button class="btn btn-outline-secondary btn-sync" title="Sync Tools" data-id="${
                      server.id
                    }"><i class="bi bi-arrow-repeat"></i></button>
                    <button class="btn btn-outline-secondary btn-edit" title="Edit" data-id="${
                      server.id
                    }"><i class="bi bi-pencil"></i></button>
                    <button class="btn btn-outline-danger btn-del" title="Delete" data-id="${
                      server.id
                    }"><i class="bi bi-trash"></i></button>
                </div>
            `;
      listContainer.appendChild(item);
    });

    bindListEvents();
  };

  // Bind events for buttons inside the list
  const bindListEvents = () => {
    // Toggle Enable/Disable
    listContainer.querySelectorAll('.mcp-enable-switch').forEach((el) => {
      el.addEventListener('change', async (e) => {
        const id = e.target.dataset.id;
        const sconfig = await getChatSessionConfig();
        const server = sconfig.mcp_servers.find((s) => s.id === id);
        if (server) {
          server.enabled = e.target.checked;
          await saveChatSessionConfig(sconfig);
          // Rerender not strictly needed but good for status color update if added
        }
      });
    });

    // Sync (Fetch Tools)
    listContainer.querySelectorAll('.btn-sync').forEach((el) => {
      el.addEventListener('click', async (e) => {
        const id = e.currentTarget.dataset.id;
        await syncMCPServerTools(id);
        await renderList();
      });
    });

    // Edit
    listContainer.querySelectorAll('.btn-edit').forEach((el) => {
      el.addEventListener('click', async (e) => {
        const id = e.currentTarget.dataset.id;
        const sconfig = await getChatSessionConfig();
        const server = sconfig.mcp_servers.find((s) => s.id === id);
        if (server) {
          openModal(server);
        }
      });
    });

    // Delete
    listContainer.querySelectorAll('.btn-del').forEach((el) => {
      el.addEventListener('click', async (e) => {
        const id = e.currentTarget.dataset.id;
        ConfirmModal('Delete this MCP server?', async () => {
          const sconfig = await getChatSessionConfig();
          sconfig.mcp_servers = sconfig.mcp_servers.filter((s) => s.id !== id);
          await saveChatSessionConfig(sconfig);
          await renderList();
        });
      });
    });
  };

  // Open Modal (Add or Edit)
  const openModal = (server = null) => {
    const body = modalEle.querySelector('.modal-body');
    if (server) {
      body.querySelector('.mcp-id').value = server.id;
      body.querySelector('.mcp-name').value = server.name;
      body.querySelector('.mcp-url').value = server.url;
      body.querySelector('.mcp-key').value = server.api_key || '';
    } else {
      body.querySelector('.mcp-id').value = '';
      body.querySelector('.mcp-name').value = '';
      body.querySelector('.mcp-url').value = '';
      body.querySelector('.mcp-key').value = '';
    }
    modal.show();
  };

  // Save Button Handler
  modalEle.querySelector('.btn-save-mcp').addEventListener('click', async () => {
    const body = modalEle.querySelector('.modal-body');
    const id = body.querySelector('.mcp-id').value;
    const name = body.querySelector('.mcp-name').value.trim();
    const url = body.querySelector('.mcp-url').value.trim();
    const apiKey = body.querySelector('.mcp-key').value.trim();

    if (!name || !url) {
      showalert('warning', 'Name and URL are required.');
      return;
    }

    try {
      ShowSpinner();
      const sconfig = await getChatSessionConfig();

      // Ensure array exists
      if (!sconfig.mcp_servers) sconfig.mcp_servers = [];

      let server;
      if (id) {
        // Update existing
        server = sconfig.mcp_servers.find((s) => s.id === id);
        server.name = name;
        server.url = url;
        server.api_key = apiKey;
      } else {
        // Create new
        server = {
          id: libs.RandomString(8),
          name,
          url,
          api_key: apiKey,
          enabled: true,
          tools: [],
        };
        sconfig.mcp_servers.push(server);
      }

      // Save basic info first
      await saveChatSessionConfig(sconfig);

      // Auto-fetch tools
      modal.hide(); // Hide modal first
      await syncMCPServerTools(server.id); // This will update config again
      await renderList();
    } catch (err) {
      showalert('danger', 'Error saving MCP server: ' + err.message);
    } finally {
      HideSpinner();
    }
  });

  // Add Button Handler
  container.querySelector('.add-mcp-server').addEventListener('click', () => {
    openModal();
  });

  // Initial Render
  await renderList();

  // Listen for session changes to re-render
  libs.KvAddListener(
    KvKeyPrefixSelectedSession,
    async () => {
      await renderList();
    },
    'mcp_manager_session_change'
  );
}

/**
 * Fetch tools from the remote MCP server and update storage
 */
async function syncMCPServerTools(serverId) {
  const sconfig = await getChatSessionConfig();
  const server = sconfig.mcp_servers.find((s) => s.id === serverId);
  if (!server) return;

  try {
    ShowSpinner();
    // Assuming standard MCP HTTP fetch (e.g., GET /tools/list or similar)
    // Adjust endpoint path based on your specific MCP implementation standard
    // Common pattern for OpenAI tools is often just GET /tools
    const headers = {};
    if (server.api_key) {
      headers['Authorization'] = `Bearer ${server.api_key}`;
    }

    // We assume the MCP URL provided points to the base.
    // We append /tools or use the URL directly if user provided full path.
    // Heuristic: if URL doesn't end in /tools, try appending it.
    let fetchUrl = server.url;
    // if (!fetchUrl.endsWith('/tools')) fetchUrl = fetchUrl.replace(/\/+$/, '') + '/tools';

    const resp = await fetch(fetchUrl, {
      method: 'GET',
      headers: headers,
    });

    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

    const data = await resp.json();

    // Normalize data: expect { tools: [...] } or just [...]
    let tools = [];
    if (Array.isArray(data)) {
      tools = data;
    } else if (data.tools && Array.isArray(data.tools)) {
      tools = data.tools;
    } else {
      throw new Error('Invalid tool list format received');
    }

    server.tools = tools;
    await saveChatSessionConfig(sconfig);
    showalert('success', `Successfully fetched ${tools.length} tools for ${server.name}`);
  } catch (err) {
    console.error(err);
    showalert('danger', `Failed to fetch tools from ${server.name}: ${err.message}`);
  } finally {
    HideSpinner();
  }
}
```

**3. Activate MCP Manager**
Modify `setupChatJs` to call `setupMCPManager()`:

```javascript
async function setupChatJs() {
  await setupSessionManager();
  await setupConfig();
  await setupMCPManager(); // <--- ADD THIS LINE
  await setupChatInput();
  // ... rest of function
}
```

**4. Modify `sendChat2Server` to Inject Tools**
Find the `sendChat2Server` function. Inside the `case 'chat':` block, specifically where `reqBody` is constructed, inject the tools.

```javascript
// ... inside sendChat2Server function ...
    switch (promptType) {
    case 'chat':
        messages = await getLastNChatMessages(nContexts, chatID);
        // ... (existing message construction logic) ...

        // START OF MCP MODIFICATION
        // Gather tools from all enabled MCP servers
        let requestTools = [];
        if (sconfig.mcp_servers && Array.isArray(sconfig.mcp_servers)) {
            sconfig.mcp_servers.forEach(server => {
                if (server.enabled && server.tools && server.tools.length > 0) {
                    // Spread tools into the request array
                    requestTools = requestTools.concat(server.tools);
                }
            });
        }
        // END OF MCP MODIFICATION

        // Construct Request Body
        const payload = {
            model: selectedModel,
            stream: true,
            max_tokens: parseInt(sconfig.max_tokens),
            temperature: parseFloat(sconfig.temperature),
            presence_penalty: parseFloat(sconfig.presence_penalty),
            frequency_penalty: parseFloat(sconfig.frequency_penalty),
            messages,
            stop: ['\n\n'],
            laisky_extra: {
                chat_switch: sconfig.chat_switch
            }
        };

        // Inject tools if available
        if (requestTools.length > 0) {
            payload.tools = requestTools;
            // Optionally force auto
            payload.tool_choice = "auto";
        }

        reqBody = JSON.stringify(payload);
        break;
// ... rest of function ...
```

### Summary of UX Improvements

1.  **Unified Config:** The MCP configuration lives right next to the System Prompt and Token settings, respecting the selected Session.
2.  **Auto-Fetch:** When saving a new server, the app automatically attempts to fetch the tool list immediately, giving instant feedback on connection success.
3.  **Toggle Switch:** Users can temporarily disable a specific MCP server using a toggle switch in the UI without deleting the configuration.
4.  **Visual Feedback:** The list shows exactly how many tools are cached for each server, so the user knows what context is being sent to the AI.

### Important Note on Tool Execution

This implementation handles **fetching definitions** and **sending them to the LLM**.
If the LLM decides to call a tool, the response will contain `tool_calls`.

- If your backend (OneAPI/Proxy) handles the actual tool execution (forwarding the call to the MCP URL), this code is complete.
- If the _Client_ (browser) is expected to execute the tool: You would need to add logic in the `globalAIRespSSE.addEventListener('message', ...)` block to detect `tool_calls`, perform a `POST` request to the MCP server to execute the tool, and then send the result back to the LLM in a new message. Based on the request for "fetching lists" and "remote server support" in a web app context, passing definitions is the critical first step.
