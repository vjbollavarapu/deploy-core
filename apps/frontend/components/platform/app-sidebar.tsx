'use client'

import { LogOut, Settings, User } from 'lucide-react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@/components/ui/sidebar'
import { adminNavEntry, navGroups } from '@/lib/nav'
import { OrganizationSwitcher } from './organization-switcher'

export function AppSidebar() {
  const pathname = usePathname()
  const { state } = useSidebar()
  const collapsed = state === 'collapsed'

  const isActive = (url: string) => {
    if (url === '/dashboard') return pathname === '/dashboard' || pathname === '/'
    return pathname === url || pathname.startsWith(`${url}/`)
  }

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link href="/dashboard" />}>
              <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
                <svg viewBox="0 0 24 24" fill="none" className="size-4" aria-hidden>
                  <path
                    d="M4 12L12 4L20 12L12 20L4 12Z"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinejoin="round"
                  />
                </svg>
              </div>
              <div className="flex flex-col gap-0 leading-none">
                <span className="font-semibold">DeployCore</span>
                <span className="text-xs text-muted-foreground">Control Plane</span>
              </div>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <div className="px-2 pb-1 md:hidden">
          <OrganizationSwitcher collapsed={collapsed} />
        </div>
      </SidebarHeader>

      <SidebarContent>
        {navGroups.map((group) => (
          <SidebarGroup key={group.title}>
            <SidebarGroupLabel>{group.title}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {group.items.map((item) => (
                  <SidebarMenuItem key={item.url}>
                    <SidebarMenuButton
                      render={<Link href={item.url} />}
                      isActive={isActive(item.url)}
                      tooltip={item.title}
                    >
                      <item.icon />
                      <span>{item.title}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}

        {/* Super Admin is a separately authorised area — visually isolated from org nav. */}
        <SidebarGroup className="mt-auto">
          <SidebarGroupLabel className="text-critical/80">Super Admin</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={<Link href={adminNavEntry.url} />}
                  isActive={pathname.startsWith('/admin')}
                  tooltip={adminNavEntry.title}
                >
                  <adminNavEntry.icon />
                  <span>{adminNavEntry.title}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger render={<SidebarMenuButton size="lg" />}>
                <Avatar className="size-6 rounded-md">
                  <AvatarFallback className="rounded-md text-xs">AO</AvatarFallback>
                </Avatar>
                <div className="flex flex-col gap-0 leading-none">
                  <span className="text-sm font-medium">Amara Osei</span>
                  <span className="text-xs text-muted-foreground">Owner</span>
                </div>
              </DropdownMenuTrigger>
              <DropdownMenuContent className="w-56" align="end">
                <DropdownMenuLabel>amara@daya.io</DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  <DropdownMenuItem render={<Link href="/settings" />}>
                    <User />
                    Profile
                  </DropdownMenuItem>
                  <DropdownMenuItem render={<Link href="/settings" />}>
                    <Settings />
                    Settings
                  </DropdownMenuItem>
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuItem>
                  <LogOut />
                  Log out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
