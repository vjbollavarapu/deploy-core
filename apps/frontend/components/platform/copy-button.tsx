'use client'

import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

interface CopyButtonProps {
  value: string
  className?: string
  label?: string
}

export function CopyButton({ value, className, label = 'Copy' }: CopyButtonProps) {
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={cn('size-7 text-muted-foreground hover:text-foreground', className)}
            onClick={handleCopy}
          >
            {copied ? <Check className="size-3.5 text-success" /> : <Copy className="size-3.5" />}
            <span className="sr-only">{label}</span>
          </Button>
        }
      />
      <TooltipContent>{copied ? 'Copied' : label}</TooltipContent>
    </Tooltip>
  )
}
