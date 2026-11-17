/**
 * API key field of the GPTChat settings sidebar.
 */
import { useState } from 'react'

import { SecretInput } from '@/components/ui/secret-input'
import type { SessionConfig } from '../types'
import {
  canCompleteApiTokenReset,
  isApiTokenClobberedBySyncKey,
} from '../utils/api-token'

/**
 * API_KEY_INPUT_ID is the DOM id of the API key input; the sidebar focuses it
 * when opened from the API key reset alert.
 */
export const API_KEY_INPUT_ID = 'cfg-api-key'

const API_KEY_ERROR_ID = 'cfg-api-key-error'

/**
 * ApiKeyFieldProps configures ApiKeyField: config is the active session
 * config and onConfigChange persists a partial update of it.
 */
export interface ApiKeyFieldProps {
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void
}

/**
 * ApiKeyField renders the labelled API key input. A value that is the sync
 * key is not saved; the field explains why instead of silently dropping it.
 * Leaving the field completes a pending API key reset once the session holds
 * a key of the user's own. It returns the field with its label and feedback.
 */
export function ApiKeyField({ config, onConfigChange }: ApiKeyFieldProps) {
  const [rejectedSyncKey, setRejectedSyncKey] = useState(false)

  const handleValueChange = (apiToken: string) => {
    if (isApiTokenClobberedBySyncKey(apiToken, config.sync_key)) {
      setRejectedSyncKey(true)
      return
    }
    setRejectedSyncKey(false)
    onConfigChange({ api_token: apiToken })
  }

  const handleBlur = () => {
    if (canCompleteApiTokenReset(config)) {
      onConfigChange({ api_token_reset_required: false })
    }
  }

  return (
    <div>
      <label
        htmlFor={API_KEY_INPUT_ID}
        className="mb-1 block text-sm font-medium"
      >
        API Key
      </label>
      <SecretInput
        id={API_KEY_INPUT_ID}
        name="gptchat-api-key"
        label="API key"
        value={config.api_token}
        onValueChange={handleValueChange}
        onBlur={handleBlur}
        aria-invalid={rejectedSyncKey || undefined}
        aria-describedby={rejectedSyncKey ? API_KEY_ERROR_ID : undefined}
        placeholder="sk-..."
      />
      {rejectedSyncKey && (
        <p
          id={API_KEY_ERROR_ID}
          role="alert"
          className="mt-1 text-xs text-destructive"
        >
          That value is your sync key, so it was not saved as the API key. Enter
          your API key instead; the two keys must be different.
        </p>
      )}
    </div>
  )
}
