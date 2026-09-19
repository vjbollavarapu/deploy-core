'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { cn } from '@/lib/utils'

const TABS = [
  { label: 'Overview', suffix: '' },
  { label: 'Deployments', suffix: '/deployments' },
  { label: 'Revisions', suffix: '/revisions' },
  { label: 'Logs', suffix: '/logs' },
  { label: 'Metrics', suffix: '/metrics' },
  { label: 'Domains', suffix: '/domains' },
  { label: 'Environment', suffix: '/environment' },
  { label: 'Secrets', suffix: '/secrets' },
  { label: 'Networking', suffix: '/networking' },
  { label: 'Volumes', suffix: '/volumes' },
  { label: 'Settings', suffix: '/settings' },
] as const

interface ApplicationSubnavProps {
  applicationId: string
}

export function ApplicationSubnav({ applicationId }: ApplicationSubnavProps) {
  const pathname = usePathname()
  const base = `/applications/${applicationId}`

  return (
    <nav
      aria-label="Application sections"
      className="-mx-1 flex gap-1 overflow-x-auto border-b border-border pb-px"
    >
      {TABS.map((tab) => {
        const href = `${base}${tab.suffix}`
        const active =
          tab.suffix === ''
            ? pathname === base
            : pathname === href || pathname.startsWith(`${href}/`)
        return (
          <Link
            key={tab.suffix || 'overview'}
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
