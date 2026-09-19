'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { cn } from '@/lib/utils'
import { DATABASE_SECTIONS } from '@/lib/databases'

interface DatabaseSubnavProps {
  databaseId: string
}

export function DatabaseSubnav({ databaseId }: DatabaseSubnavProps) {
  const pathname = usePathname()
  const base = `/databases/${databaseId}`

  return (
    <nav
      aria-label="Database sections"
      className="-mx-1 flex gap-1 overflow-x-auto border-b border-border pb-px"
    >
      {DATABASE_SECTIONS.map((tab) => {
        const href = `${base}${tab.suffix}`
        const active =
          tab.suffix === ''
            ? pathname === base
            : pathname === href || pathname.startsWith(`${href}/`)
        return (
          <Link
            key={tab.id}
            href={href}
            className={cn(
              'inline-flex shrink-0 items-center border-b-2 px-3 py-2 text-sm transition-colors',
              active
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {tab.label}
          </Link>
        )
      })}
    </nav>
  )
}
