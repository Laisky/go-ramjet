/**
 * Write-only field for a stored app secret (API key, sync key).
 *
 * The stored value is never placed in an `<input>`. At rest the field shows
 * a masked hint such as `sk-…a1b2`. "Replace" opens an empty SecretInput, and
 * the new value is stored only on an explicit Save. This is the pattern API
 * dashboards use for secrets, and it closes the gaps that masking an input
 * cannot close:
 *   - a password manager has nothing to autofill over, and an autofilled
 *     draft still needs a Save click; extensions such as Bitwarden focus a
 *     field before filling it, so a focus check alone does not stop them;
 *   - screen readers announce a CSS-masked text input's real value, while
 *     the hint reveals nothing until the user asks to show the secret;
 *   - the value is stored in one write, never keystroke by keystroke.
 */
import { Check, Copy, Eye, EyeOff } from 'lucide-react'
import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react'

import { cn } from '@/utils/cn'
import { Button } from './button'
import { SecretInput } from './secret-input'

/**
 * SecretFieldProps configures SecretField.
 *   - inputId and name identify the replacement input; a visible
 *     `<label htmlFor={inputId}>` names it, and labelId lets that label name
 *     the whole field too.
 *   - label is the secret's name in button labels ("Replace API key").
 *   - value is the stored secret; hint is its masked form (defaults to the
 *     last four characters).
 *   - onSave stores a new value; it returns an error message to keep the
 *     editor open, or nothing on success.
 *   - defaultEditing opens the editor on mount, for "re-enter your key" flows.
 *   - className styles the text of the hint and the input.
 */
export interface SecretFieldProps {
  inputId: string
  name: string
  label: string
  labelId?: string
  value: string
  hint?: string
  onSave: (next: string) => string | void | Promise<string | void>
  placeholder?: string
  emptyText?: string
  defaultEditing?: boolean
  className?: string
  iconClassName?: string
  children?: ReactNode
}

/**
 * defaultHint masks value down to its last four characters.
 */
function defaultHint(value: string): string {
  return value.length < 12 ? '••••' : `…${value.slice(-4)}`
}

/**
 * SecretField renders a stored secret as a write-only field: a masked hint
 * with reveal, copy, and replace actions, or an empty editor with Save and
 * Cancel. children render below the field, for extra guidance.
 * It returns the field.
 */
export function SecretField({
  inputId,
  name,
  label,
  labelId,
  value,
  hint,
  onSave,
  placeholder,
  emptyText = 'Not set',
  defaultEditing = false,
  className,
  iconClassName = 'h-4 w-4',
  children,
}: SecretFieldProps) {
  const [editing, setEditing] = useState(defaultEditing)
  const [draft, setDraft] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [revealed, setRevealed] = useState(false)
  const [copied, setCopied] = useState(false)
  const replaceButtonRef = useRef<HTMLButtonElement>(null)
  const restoreFocusRef = useRef(false)
  const errorId = `${inputId}-error`

  useEffect(() => {
    if (editing || !restoreFocusRef.current) return
    restoreFocusRef.current = false
    replaceButtonRef.current?.focus()
  }, [editing])

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(timer)
  }, [copied])

  const closeEditor = () => {
    restoreFocusRef.current = true
    setEditing(false)
    setDraft('')
    setError('')
  }

  const save = async () => {
    const next = draft.trim()
    if (!next || saving) return
    setSaving(true)
    try {
      const failure = await onSave(next)
      if (failure) {
        setError(failure)
        return
      }
      closeEditor()
    } finally {
      setSaving(false)
    }
  }

  const handleInputKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      void save()
    }
  }

  // Escape anywhere in the editor (input, Save, Cancel) cancels the edit. A
  // surrounding dialog keeps itself open by ignoring Escape inside
  // [data-secret-editing] (see isSecretEditorEvent in secret-input-policy).
  const handleGroupKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (editing && e.key === 'Escape') {
      e.preventDefault()
      closeEditor()
    }
  }

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setError('')
    } catch {
      setError(`Copy failed. Show the ${label} and copy it by hand.`)
    }
  }

  return (
    <div
      role="group"
      aria-labelledby={labelId}
      data-secret-editing={editing ? 'true' : undefined}
      onKeyDown={handleGroupKeyDown}
      className="space-y-1"
    >
      {editing ? (
        <div className="flex items-center gap-2">
          <div className="min-w-0 flex-1">
            <SecretInput
              id={inputId}
              name={name}
              label={label}
              value={draft}
              onValueChange={(next) => {
                setDraft(next)
                setError('')
              }}
              onKeyDown={handleInputKeyDown}
              placeholder={placeholder}
              autoFocus
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
              className={className}
              iconClassName={iconClassName}
            />
          </div>
          <Button
            type="button"
            size="sm"
            onClick={() => void save()}
            disabled={!draft.trim() || saving}
          >
            Save
          </Button>
          <Button type="button" size="sm" variant="ghost" onClick={closeEditor}>
            Cancel
          </Button>
        </div>
      ) : (
        <div className="flex min-h-10 items-center gap-1 rounded-md border border-input bg-background py-1 pl-3 pr-1">
          <code
            className={cn(
              'min-w-0 flex-1 break-all font-mono text-sm',
              !value && 'font-sans text-muted-foreground',
              className,
            )}
            data-secret-revealed={revealed ? 'true' : 'false'}
          >
            {value
              ? revealed
                ? value
                : hint || defaultHint(value)
              : emptyText}
          </code>
          {value && (
            <>
              <button
                type="button"
                onClick={() => setRevealed((prev) => !prev)}
                className="rounded p-1.5 text-muted-foreground hover:text-foreground"
                aria-label={revealed ? `Hide ${label}` : `Show ${label}`}
              >
                {revealed ? (
                  <EyeOff className={iconClassName} />
                ) : (
                  <Eye className={iconClassName} />
                )}
              </button>
              <button
                type="button"
                onClick={() => void copy()}
                className="rounded p-1.5 text-muted-foreground hover:text-foreground"
                aria-label={`Copy ${label}`}
              >
                {copied ? (
                  <Check className={iconClassName} />
                ) : (
                  <Copy className={iconClassName} />
                )}
              </button>
            </>
          )}
          <Button
            ref={replaceButtonRef}
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              setEditing(true)
              setRevealed(false)
              setError('')
            }}
            aria-label={`${value ? 'Replace' : 'Set'} ${label}`}
          >
            {value ? 'Replace' : 'Set'}
          </Button>
        </div>
      )}
      <span role="status" className="sr-only">
        {copied ? `${label} copied` : ''}
      </span>
      {error && (
        <p id={errorId} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      {children}
    </div>
  )
}
