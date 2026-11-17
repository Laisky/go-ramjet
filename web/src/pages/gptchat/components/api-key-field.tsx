/**
 * API key field of the GPTChat settings sidebar.
 */
import { SecretField } from '@/components/ui/secret-field'
import type { SessionConfig } from '../types'
import { maskSecretHint } from '../utils/api-token'
import { validateCredentialUpdate } from '../utils/credential-policy'

/**
 * API_KEY_INPUT_ID is the DOM id of the API key editor input; the sidebar
 * focuses it when opened from the API key reset alert.
 */
export const API_KEY_INPUT_ID = 'cfg-api-key'

const API_KEY_LABEL_ID = 'cfg-api-key-label'

/**
 * API_KEY_IS_SYNC_KEY_MESSAGE explains why a sync key cannot be saved as
 * the API key.
 */
const API_KEY_IS_SYNC_KEY_MESSAGE =
  'That value is your sync key (or an older one), so it was not saved as the API key. Enter your API key instead; the two keys must be different.'

/**
 * ApiKeyFieldProps configures ApiKeyField. config is the active session
 * config, onConfigChange persists a partial update of it, and
 * defaultEditing opens the editor on mount.
 */
export interface ApiKeyFieldProps {
  config: SessionConfig
  onConfigChange: (updates: Partial<SessionConfig>) => void | Promise<void>
  defaultEditing?: boolean
}

/**
 * ApiKeyField renders the API key as a write-only field. A new key is
 * checked against the credential policy before it is saved, and the field
 * explains a rejection instead of dropping the value silently. Saving a key
 * of the user's own completes a pending API key reset.
 * It returns the labelled field.
 */
export function ApiKeyField({
  config,
  onConfigChange,
  defaultEditing,
}: ApiKeyFieldProps) {
  const handleSave = async (apiToken: string) => {
    const { rejections } = validateCredentialUpdate(config, {
      api_token: apiToken,
    })
    if (rejections.length > 0) return API_KEY_IS_SYNC_KEY_MESSAGE
    await onConfigChange({ api_token: apiToken })
  }

  return (
    <div>
      <label
        id={API_KEY_LABEL_ID}
        htmlFor={API_KEY_INPUT_ID}
        className="mb-1 block text-sm font-medium"
      >
        API Key
      </label>
      <SecretField
        inputId={API_KEY_INPUT_ID}
        name="gptchat-api-key"
        label="API key"
        labelId={API_KEY_LABEL_ID}
        value={config.api_token}
        hint={maskSecretHint(config.api_token)}
        onSave={handleSave}
        placeholder="Paste your API key (sk-...)"
        defaultEditing={defaultEditing}
      />
    </div>
  )
}
