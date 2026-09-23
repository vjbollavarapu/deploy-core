'use client'

import { Moon, Search, Sun } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Kbd } from '@/components/ui/kbd'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { CommandPalette } from './command-palette'
import { OrganizationSwitcher } from './organization-switcher'

export function TopBar() {
  const { theme, setTheme } = useTheme()
  const [showPalette, setShowPalette] = useState(false)

  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-4">
      <SidebarTrigger className="-ml-1" />
      <Separator orientation="vertical" className="hidden h-5 sm:block" />
      <div className="hidden md:block">
        <OrganizationSwitcher className="w-44" />
      </div>
      <Button
        variant="outline"
        className="h-8 max-w-xs flex-1 justify-start text-muted-foreground sm:max-w-sm"
        onClick={() => setShowPalette(true)}
      >
        <Search data-icon="inline-start" />
        <span className="truncate">Search resources…</span>
        <Kbd className="ml-auto hidden sm:inline-flex">⌘K</Kbd>
      </Button>
      <div className="ml-auto flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          className="size-8"
          aria-label="Toggle theme"
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
        >
          <Sun className="hidden dark:block" aria-hidden />
          <Moon className="block dark:hidden" aria-hidden />
        </Button>
      </div>
      <CommandPalette open={showPalette} onOpenChange={setShowPalette} />
    </header>
  )
}
