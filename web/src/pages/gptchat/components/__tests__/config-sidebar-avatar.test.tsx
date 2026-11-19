import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { DefaultSessionConfig } from '../../types'
import { ConfigSidebar } from '../config-sidebar'

const state = vi.hoisted(() => ({ imageUrl: '' }))

vi.mock('../../hooks/use-user', () => ({
  useUser: () => ({
    user: {
      user_name: 'Avatar Tester',
      image_url: state.imageUrl,
      is_free: true,
      no_limit_expensive_models: false,
    },
    isLoading: false,
    isError: null,
    mutate: vi.fn(),
  }),
}))

vi.mock('../session-manager', () => ({ SessionManager: () => null }))
vi.mock('../model-selector', () => ({ ModelSelector: () => null }))
vi.mock('../prompt-shortcut-manager', () => ({
  PromptShortcutManager: () => null,
}))
vi.mock('../dataset-manager', () => ({ DatasetManager: () => null }))
vi.mock('../mcp-server-manager', () => ({ McpServerManager: () => null }))
vi.mock('../data-sync-manager', () => ({ DataSyncManager: () => null }))

/**
 * renderSidebarWithAvatar renders the open sidebar for a user whose profile
 * reports imageUrl as the avatar.
 *
 * @param imageUrl - The image_url value returned by /user/me.
 * @returns The rendered avatar image, or null when the fallback icon is shown.
 */
function renderSidebarWithAvatar(imageUrl: string): HTMLElement | null {
  state.imageUrl = imageUrl
  render(
    <ConfigSidebar
      isOpen={true}
      onClose={vi.fn()}
      config={DefaultSessionConfig}
      onConfigChange={vi.fn()}
      onClearChats={vi.fn()}
      onReset={vi.fn()}
      onExportData={vi.fn().mockResolvedValue({})}
      onImportData={vi.fn().mockResolvedValue(undefined)}
    />,
  )
  return screen.queryByRole('img', { name: 'Avatar Tester' })
}

// CodeQL alert #100 flags `image_url.includes('openai.com')` as incomplete URL
// sanitization. It is a cosmetic deny-list that swaps OpenAI-hosted avatars for
// the default icon; it never allows anything. Every other URL is already
// rendered as a plain <img>, so a crafted host can at most hide or show an
// avatar image, which grants an attacker nothing.
describe('ConfigSidebar user avatar', () => {
  beforeEach(() => {
    state.imageUrl = ''
  })

  it('renders an avatar from an arbitrary host as a plain image', () => {
    const avatar = renderSidebarWithAvatar('https://avatars.example/u.png')

    expect(avatar).not.toBeNull()
    expect(avatar?.getAttribute('src')).toBe('https://avatars.example/u.png')
  })

  it('shows the fallback icon for OpenAI-hosted avatars', () => {
    expect(
      renderSidebarWithAvatar('https://cdn.openai.com/avatar.png'),
    ).toBeNull()
  })

  it.each([
    'https://openai.com.evil.example/a.png',
    'https://evil.example/a.png?openai.com',
    'https://evilopenai.com/a.png',
  ])('only ever hides the avatar for the crafted URL %s', (url) => {
    expect(renderSidebarWithAvatar(url)).toBeNull()
  })

  it('shows the fallback icon when no avatar is set', () => {
    expect(renderSidebarWithAvatar('')).toBeNull()
  })
})
