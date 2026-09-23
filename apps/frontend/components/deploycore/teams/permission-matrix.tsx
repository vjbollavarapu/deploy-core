'use client'

import { useMemo, useState } from 'react'
import { Check, Minus, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import {
  CAPABILITIES,
  CAPABILITY_CATEGORIES,
  hasCapability,
  type DefaultRoleName,
} from '@/lib/rbac'
import type { PermissionLevel } from '@/lib/types'
import { cn } from '@/lib/utils'

interface PermissionMatrixProps {
  roles?: readonly string[]
  resources?: readonly string[]
  matrix?: Record<string, Record<string, PermissionLevel>>
}

const levelStyles: Record<PermissionLevel, string> = {
  full: 'bg-success/15 text-success',
  edit: 'bg-info/15 text-info',
  view: 'bg-muted text-muted-foreground',
  none: 'bg-transparent text-muted-foreground/40',
}

const DEFAULT_ROLE_NAMES: DefaultRoleName[] = [
  'Owner',
  'Administrator',
  'DevOps',
  'Developer',
  'Support',
  'Viewer',
]

export function PermissionMatrix({
  roles = DEFAULT_ROLE_NAMES,
  resources = [],
  matrix = {},
}: PermissionMatrixProps) {
  const [viewMode, setViewMode] = useState<'capability' | 'resource'>('capability')
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<string>('all')

  const filteredCapabilities = useMemo(() => {
    return CAPABILITIES.filter((cap) => {
      if (selectedCategory !== 'all' && cap.category !== selectedCategory) {
        return false
      }
      if (!searchQuery.trim()) return true
      const q = searchQuery.toLowerCase()
      return (
        cap.name.toLowerCase().includes(q) ||
        cap.key.toLowerCase().includes(q) ||
        cap.description.toLowerCase().includes(q) ||
        cap.category.toLowerCase().includes(q)
      )
    })
  }, [searchQuery, selectedCategory])

  // Group capabilities by category
  const groupedCapabilities = useMemo(() => {
    const map = new Map<string, typeof CAPABILITIES>()
    for (const cap of filteredCapabilities) {
      if (!map.has(cap.category)) {
        map.set(cap.category, [])
      }
      map.get(cap.category)!.push(cap)
    }
    return map
  }, [filteredCapabilities])

  return (
    <div className="flex flex-col gap-4">
      {/* View mode toggle & Search */}
      <div className="flex flex-col gap-3 p-4 md:flex-row md:items-center md:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-64">
            <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
            <Input
              placeholder="Search capability or key…"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-8 text-xs"
            />
          </div>
          <select
            value={selectedCategory}
            onChange={(e) => setSelectedCategory(e.target.value)}
            className="h-9 rounded-md border border-input bg-background px-3 py-1 text-xs text-foreground shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            aria-label="Filter by category"
          >
            <option value="all">All Domains ({CAPABILITIES.length})</option>
            {CAPABILITY_CATEGORIES.map((cat) => (
              <option key={cat} value={cat}>
                {cat}
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-1 rounded-md border border-border bg-secondary/40 p-1">
          <Button
            variant={viewMode === 'capability' ? 'secondary' : 'ghost'}
            size="sm"
            className="h-7 text-xs"
            onClick={() => setViewMode('capability')}
          >
            Capability Matrix
          </Button>
          <Button
            variant={viewMode === 'resource' ? 'secondary' : 'ghost'}
            size="sm"
            className="h-7 text-xs"
            onClick={() => setViewMode('resource')}
          >
            Resource Overview
          </Button>
        </div>
      </div>

      {viewMode === 'capability' ? (
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="min-w-[280px]">Capability</TableHead>
                {DEFAULT_ROLE_NAMES.map((role) => (
                  <TableHead key={role} className="min-w-[100px] text-center">
                    <span className="font-semibold">{role}</span>
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {Array.from(groupedCapabilities.entries()).map(([category, items]) => (
                <>
                  <TableRow key={category} className="bg-muted/40 hover:bg-muted/40">
                    <TableCell
                      colSpan={DEFAULT_ROLE_NAMES.length + 1}
                      className="py-1.5 text-xs font-semibold uppercase tracking-wider text-muted-foreground"
                    >
                      {category}
                    </TableCell>
                  </TableRow>
                  {items.map((cap) => (
                    <TableRow key={cap.key}>
                      <TableCell>
                        <div className="flex flex-col gap-0.5">
                          <span className="text-xs font-medium text-foreground">{cap.name}</span>
                          <span className="font-mono text-[10px] text-muted-foreground">
                            {cap.key}
                          </span>
                        </div>
                      </TableCell>
                      {DEFAULT_ROLE_NAMES.map((role) => {
                        const granted = hasCapability(role, cap.key)
                        return (
                          <TableCell key={role} className="text-center">
                            {granted ? (
                              <span className="inline-flex size-6 items-center justify-center rounded-full bg-success/15 text-success">
                                <Check className="size-3.5" />
                              </span>
                            ) : (
                              <span className="inline-flex size-6 items-center justify-center rounded-full text-muted-foreground/30">
                                <Minus className="size-3.5" />
                              </span>
                            )}
                          </TableCell>
                        )
                      })}
                    </TableRow>
                  ))}
                </>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Resource</TableHead>
                {roles.map((role) => (
                  <TableHead key={role} className="text-center">
                    {role}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {resources.map((resource) => (
                <TableRow key={resource}>
                  <TableCell className="font-medium text-foreground">{resource}</TableCell>
                  {roles.map((role) => {
                    const level = matrix[role]?.[resource] ?? 'none'
                    return (
                      <TableCell key={role} className="text-center">
                        <span
                          className={cn(
                            'inline-flex min-w-16 items-center justify-center rounded-md px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                            levelStyles[level],
                          )}
                        >
                          {level}
                        </span>
                      </TableCell>
                    )
                  })}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
