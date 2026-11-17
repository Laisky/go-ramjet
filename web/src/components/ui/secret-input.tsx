/**
 * Masked input for non-login secrets such as API keys and sync keys.
 */
import { Eye, EyeOff } from 'lucide-react'
import { useState, type ChangeEvent } from 'react'

import { cn } from '@/utils/cn'
import { Input, type InputProps } from './input'
import {
  isUserEditing,
  SECRET_INPUT_MASK_CLASS,
  SECRET_INPUT_OPT_OUT_ATTRIBUTES,
} from './secret-input-policy'

/**
 * SecretInputProps configures SecretInput. value and onValueChange make it a
 * controlled field; label names the secret for the reveal toggle's
 * accessible name; toggleClassName and iconClassName size the reveal toggle.
 */
export interface SecretInputProps extends Omit<
  InputProps,
  'type' | 'value' | 'onChange' | 'autoComplete'
> {
  value: string
  onValueChange: (value: string) => void
  label: string
  toggleClassName?: string
  iconClassName?: string
}

/**
 * SecretInput renders a masked, controlled input with a reveal toggle for app
 * secrets. It is a CSS-masked text field (never `type="password"`) so password
 * managers neither autofill a saved site password into it nor offer to save
 * its value, and it ignores value changes that arrive while the field is not
 * focused, so an autofill cannot silently overwrite the stored secret.
 * It returns the field wrapped together with its reveal toggle.
 */
export function SecretInput({
  value,
  onValueChange,
  label,
  className,
  toggleClassName,
  iconClassName = 'h-4 w-4',
  spellCheck = false,
  ...props
}: SecretInputProps) {
  const [revealed, setRevealed] = useState(false)

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    if (!isUserEditing(e.currentTarget)) {
      // React restores the controlled value after this handler returns, so
      // the unsolicited fill never reaches state or storage.
      console.warn(`[SecretInput] ignored unfocused autofill of ${label}`)
      return
    }
    onValueChange(e.currentTarget.value)
  }

  return (
    <div className="relative">
      <Input
        {...props}
        {...SECRET_INPUT_OPT_OUT_ATTRIBUTES}
        type="text"
        value={value}
        onChange={handleChange}
        spellCheck={spellCheck}
        autoCapitalize="off"
        autoCorrect="off"
        data-secret-masked={revealed ? 'false' : 'true'}
        className={cn('pr-10', !revealed && SECRET_INPUT_MASK_CLASS, className)}
      />
      <button
        type="button"
        onClick={() => setRevealed((prev) => !prev)}
        className={cn(
          'absolute right-0 top-0 flex h-full items-center px-3 text-muted-foreground hover:text-foreground',
          toggleClassName,
        )}
        aria-label={revealed ? `Hide ${label}` : `Show ${label}`}
      >
        {revealed ? (
          <EyeOff className={iconClassName} />
        ) : (
          <Eye className={iconClassName} />
        )}
      </button>
    </div>
  )
}
