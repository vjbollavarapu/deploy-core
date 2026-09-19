'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { adminNavItems } from '@/lib/nav'
import { cn } from '@/lib/utils'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'

export function AdminNav({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()

  return (
    <PageContainer density="wide">
      <PageHeader
        eyebrow="Super Admin"
        title="Platform administration"
        description="Cross-organization controls. Access is restricted to platform operators."
      />
      <div className="flex flex-col gap-6 lg:flex-row">
        <nav
          aria-label="Admin sections"
          className="flex shrink-0 gap-1 overflow-x-auto lg:w-44 lg:flex-col lg:overflow-visible"
        >
          {adminNavItems.map((item) => {
            const active = pathname === item.url || pathname.startsWith(`${item.url}/`)
            return (
              <Link
                key={item.url}
                href={item.url}
                className={cn(
                  'inline-flex items-center gap-2 rounded-md px-2.5 py-1.5 text-sm whitespace-nowrap transition-colors',
                  active
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                )}
              >
                <item.icon className="size-3.5" />
                {item.title}
              </Link>
            )
          })}
        </nav>
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </PageContainer>
  )
}
