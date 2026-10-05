import type { SourceDisplayRow } from '@/lib/applications/application-source-display'

export function ApplicationSourceValue({ row }: { row: SourceDisplayRow }) {
  if (row.href) {
    return (
      <a
        href={row.href}
        title={row.text}
        rel="noreferrer"
        target="_blank"
        className="font-mono text-xs underline-offset-2 hover:underline"
      >
        {row.text}
      </a>
    )
  }
  return (
    <span className="font-mono text-xs" title={row.text}>
      {row.text}
    </span>
  )
}
