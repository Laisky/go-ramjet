import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import type { McpServerConfig } from '../../types'
import { McpServerManager } from '../mcp-server-manager'

vi.mock('../../utils/mcp', () => ({
  syncMCPServerTools: vi.fn(),
}))

const SECRET = 'sk-mcp-secret-1234567890abcd'
const SECRET_HINT = 'sk-…abcd'

/**
 * makeServer builds an MCP server fixture that stores SECRET as its API key.
 * overrides replaces individual fields. It returns the server config.
 */
function makeServer(overrides: Partial<McpServerConfig> = {}): McpServerConfig {
  return {
    id: 'srv-1',
    name: 'Docs',
    url: 'https://mcp.example.com',
    api_key: SECRET,
    enabled: true,
    tools: [{ name: 'search', description: 'Search the docs' }],
    enabled_tool_names: ['search'],
    ...overrides,
  }
}

/**
 * renderManager renders McpServerManager with servers and a spy onChange.
 * It returns the user-event instance and the onChange spy.
 */
function renderManager(servers: McpServerConfig[]) {
  const user = userEvent.setup()
  const onChange = vi.fn<(next: McpServerConfig[]) => void>()
  render(<McpServerManager servers={servers} onChange={onChange} />)
  return { user, onChange }
}

/**
 * apiKeyInput returns the API key field of the open add/edit form.
 */
function apiKeyInput(): HTMLInputElement {
  return screen.getByRole('textbox', {
    name: 'MCP server API key',
  }) as HTMLInputElement
}

/**
 * expectSecretAbsentFromDom asserts that SECRET appears in no input value,
 * no attribute, and no text of the document.
 */
function expectSecretAbsentFromDom() {
  for (const input of Array.from(document.querySelectorAll('input'))) {
    expect(input.value).not.toContain(SECRET)
  }
  for (const el of Array.from(document.querySelectorAll('*'))) {
    for (const attr of Array.from(el.attributes)) {
      expect(attr.value).not.toContain(SECRET)
    }
  }
  expect(document.body.textContent ?? '').not.toContain(SECRET)
}

/**
 * lastSaved returns the servers passed to the most recent onChange call.
 */
function lastSaved(
  onChange: ReturnType<typeof renderManager>['onChange'],
): McpServerConfig[] {
  expect(onChange).toHaveBeenCalled()
  return onChange.mock.calls[onChange.mock.calls.length - 1][0]
}

describe('McpServerManager stored API key', () => {
  it('never places the stored key in the edit form and shows a masked hint', async () => {
    const { user } = renderManager([makeServer()])

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )

    expect(apiKeyInput()).toHaveValue('')
    expect(screen.getByText(new RegExp(SECRET_HINT))).toBeInTheDocument()
    expect(apiKeyInput()).toHaveAccessibleDescription(
      new RegExp(`${SECRET_HINT}.*leave empty to keep it`),
    )
    expectSecretAbsentFromDom()

    // Revealing the (empty) editor must not reveal the stored key either.
    await user.click(
      screen.getByRole('button', { name: 'Show MCP server API key' }),
    )
    expectSecretAbsentFromDom()
  })

  it('keeps the stored key when the key field is left empty', async () => {
    const server = makeServer()
    const { user, onChange } = renderManager([server])

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )
    await user.type(
      screen.getByRole('textbox', { name: 'MCP server name' }),
      ' v2',
    )
    await user.click(screen.getByRole('button', { name: 'Update' }))

    expect(lastSaved(onChange)).toEqual([
      { ...server, name: 'Docs v2', api_key: SECRET },
    ])
  })

  it('replaces the stored key with a newly typed one', async () => {
    const server = makeServer()
    const { user, onChange } = renderManager([server])

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )
    await user.type(apiKeyInput(), 'sk-new-key-0987654321')
    await user.click(screen.getByRole('button', { name: 'Update' }))

    expect(lastSaved(onChange)).toEqual([
      { ...server, api_key: 'sk-new-key-0987654321' },
    ])
  })

  it('removes the stored key after an explicit Remove key', async () => {
    const server = makeServer()
    const { user, onChange } = renderManager([server])

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )
    await user.click(screen.getByRole('button', { name: 'Remove key' }))
    expect(screen.getByText(/will be removed when you save/)).toBeVisible()
    expectSecretAbsentFromDom()
    await user.click(screen.getByRole('button', { name: 'Update' }))

    const [saved] = lastSaved(onChange)
    expect(saved).not.toHaveProperty('api_key')
    const expected: McpServerConfig = { ...server }
    delete expected.api_key
    expect(saved).toEqual(expected)
  })

  it('keeps the stored key when Remove key is undone', async () => {
    const server = makeServer()
    const { user, onChange } = renderManager([server])

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )
    await user.click(screen.getByRole('button', { name: 'Remove key' }))
    await user.click(screen.getByRole('button', { name: 'Keep key' }))
    await user.click(screen.getByRole('button', { name: 'Update' }))

    expect(lastSaved(onChange)).toEqual([server])
  })

  it('does not offer Remove key for a server without a key', async () => {
    const server = makeServer({ api_key: undefined })
    const { user, onChange } = renderManager([server])

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )
    expect(screen.queryByRole('button', { name: 'Remove key' })).toBeNull()
    expect(apiKeyInput()).toHaveValue('')
    await user.type(apiKeyInput(), 'sk-first-key-1111222233')
    await user.click(screen.getByRole('button', { name: 'Update' }))

    expect(lastSaved(onChange)).toEqual([
      { ...server, api_key: 'sk-first-key-1111222233' },
    ])
  })

  it('discards a typed key on Cancel and reopens with an empty field', async () => {
    const { user, onChange } = renderManager([makeServer()])

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )
    await user.type(apiKeyInput(), 'sk-draft-should-vanish')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onChange).not.toHaveBeenCalled()

    await user.click(
      screen.getByRole('button', { name: 'Edit MCP server Docs' }),
    )
    expect(apiKeyInput()).toHaveValue('')
    expectSecretAbsentFromDom()
  })
})

describe('McpServerManager new server API key', () => {
  it('stores a typed key on a new server', async () => {
    const { user, onChange } = renderManager([])

    await user.click(screen.getByRole('button', { name: 'Add MCP server' }))
    expect(screen.queryByRole('button', { name: 'Remove key' })).toBeNull()
    expect(apiKeyInput()).toHaveValue('')
    await user.type(
      screen.getByRole('textbox', { name: 'MCP server name' }),
      'Search',
    )
    await user.type(
      screen.getByRole('textbox', { name: 'MCP server URL' }),
      'https://mcp.search.example',
    )
    await user.type(apiKeyInput(), 'sk-added-key-5555666677')
    await user.click(screen.getByRole('button', { name: 'Add' }))

    const saved = lastSaved(onChange)
    expect(saved).toHaveLength(1)
    expect(saved[0]).toMatchObject({
      name: 'Search',
      url: 'https://mcp.search.example',
      api_key: 'sk-added-key-5555666677',
      enabled: true,
      tools: [],
      enabled_tool_names: [],
    })
  })

  it('adds a server without a key when the key field is empty', async () => {
    const { user, onChange } = renderManager([])

    await user.click(screen.getByRole('button', { name: 'Add MCP server' }))
    await user.type(
      screen.getByRole('textbox', { name: 'MCP server name' }),
      'Open',
    )
    await user.type(
      screen.getByRole('textbox', { name: 'MCP server URL' }),
      'https://mcp.open.example',
    )
    await user.click(screen.getByRole('button', { name: 'Add' }))

    const [saved] = lastSaved(onChange)
    expect(saved.api_key).toBeFalsy()
  })
})
