import { cn } from '@/lib/utils'
import { buildRevisionCompareRows, formatRevisionNumber } from '@/lib/revisions'
import type { Revision } from '@/lib/types'

interface RevisionCompareProps {
  left: Revision
  right: Revision
  className?: string
}

export function RevisionCompare({ left, right, className }: RevisionCompareProps) {
  const rows = buildRevisionCompareRows(left, right)
  const changedCount = rows.filter((r) => r.changed).length

  return (
    <div className={cn('overflow-hidden rounded-lg border border-border', className)}>
      <div className="grid grid-cols-[10rem_1fr_1fr] gap-0 border-b border-border bg-muted/40 px-3 py-2 text-xs font-medium text-muted-foreground">
        <div>Field</div>
        <div>
          Revision {formatRevisionNumber(left.number)}
          <span className="ml-1 font-mono text-[10px]">{left.commit}</span>
        </div>
        <div>
          Revision {formatRevisionNumber(right.number)}
          <span className="ml-1 font-mono text-[10px]">{right.commit}</span>
        </div>
      </div>
      <div className="border-b border-border px-3 py-1.5 text-xs text-muted-foreground">
        {changedCount === 0
          ? 'Revisions are identical across compared fields.'
          : `${changedCount} field${changedCount === 1 ? '' : 's'} differ. Secret values are never shown.`}
      </div>
      <ul className="divide-y divide-border">
        {rows.map((row) => (
          <li
            key={row.label}
            className={cn(
              'grid grid-cols-[10rem_1fr_1fr] gap-3 px-3 py-2.5 text-sm',
              row.changed && 'bg-warning/5',
            )}
          >
            <div className="text-xs font-medium text-muted-foreground">{row.label}</div>
            <CompareValue value={row.left} changed={row.changed} />
            <CompareValue value={row.right} changed={row.changed} />
          </li>
        ))}
      </ul>
    </div>
  )
}

function CompareValue({ value, changed }: { value: string; changed: boolean }) {
  return (
    <pre
      className={cn(
        'whitespace-pre-wrap break-all font-mono text-[11px] leading-relaxed text-foreground',
        changed && 'font-medium',
      )}
    >
      {value}
    </pre>
  )
}
