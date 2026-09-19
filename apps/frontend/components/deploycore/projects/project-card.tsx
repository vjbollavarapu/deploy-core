import Link from 'next/link'
import { Boxes } from 'lucide-react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { StatusBadge } from '@/components/platform/status-badge'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import type { Project } from '@/lib/types'

function initials(name: string) {
  return name
    .split(' ')
    .map((p) => p[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

export function ProjectCard({ project }: { project: Project }) {
  return (
    <Link
      href={`/projects/${project.slug}`}
      className="flex flex-col gap-4 rounded-lg border border-border bg-card p-4 transition-colors hover:border-primary/40 hover:bg-muted/20"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2.5">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-accent text-accent-foreground">
            <Boxes className="size-4" />
          </div>
          <div className="flex flex-col gap-0.5">
            <span className="text-sm font-semibold text-foreground">{project.name}</span>
            <span className="font-mono text-xs text-muted-foreground">{project.slug}</span>
          </div>
        </div>
        <StatusBadge status={project.health} />
      </div>
      <div className="flex flex-wrap gap-1.5">
        {project.environments.map((env) => (
          <EnvironmentBadge key={env} environment={env} />
        ))}
      </div>
      <div className="flex items-center justify-between gap-2 border-t border-border pt-3 text-xs text-muted-foreground">
        <span>{project.applicationCount} applications</span>
        <span>Deployed {project.lastDeployment}</span>
      </div>
      <div className="flex items-center gap-2">
        <Avatar className="size-5">
          <AvatarFallback className="text-[10px]">{initials(project.owner.name)}</AvatarFallback>
        </Avatar>
        <span className="text-xs text-muted-foreground">{project.owner.name}</span>
      </div>
    </Link>
  )
}
