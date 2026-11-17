import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import { Switch } from '@/components/ui/switch'
import {
  ChevronDown,
  ChevronUp,
  Edit2,
  Plus,
  RotateCw,
  Trash2,
} from 'lucide-react'
import { useId, useState } from 'react'
import type { McpServerConfig, McpTool } from '../types'
import { maskSecretHint } from '../utils/api-token'
import { syncMCPServerTools } from '../utils/mcp'

/**
 * McpServerManagerProps configures McpServerManager: servers is the stored
 * list of MCP servers, and onChange receives the whole updated list whenever
 * a server is added, edited, toggled, synced, or removed.
 */
interface McpServerManagerProps {
  servers: McpServerConfig[]
  onChange: (servers: McpServerConfig[]) => void
}

/**
 * McpServerFormData holds the non-secret fields of the add/edit server form.
 * The API key is deliberately not part of it: the stored key never enters
 * form state, and a newly typed key lives in a separate write-only draft.
 */
interface McpServerFormData {
  name: string
  url: string
  enabled: boolean
}

/**
 * EMPTY_FORM is the initial state of the add/edit server form.
 */
const EMPTY_FORM: McpServerFormData = { name: '', url: '', enabled: true }

/**
 * withoutApiKey returns a copy of server without its api_key property, the
 * shape of a server that has no key. It accepts the server to copy.
 */
function withoutApiKey(server: McpServerConfig): McpServerConfig {
  const next = { ...server }
  delete next.api_key
  return next
}

/**
 * applyApiKeyEdit resolves the API key of an edited server from the form's
 * write-only key field. server already carries the stored key; draft is what
 * the user typed; removeKey records an explicit "Remove key". A non-empty
 * draft replaces the key, removeKey drops it, and otherwise the stored key is
 * kept unchanged. It returns the server with the resolved key.
 */
function applyApiKeyEdit(
  server: McpServerConfig,
  draft: string,
  removeKey: boolean,
): McpServerConfig {
  const nextKey = draft.trim()
  if (nextKey) return { ...server, api_key: nextKey }
  if (removeKey) return withoutApiKey(server)
  return server
}

/**
 * describeStoredKey builds the guidance shown under the key field while a
 * server with a stored key is edited. hint is the masked stored key; draft
 * and removeKey are the field's current state. It returns a sentence that
 * never contains the stored key itself.
 */
function describeStoredKey(
  hint: string,
  draft: string,
  removeKey: boolean,
): string {
  if (removeKey) return `Current key ${hint} will be removed when you save.`
  if (draft.trim()) return `Current key ${hint} will be replaced when you save.`
  return `Current key ${hint} — leave empty to keep it.`
}

/**
 * McpServerManager lists the configured MCP servers and lets the user add,
 * edit, enable, sync, and remove them and pick their enabled tools. A stored
 * API key is write-only: editing a server shows only a masked hint of it,
 * an empty key field keeps it, a typed key replaces it, and "Remove key"
 * clears it. servers and onChange are described on McpServerManagerProps.
 * It returns the manager UI.
 */
