'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { cn } from '@/lib/utils'
import { SERVER_SECTIONS } from '@/lib/servers'

interface ServerSubnavProps {
  serverId: string
}

export function ServerSubnav({ serverId }: ServerSubnavProps) {
  const pathname = usePathname()
  const base = `/servers/${serverId}`

  return (
    <nav
      aria-label="Server sections"
      className="-mx-1 flex gap-1 overflow-x-auto border-b border-border pb-px"
    >
      {SERVER_SECTIONS.map((tab) => {
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
