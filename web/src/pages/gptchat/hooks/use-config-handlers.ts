/**
 * Config handlers of the GPTChat page: the callbacks it hands to the
 * settings sidebar, the chat input, and the API key reset alert, and the
 * effect that keeps the selected models within the user's allowed models.
 */
import { useCallback, useEffect } from 'react'

import {
  ChatModels,
  getFirstAllowedModel,
  ImageModelFluxDev,
  ImageModels,
  isImageModel,
  isModelAllowed,
} from '../models'
import { DefaultSessionConfig, type SessionConfig } from '../types'
import type { ConfigUpdateHandler } from '../utils/credential-policy'
import { useSyncKeyRotation } from './use-sync-key-rotation'

/**
 * UseConfigHandlersOptions configures useConfigHandlers: config is the
 * active session config, updateConfig stores an update (useConfig),
 * exportAllData and importAllData move all local data to and from a cloud
 * backup, and allowedModels lists the models the user may select ('*' or
 * none means all).
 */
export interface UseConfigHandlersOptions {
  config: SessionConfig
  updateConfig: ConfigUpdateHandler
  exportAllData: () => Promise<unknown>
  importAllData: (
    data: Record<string, unknown>,
    mode?: 'merge' | 'download',
  ) => Promise<void>
  allowedModels?: string[]
}

/**
 * pickAllowedModelUpdates returns the model updates that bring config
 * within allowedModels, or an empty object when nothing has to change.
 */
function pickAllowedModelUpdates(
  config: SessionConfig,
  allowedModels: string[],
): Partial<SessionConfig> {
  const chatModel = config.selected_chat_model || config.selected_model
  const drawModel = config.selected_draw_model || ImageModelFluxDev
  const nextChatModel = isModelAllowed(chatModel, allowedModels)
    ? chatModel
    : getFirstAllowedModel(ChatModels, allowedModels)
  const nextDrawModel = isModelAllowed(drawModel, allowedModels)
    ? drawModel
    : getFirstAllowedModel(ImageModels, allowedModels)

  const nextUpdates: Partial<SessionConfig> = {}
  if (nextChatModel && nextChatModel !== config.selected_chat_model) {
    nextUpdates.selected_chat_model = nextChatModel
  }
  if (nextDrawModel && nextDrawModel !== config.selected_draw_model) {
    nextUpdates.selected_draw_model = nextDrawModel
  }
  if (!isModelAllowed(config.selected_model, allowedModels)) {
    const fallbackSelectedModel = isImageModel(config.selected_model)
      ? nextDrawModel || nextChatModel
      : nextChatModel || nextDrawModel
    if (
      fallbackSelectedModel &&
      fallbackSelectedModel !== config.selected_model
    ) {
      nextUpdates.selected_model = fallbackSelectedModel
    }
  }
  return nextUpdates
}

/**
 * useConfigHandlers builds the page's config callbacks. handleConfigChange
 * is updateConfig itself, so the settings sidebar can wait for a write and
 * see it fail; applyConfigInBackground stores an update without waiting and
 * logs a failure, for controls that apply as they change. It also keeps the
 * selected models within allowedModels.
 * It returns the callbacks, plus rotateSyncKey for the API key reset alert.
 */
export function useConfigHandlers({
  config,
  updateConfig,
  exportAllData,
  importAllData,
  allowedModels,
}: UseConfigHandlersOptions) {
  const applyConfigInBackground = useCallback(
    (updates: Partial<SessionConfig>, canApply?: () => boolean) => {
      const run = async () => {
        try {
          if (canApply) await updateConfig(updates, canApply)
          else await updateConfig(updates)
        } catch (error) {
          console.error(`[GPTChat] saving settings failed: ${String(error)}`)
        }
      }
      void run()
    },
    [updateConfig],
  )

  useEffect(() => {
    if (
      !allowedModels ||
      allowedModels.length === 0 ||
      allowedModels.includes('*')
    ) {
      return
    }
    const nextUpdates = pickAllowedModelUpdates(config, allowedModels)
    if (Object.keys(nextUpdates).length > 0) {
      applyConfigInBackground(nextUpdates)
    }
    // Only the model fields matter; the whole config would rerun this on
    // every keystroke of the system prompt.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    allowedModels,
    applyConfigInBackground,
    config.selected_chat_model,
    config.selected_draw_model,
    config.selected_model,
  ])

  const handleChatSwitchChange = useCallback(
    (updates: Partial<SessionConfig['chat_switch']>) => {
      applyConfigInBackground({
        chat_switch: { ...config.chat_switch, ...updates },
      })
    },
    [applyConfigInBackground, config.chat_switch],
  )

  const handleChatModelChange = useCallback(
    (model: string) => {
      applyConfigInBackground({
        selected_model: model,
        selected_chat_model: model,
      })
    },
    [applyConfigInBackground],
  )

  const handleDrawModelChange = useCallback(
    (model: string) => {
      applyConfigInBackground({
        selected_model: model,
        selected_draw_model: model,
      })
    },
    [applyConfigInBackground],
  )

  const handleReset = useCallback(async () => {
    // Reset every setting except the browser-wide sync key.
    const { sync_key: _syncKey, ...rest } = DefaultSessionConfig // eslint-disable-line @typescript-eslint/no-unused-vars
    try {
      await updateConfig(rest)
    } catch (error) {
      console.error(`[GPTChat] resetting settings failed: ${String(error)}`)
    }
  }, [updateConfig])

  const handleImportData = useCallback(
    async (data: unknown, mode: 'merge' | 'download' = 'merge') => {
      // Accept any shape but pass an object map to the storage importer.
      await importAllData((data as Record<string, unknown>) || {}, mode)
    },
    [importAllData],
  )

  const rotateSyncKey = useSyncKeyRotation({
    exportData: exportAllData,
    importData: handleImportData,
    updateConfig,
  })

  return {
    handleConfigChange: updateConfig,
    applyConfigInBackground,
    handleChatSwitchChange,
    handleChatModelChange,
    handleDrawModelChange,
    handleReset,
    handleImportData,
    rotateSyncKey,
  }
}
