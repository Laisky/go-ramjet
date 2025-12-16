import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  ChevronDown,
  ChevronUp,
  Edit2,
  Plus,
  RotateCw,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import type { McpServerConfig, McpTool } from '../types'
import { syncMCPServerTools } from '../utils/mcp'

interface McpServerManagerProps {
  servers: McpServerConfig[]
  onChange: (servers: McpServerConfig[]) => void
}

export function McpServerManager({
  servers = [],
  onChange,
}: McpServerManagerProps) {
  const [isAdding, setIsAdding] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [syncingId, setSyncingId] = useState<string | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)

  // Form state
  const [formData, setFormData] = useState<Partial<McpServerConfig>>({
    name: '',
    url: '',
    api_key: '',
    enabled: true,
  })

  const resetForm = () => {
    setFormData({ name: '', url: '', api_key: '', enabled: true })
    setEditingId(null)
    setIsAdding(false)
  }

  const handleAdd = () => {
    if (!formData.name || !formData.url) return

    const newServer: McpServerConfig = {
      id: crypto.randomUUID(),
      name: formData.name,
      url: formData.url,
      api_key: formData.api_key,
      enabled: formData.enabled ?? true,
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
        ? {
            ...s,
            name: formData.name!,
            url: formData.url!,
            api_key: formData.api_key,
          }
        : s,
    )

    onChange(updatedServers)
    resetForm()
  }

  const startEdit = (server: McpServerConfig) => {
    setFormData({
      name: server.name,
      url: server.url,
      api_key: server.api_key,
      enabled: server.enabled,
    })
    setEditingId(server.id)
    setIsAdding(false)
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
        >
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      {(isAdding || editingId) && (
        <Card className="p-3 bg-slate-50 dark:bg-slate-900 border-dashed dark:border-slate-700">
          <div className="space-y-3">
            <h4 className="text-xs font-medium uppercase text-slate-500 dark:text-slate-400">
              {editingId ? 'Edit Server' : 'Add Server'}
            </h4>
            <div className="space-y-2">
              <Input
                placeholder="Server Name"
                className="h-8 text-sm"
                value={formData.name}
                onChange={(e) =>
                  setFormData({ ...formData, name: e.target.value })
                }
              />
              <Input
                placeholder="Server URL (e.g., https://mcp.laisky.com)"
                className="h-8 text-sm"
                value={formData.url}
                onChange={(e) =>
                  setFormData({ ...formData, url: e.target.value })
                }
              />
              <Input
                placeholder="API Key (Optional)"
                className="h-8 text-sm"
                type="password"
                value={formData.api_key}
                onChange={(e) =>
                  setFormData({ ...formData, api_key: e.target.value })
                }
              />
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
        <div className="rounded-lg border border-dashed p-4 text-center text-sm text-slate-500 dark:text-slate-400 dark:border-slate-700">
          No MCP servers configured.
        </div>
      )}

      <div className="space-y-2">
        {servers.map((server) => (
          <div
            key={server.id}
            className="group flex flex-col gap-2 rounded-lg border p-3 hover:bg-slate-50 dark:hover:bg-slate-900/50 dark:border-slate-700"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Switch
                  checked={server.enabled}
                  onCheckedChange={(checked) =>
                    handleToggle(server.id, checked)
                  }
                />
                <div className="flex flex-col">
                  <span className="font-medium text-sm">{server.name}</span>
                  <span className="text-xs text-slate-500 dark:text-slate-400 truncate max-w-[150px]">
                    {server.url}
                  </span>
                </div>
              </div>
              <div className="flex gap-1 items-center">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 w-7 p-0 text-slate-500 hover:text-blue-600 dark:text-slate-400 dark:hover:text-blue-400"
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
                    className="h-7 w-7 p-0 text-slate-500 hover:text-blue-600 dark:text-slate-400 dark:hover:text-blue-400"
                    onClick={() => startEdit(server)}
                  >
                    <Edit2 className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0 text-slate-500 hover:text-red-600 dark:text-slate-400 dark:hover:text-red-400"
                    onClick={() => handleDelete(server.id)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            </div>

            {/* Tools List */}
            {expandedId === server.id && (
              <div className="ml-10 border-l pl-3 mt-2 space-y-2 dark:border-slate-700">
                <div className="text-xs font-semibold text-slate-500 dark:text-slate-400 mb-2">
                  Tools ({server.tools?.length || 0})
                </div>
                {(!server.tools || server.tools.length === 0) && (
                  <div className="text-xs text-slate-400 dark:text-slate-500 italic">
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
                          className="text-slate-500 dark:text-slate-400 line-clamp-1"
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

      <p className="text-xs text-slate-500 dark:text-slate-400">
        Remote MCP servers provide tools/functions for the AI.
      </p>
    </div>
  )
}
