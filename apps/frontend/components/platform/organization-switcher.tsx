'use client'

import { Check, ChevronsUpDown } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { organizations } from '@/lib/mock-data'
import { cn } from '@/lib/utils'

interface OrganizationSwitcherProps {
  className?: string
  collapsed?: boolean
}

export function OrganizationSwitcher({ className, collapsed = false }: OrganizationSwitcherProps) {
  const [activeId, setActiveId] = useState(organizations[0]?.id ?? '')
  const active = organizations.find((org) => org.id === activeId) ?? organizations[0]

  if (!active) return null

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            size={collapsed ? 'icon' : 'sm'}
            className={cn(
              collapsed ? 'size-8' : 'h-8 w-full justify-between gap-2 px-2 font-normal',
              className,
            )}
            aria-label={`Organization: ${active.name}`}
          />
        }
      >
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded bg-primary/15 text-[10px] font-semibold text-primary',
            collapsed && 'size-4 text-[9px]',
          )}
        >
          {active.name.slice(0, 2).toUpperCase()}
        </span>
        {!collapsed && (
          <>
            <span className="truncate text-left text-xs">{active.name}</span>
            <ChevronsUpDown className="ml-auto size-3.5 shrink-0 text-muted-foreground" />
          </>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-56" align="start">
        <DropdownMenuLabel>Organizations</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {organizations.map((org) => (
          <DropdownMenuItem key={org.id} onClick={() => setActiveId(org.id)}>
            <span className="flex size-5 items-center justify-center rounded bg-muted text-[10px] font-semibold">
              {org.name.slice(0, 2).toUpperCase()}
            </span>
            <span className="flex-1 truncate">{org.name}</span>
            {org.id === active.id && <Check className="size-3.5 text-primary" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
