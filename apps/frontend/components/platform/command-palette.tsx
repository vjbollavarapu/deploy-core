'use client'

import { Globe, Rocket, Server as ServerIcon } from 'lucide-react'
import { useRouter } from 'next/navigation'
import { useEffect } from 'react'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { applications, deployments, domains, servers } from '@/lib/mock-data'
import { allNavItems } from '@/lib/nav'

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CommandPalette({ open, onOpenChange: setOpen }: CommandPaletteProps) {
  const router = useRouter()

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen(!open)
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [open, setOpen])

  function go(url: string) {
    router.push(url)
    setOpen(false)
  }

  return (
    <CommandDialog open={open} onOpenChange={setOpen} title="Command Palette" description="Search DeployCore">
      <CommandInput placeholder="Search applications, deployments, servers…" />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        <CommandGroup heading="Navigate">
          {allNavItems.map((item) => (
            <CommandItem key={item.url} onClick={() => go(item.url)}>
              <item.icon />
              {item.title}
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Applications">
          {applications.slice(0, 5).map((app) => (
            <CommandItem key={app.id} onClick={() => go(`/applications/${app.id}`)}>
              <Rocket />
              {app.name}
              <span className="ml-auto text-xs text-muted-foreground">{app.project}</span>
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Servers">
          {servers.slice(0, 4).map((server) => (
            <CommandItem key={server.id} onClick={() => go(`/servers/${server.id}`)}>
              <ServerIcon />
              {server.name}
              <span className="ml-auto text-xs text-muted-foreground">{server.region}</span>
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Domains">
          {domains.slice(0, 4).map((domain) => (
            <CommandItem key={domain.id} onClick={() => go(`/applications`)}>
              <Globe />
              {domain.domain}
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Recent Deployments">
          {deployments.slice(0, 4).map((dep) => (
            <CommandItem key={dep.id} onClick={() => go(`/deployments/${dep.id}`)}>
              <ServerIcon />
              {dep.application} #{dep.number}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
