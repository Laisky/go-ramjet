import type { ReactNode } from 'react'

import {
  ConfirmDialog,
  type ConfirmDialogProps,
} from '@/components/confirm-dialog'

/**
 * ConfirmActionType represents supported destructive action presets.
 */
export type ConfirmActionType =
  | 'clear-chat-history'
  | 'purge-all-sessions'
  | 'delete-message'
  | 'delete-session'

/**
 * ConfirmActionContext provides extra data used to render action-specific copy.
 */
export interface ConfirmActionContext {
  sessionName?: string
}

/**
 * ConfirmActionProps describes the reusable confirmation wrapper for destructive actions.
 */
export interface ConfirmActionProps {
  action: ConfirmActionType
  onConfirm: ConfirmDialogProps['onConfirm']
  trigger: ReactNode
  context?: ConfirmActionContext
}

interface ConfirmActionCopy {
  title: string
  description: string
  confirmText?: string
  variant: ConfirmDialogProps['variant']
}

/**
 * getConfirmActionCopy returns standardized dialog copy for destructive actions.
 */
export function getConfirmActionCopy(
  action: ConfirmActionType,
  context?: ConfirmActionContext,
): ConfirmActionCopy {
  switch (action) {
    case 'clear-chat-history':
      return {
        title: 'Clear Chat History',
        description:
          'Are you sure you want to clear all chat history for the current session? This action cannot be undone.',
        variant: 'destructive',
      }
    case 'purge-all-sessions':
      return {
        title: 'Purge All Sessions',
        description:
          'Remove every session, config, and chat history while keeping your current API token and base URL. This cannot be undone.',
        variant: 'destructive',
      }
    case 'delete-message':
      return {
        title: 'Delete Message',
        description:
          'Are you sure you want to delete this message pair? This action cannot be undone.',
        confirmText: 'Delete',
        variant: 'destructive',
      }
    case 'delete-session':
      return {
        title: 'Delete Session',
        description: `Are you sure you want to delete "${context?.sessionName || 'this session'}"? This will delete all chat history and settings for this session.`,
        variant: 'destructive',
      }
    default:
      return {
        title: 'Confirm Action',
        description: 'Please confirm this action.',
        variant: 'default',
      }
  }
}

/**
 * ConfirmAction wraps a trigger with a standardized confirmation dialog preset.
 */
export function ConfirmAction({
  action,
  onConfirm,
  trigger,
  context,
}: ConfirmActionProps) {
  const copy = getConfirmActionCopy(action, context)

  return (
    <ConfirmDialog
      title={copy.title}
      description={copy.description}
      confirmText={copy.confirmText}
      variant={copy.variant}
      onConfirm={onConfirm}
      trigger={trigger}
    />
  )
}