export function McpServerManager({
  servers = [],
  onChange,
}: McpServerManagerProps) {
  const [isAdding, setIsAdding] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [syncingId, setSyncingId] = useState<string | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const apiKeyHintId = useId()

  // Form state. The API key is kept out of formData: apiKeyDraft only ever
  // holds what the user types, and removeApiKey records an explicit removal.
  const [formData, setFormData] = useState<McpServerFormData>(EMPTY_FORM)
  const [apiKeyDraft, setApiKeyDraft] = useState('')
  const [removeApiKey, setRemoveApiKey] = useState(false)

  const editingServer = editingId
    ? servers.find((s) => s.id === editingId)
    : undefined
  const storedKeyHint = maskSecretHint(editingServer?.api_key)

  const resetForm = () => {
    setFormData(EMPTY_FORM)
    setApiKeyDraft('')
    setRemoveApiKey(false)
    setEditingId(null)
    setIsAdding(false)
  }

  const handleAdd = () => {
    if (!formData.name || !formData.url) return

    const apiKey = apiKeyDraft.trim()
    const newServer: McpServerConfig = {
      id: crypto.randomUUID(),
      name: formData.name,
      url: formData.url,
      ...(apiKey ? { api_key: apiKey } : {}),
      enabled: formData.enabled,
      tools: [],
      enabled_tool_names: [],
    }

    onChange([...servers, newServer])
    resetForm()
  }

  const handleUpdate = () => {
    if (!editingId || !formData.name || !formData.url) return

    const updatedServers = servers.map((s) =>
      s.id === editingId
        ? applyApiKeyEdit(
            { ...s, name: formData.name, url: formData.url },
            apiKeyDraft,
            removeApiKey,
          )
        : s,
    )

    onChange(updatedServers)
    resetForm()
  }

  const startEdit = (server: McpServerConfig) => {
    setFormData({
      name: server.name,
      url: server.url,
      enabled: server.enabled,
    })
    setApiKeyDraft('')
    setRemoveApiKey(false)
    setEditingId(server.id)
    setIsAdding(false)
  }

  const handleApiKeyDraftChange = (next: string) => {
    setApiKeyDraft(next)
    // Typing a new key supersedes a pending removal; clearing the field
    // again falls back to keeping the stored key, the non-destructive choice.
    if (next) setRemoveApiKey(false)
  }

  const handleRemoveApiKey = () => {
    setApiKeyDraft('')
    setRemoveApiKey(true)
  }

  const handleDelete = (id: string) => {
    if (confirm('Are you sure you want to remove this MCP server?')) {
      onChange(servers.filter((s) => s.id !== id))
    }
  }

  const handleToggle = (id: string, enabled: boolean) => {
    onChange(servers.map((s) => (s.id === id ? { ...s, enabled } : s)))
  }

  const handleSync = async (server: McpServerConfig) => {
    setSyncingId(server.id)
    try {
      const { updatedServer, count } = await syncMCPServerTools(server)

      // Update state with new server config (containing tools)
      onChange(servers.map((s) => (s.id === server.id ? updatedServer : s)))

      alert(`Successfully synced ${count} tools from ${server.name}`)
    } catch (error) {
      const err = error instanceof Error ? error : new Error(String(error))
      console.error(err)
      alert(`Failed to sync tools: ${err.message}`)
    } finally {
      setSyncingId(null)
    }
  }

  const handleToggleTool = (
    serverId: string,
    toolName: string,
    checked: boolean,
  ) => {
    onChange(
      servers.map((s) => {
        if (s.id !== serverId) return s

        const currentEnabled = new Set(s.enabled_tool_names || [])
        if (checked) {
          currentEnabled.add(toolName)
        } else {
          currentEnabled.delete(toolName)
        }

        return { ...s, enabled_tool_names: Array.from(currentEnabled) }
      }),
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium">MCP Servers</label>
        <Button
          variant="outline"
          size="sm"
          className="h-8 w-8 p-0"
          onClick={() => {
            resetForm()
            setIsAdding(true)
          }}
          disabled={isAdding || editingId !== null}
          aria-label="Add MCP server"
        >
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      {(isAdding || editingId) && (
        <Card className="p-3 bg-muted border-dashed">
          <div className="space-y-3">
            <h4 className="text-xs font-medium uppercase text-muted-foreground">
              {editingId ? 'Edit Server' : 'Add Server'}
            </h4>
            <div className="space-y-2">
              <Input
                aria-label="MCP server name"
                placeholder="Server Name"
                className="h-8 text-sm"
                value={formData.name}
                onChange={(e) =>
                  setFormData({ ...formData, name: e.target.value })
                }
              />
              <Input
                aria-label="MCP server URL"
                placeholder="Server URL (e.g., https://mcp.laisky.com)"
                className="h-8 text-sm"
                value={formData.url}
                onChange={(e) =>
                  setFormData({ ...formData, url: e.target.value })
                }
              />
              <SecretInput
                name="mcp-server-api-key"
                label="MCP server API key"
                placeholder={
                  storedKeyHint
                    ? 'New API Key (Optional)'
                    : 'API Key (Optional)'
                }
                className="h-8 text-sm"
                iconClassName="h-3.5 w-3.5"
                value={apiKeyDraft}
                onValueChange={handleApiKeyDraftChange}
                aria-describedby={storedKeyHint ? apiKeyHintId : undefined}
              />
              {storedKeyHint && (
                <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                  <span id={apiKeyHintId} aria-live="polite">
                    {describeStoredKey(
                      storedKeyHint,
                      apiKeyDraft,
                      removeApiKey,
                    )}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-6 shrink-0 px-2 text-xs"
                    onClick={
                      removeApiKey
                        ? () => setRemoveApiKey(false)
                        : handleRemoveApiKey
                    }
                  >
                    {removeApiKey ? 'Keep key' : 'Remove key'}
                  </Button>
                </div>
              )}
            </div>
            <div className="flex justify-end gap-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={resetForm}
                className="h-7"
              >
                Cancel
              </Button>
              <Button
                size="sm"
                className="h-7"
                onClick={editingId ? handleUpdate : handleAdd}
                disabled={!formData.name || !formData.url}
              >
                {editingId ? 'Update' : 'Add'}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {servers.length === 0 && !isAdding && (
        <div className="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">
          No MCP servers configured.
        </div>
      )}

      <div className="space-y-2">
        {servers.map((server) => (
          <div
            key={server.id}
            className="group flex flex-col gap-2 rounded-lg border border-border p-3 hover:bg-muted"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Switch
                  checked={server.enabled}
                  onCheckedChange={(checked) =>
                    handleToggle(server.id, checked)
                  }
                  aria-label={`Enable MCP server ${server.name}`}
                />
                <div className="flex flex-col">
                  <span className="font-medium text-sm">{server.name}</span>
                  <span className="text-xs text-muted-foreground truncate max-w-[150px]">
                    {server.url}
                  </span>
                </div>
              </div>
              <div className="flex gap-1 items-center">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-primary"
                  onClick={() => handleSync(server)}
                  disabled={syncingId === server.id}
                  title="Sync Tools"
                >
                  <RotateCw
                    className={`h-3.5 w-3.5 ${syncingId === server.id ? 'animate-spin' : ''}`}
                  />
                </Button>

                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 w-7 p-0"
                  onClick={() =>
                    setExpandedId(expandedId === server.id ? null : server.id)
                  }
                  aria-expanded={expandedId === server.id}
                  aria-label={`${expandedId === server.id ? 'Hide' : 'Show'} tools of MCP server ${server.name}`}
                >
                  {expandedId === server.id ? (
                    <ChevronUp className="h-3.5 w-3.5" />
                  ) : (
                    <ChevronDown className="h-3.5 w-3.5" />
                  )}
                </Button>

                <div className="opacity-0 group-hover:opacity-100 flex gap-1 transition-opacity">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0 text-muted-foreground hover:text-primary"
                    onClick={() => startEdit(server)}
                    aria-label={`Edit MCP server ${server.name}`}
                  >
                    <Edit2 className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => handleDelete(server.id)}
                    aria-label={`Delete MCP server ${server.name}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            </div>

            {/* Tools List */}
            {expandedId === server.id && (
              <div className="ml-10 border-l border-border pl-3 mt-2 space-y-2">
                <div className="text-xs font-semibold text-muted-foreground mb-2">
                  Tools ({server.tools?.length || 0})
                </div>
                {(!server.tools || server.tools.length === 0) && (
                  <div className="text-xs text-muted-foreground italic">
                    No tools synced yet. Click sync button.
                  </div>
                )}
                {server.tools?.map((tool: McpTool) => {
                  const isEnabled = server.enabled_tool_names
                    ? server.enabled_tool_names.includes(tool.name)
                    : true
                  return (
                    <div
                      key={tool.name}
                      className="flex items-start gap-2 text-xs"
                    >
                      <input
                        type="checkbox"
                        checked={isEnabled}
                        onChange={(e) =>
                          handleToggleTool(
                            server.id,
                            tool.name,
                            e.target.checked,
                          )
                        }
                        className="mt-0.5"
                      />
                      <div>
                        <div className="font-medium">{tool.name}</div>
                        <div
                          className="text-muted-foreground line-clamp-1"
                          title={tool.description}
                        >
                          {tool.description}
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        ))}
      </div>

      <p className="text-xs text-muted-foreground">
        Remote MCP servers provide tools/functions for the AI.
      </p>
    </div>
  )
}
