'use client'

import Link from 'next/link'
import { useMemo, useState, type ReactNode } from 'react'
import { Boxes } from 'lucide-react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { StatusBadge } from '@/components/platform/status-badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { Project } from '@/lib/types'

function initials(name: string) {
  return name
    .split(' ')
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

interface ProjectsTableProps {
  projects: Project[]
  actions?: ReactNode
}

export function ProjectsTable({ projects, actions }: ProjectsTableProps) {
  const [query, setQuery] = useState('')

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return projects
    return projects.filter(
      (project) =>
        project.name.toLowerCase().includes(q) ||
        project.slug.toLowerCase().includes(q) ||
        project.owner.name.toLowerCase().includes(q),
    )
  }, [projects, query])

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search projects…"
          searchValue={query}
          onSearchChange={setQuery}
          actions={actions}
        />
      }
    >
      {filtered.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={Boxes}
            title="No projects found"
            description={
              query
                ? 'Try a different search term.'
                : 'Create a project to group applications and environments.'
            }
            className="border-0"
          />
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Project</TableHead>
              <TableHead>Applications</TableHead>
              <TableHead>Environments</TableHead>
              <TableHead>Health</TableHead>
              <TableHead>Owner</TableHead>
              <TableHead>Latest deployment</TableHead>
              <TableHead>Updated</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((project) => (
              <TableRow key={project.id}>
                <TableCell>
                  <Link href={`/projects/${project.slug}`} className="flex flex-col gap-0.5 hover:underline">
                    <span className="font-medium text-foreground">{project.name}</span>
                    <span className="font-mono text-xs text-muted-foreground">{project.slug}</span>
                  </Link>
                </TableCell>
                <TableCell className="tabular">{project.applicationCount}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {project.environments.map((env) => (
                      <EnvironmentBadge key={env} environment={env} />
                    ))}
                  </div>
                </TableCell>
                <TableCell>
                  <StatusBadge status={project.health} />
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <Avatar className="size-5">
                      <AvatarFallback className="text-[10px]">
                        {initials(project.owner.name)}
                      </AvatarFallback>
                    </Avatar>
                    <span className="text-sm">{project.owner.name}</span>
                  </div>
                </TableCell>
                <TableCell className="text-muted-foreground">{project.lastDeployment}</TableCell>
                <TableCell className="text-muted-foreground">{project.updatedAt}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </DataTable>
  )
}
