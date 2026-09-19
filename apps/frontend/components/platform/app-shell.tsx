'use client'

import type { ReactNode } from 'react'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { Toaster } from '@/components/ui/sonner'
import { AppSidebar } from './app-sidebar'
import { TopBar } from './top-bar'

interface AppShellProps {
  children: ReactNode
}

export function AppShell({ children }: AppShellProps) {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <TopBar />
        <main className="flex-1 overflow-y-auto p-4 sm:p-6">{children}</main>
      </SidebarInset>
      <Toaster />
    </SidebarProvider>
  )
}
