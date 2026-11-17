/**
 * API key field of the GPTChat settings sidebar.
 */
import { SecretField } from '@/components/ui/secret-field'
import type { SessionConfig } from '../types'
import { maskSecretHint } from '../utils/api-token'
import type { ConfigUpdateHandler } from '../utils/credential-policy'

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
 * config, onConfigChange stores a partial update of it and reports the
 * credential fields it refused, and defaultEditing opens the editor on
 * mount.
 */
export interface ApiKeyFieldProps {
  config: SessionConfig
  onConfigChange: ConfigUpdateHandler
  defaultEditing?: boolean
}

/**
 * ApiKeyField renders the API key as a write-only field. The config hook
 * checks a new key against the credential policy while storing it; the
 * field explains a refusal instead of dropping the value silently, and a
 * failed write keeps the editor open (see SecretField). Saving a key of the
 * user's own completes a pending API key reset.
 * It returns the labelled field.
 */
export function ApiKeyField({
  config,
  onConfigChange,
  defaultEditing,
}: ApiKeyFieldProps) {
  const handleSave = async (apiToken: string) => {
    const result = await onConfigChange({ api_token: apiToken })
    if (result?.rejections.some((r) => r.field === 'api_token')) {
      return API_KEY_IS_SYNC_KEY_MESSAGE
    }
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
