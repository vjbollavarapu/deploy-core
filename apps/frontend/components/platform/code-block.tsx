import { cn } from '@/lib/utils'
import { CopyButton } from './copy-button'

interface CodeBlockProps {
  code: string
  className?: string
  label?: string
}

export function CodeBlock({ code, className, label }: CodeBlockProps) {
  return (
    <div className={cn('group relative overflow-hidden rounded-lg border border-border bg-secondary', className)}>
      {label && (
        <div className="flex items-center justify-between border-b border-border px-3 py-1.5">
          <span className="text-xs font-medium text-muted-foreground">{label}</span>
        </div>
      )}
      <div className="flex items-start justify-between gap-2 p-3">
        <pre className="min-w-0 flex-1 overflow-x-auto font-mono text-xs leading-relaxed text-foreground">
          <code>{code}</code>
        </pre>
        <CopyButton value={code} className="opacity-0 group-hover:opacity-100" />
      </div>
    </div>
  )
}
