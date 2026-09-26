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
import { getDemoFixtures } from '@/lib/mock-isolation'
import { allNavItems } from '@/lib/nav'

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CommandPalette({ open, onOpenChange: setOpen }: CommandPaletteProps) {
  const router = useRouter()
  const demoApps = getDemoFixtures(applications)
  const demoServers = getDemoFixtures(servers)
  const demoDomains = getDemoFixtures(domains)
  const demoDeployments = getDemoFixtures(deployments)

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

  function go(href: string) {
    setOpen(false)
    router.push(href)
  }

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput placeholder="Type a command or search…" />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        <CommandGroup heading="Navigation">
          {allNavItems.map((item) => (
            <CommandItem key={item.url} onClick={() => go(item.url)}>
              {item.title}
            </CommandItem>
          ))}
        </CommandGroup>
        {demoApps.length > 0 ? (
          <>
            <CommandSeparator />
            <CommandGroup heading="Applications">
              {demoApps.slice(0, 5).map((app) => (
                <CommandItem key={app.id} onClick={() => go(`/applications/${app.id}`)}>
                  <Rocket />
                  {app.name}
                  <span className="ml-auto text-xs text-muted-foreground">{app.project}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        ) : null}
        {demoServers.length > 0 ? (
          <>
            <CommandSeparator />
            <CommandGroup heading="Servers">
              {demoServers.slice(0, 4).map((server) => (
                <CommandItem key={server.id} onClick={() => go(`/servers/${server.id}`)}>
                  <ServerIcon />
                  {server.name}
                  <span className="ml-auto text-xs text-muted-foreground">{server.region ?? '—'}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        ) : null}
        {demoDomains.length > 0 ? (
          <>
            <CommandSeparator />
            <CommandGroup heading="Domains">
              {demoDomains.slice(0, 4).map((domain) => (
                <CommandItem key={domain.id} onClick={() => go(`/applications`)}>
                  <Globe />
                  {domain.domain}
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        ) : null}
        {demoDeployments.length > 0 ? (
          <>
            <CommandSeparator />
            <CommandGroup heading="Recent Deployments">
              {demoDeployments.slice(0, 4).map((dep) => (
                <CommandItem key={dep.id} onClick={() => go(`/deployments/${dep.id}`)}>
                  <ServerIcon />
                  {dep.application} #{dep.number}
                </CommandItem>
              ))}
            </CommandGroup>
          </>
        ) : null}
      </CommandList>
    </CommandDialog>
  )
}
