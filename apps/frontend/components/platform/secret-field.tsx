'use client'

import { Eye, EyeOff } from 'lucide-react'
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CopyButton } from './copy-button'

interface SecretFieldProps {
  /** Display-only known value (e.g. non-secret masked mock). Prefer MaskedSecretInput for writes. */
  value: string
  className?: string
  /** When true, reveal still only shows dots — used when value must never be shown. */
  neverReveal?: boolean
}

/**
 * Read-only secret display. For tables that must stay metadata-only, prefer a static mask.
 * Never use this to “edit” an existing server secret (values are not repopulated).
 */
export function SecretField({ value, className, neverReveal = false }: SecretFieldProps) {
  const [revealed, setRevealed] = useState(false)
  const shown = neverReveal || !revealed ? '•'.repeat(Math.min(Math.max(value.length, 12), 24)) : value

  return (
    <div
      className={cn(
        'flex items-center gap-1 rounded-md border border-border bg-secondary px-2.5 py-1.5',
        className,
      )}
    >
      <span className="flex-1 truncate font-mono text-xs text-foreground">{shown}</span>
      {!neverReveal ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-6 text-muted-foreground hover:text-foreground"
          onClick={() => setRevealed((v) => !v)}
        >
          {revealed ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
          <span className="sr-only">Toggle visibility</span>
        </Button>
      ) : null}
      {!neverReveal ? <CopyButton value={value} className="size-6" /> : null}
    </div>
  )
}

interface MaskedSecretInputProps {
  id?: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
  className?: string
  disabled?: boolean
  'aria-invalid'?: boolean
}

/**
 * Write path for secrets. Always starts empty from the caller —
 * never pass an existing secret value from the server.
 */
export function MaskedSecretInput({
  id,
  value,
  onChange,
  placeholder = 'Enter new value',
  className,
  disabled,
  'aria-invalid': ariaInvalid,
}: MaskedSecretInputProps) {
  const [revealed, setRevealed] = useState(false)

  return (
    <div className={cn('relative', className)}>
      <Input
        id={id}
        type={revealed ? 'text' : 'password'}
        autoComplete="new-password"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        aria-invalid={ariaInvalid}
        className="font-mono pr-10"
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute top-1/2 right-1 size-7 -translate-y-1/2 text-muted-foreground"
        onClick={() => setRevealed((v) => !v)}
        disabled={disabled}
      >
        {revealed ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
        <span className="sr-only">Toggle visibility</span>
      </Button>
    </div>
  )
}
