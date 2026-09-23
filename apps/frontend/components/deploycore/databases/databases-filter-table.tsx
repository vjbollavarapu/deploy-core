'use client'

import { useMemo, useState } from 'react'
import Link from 'next/link'
import { Database, Search } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { FilterBar } from '@/components/platform/filter-bar'
import { EmptyState } from '@/components/platform/empty-state'
import { ResourceUsageBar } from '@/components/platform/resource-usage-bar'
import { StatusBadge } from '@/components/platform/status-badge'
import {
  DATABASE_ENGINE_FILTERS,
  DATABASE_STATUS_FILTERS,
} from '@/lib/databases'
import type { DatabaseInstance } from '@/lib/types'

interface DatabasesFilterTableProps {
  databases: DatabaseInstance[]
  headerAction?: React.ReactNode
}

export function DatabasesFilterTable({ databases, headerAction }: DatabasesFilterTableProps) {
  const [query, setQuery] = useState('')
  const [environment, setEnvironment] = useState('all')
  const [engine, setEngine] = useState('all')
  const [status, setStatus] = useState('all')

  const environments = useMemo(() => {
    const set = new Set(databases.map((db) => db.environment))
    return Array.from(set).sort()
  }, [databases])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return databases.filter((db) => {
      if (environment !== 'all' && db.environment !== environment) return false
      if (engine !== 'all' && db.type !== engine) return false
      if (status !== 'all' && db.status !== status) return false
      if (!q) return true
      return (
        db.name.toLowerCase().includes(q) ||
        db.project.toLowerCase().includes(q) ||
        db.server.toLowerCase().includes(q) ||
        db.dbName.toLowerCase().includes(q)
      )
    })
  }, [databases, query, environment, engine, status])

  function resetFilters() {
    setQuery('')
    setEnvironment('all')
    setEngine('all')
    setStatus('all')
  }

  const isFiltered = query || environment !== 'all' || engine !== 'all' || status !== 'all'

  return (
    <div className="flex flex-col gap-4">
      <FilterBar end={headerAction}>
        <InputGroup className="max-w-xs">
          <InputGroupInput
            placeholder="Search databases…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
        </InputGroup>

        <Select value={environment} onValueChange={(val) => setEnvironment(val ?? 'all')}>
          <SelectTrigger className="w-36">
            <SelectValue placeholder="Environment" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All environments</SelectItem>
            {environments.map((env) => (
              <SelectItem key={env} value={env}>
                {env}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={engine} onValueChange={(val) => setEngine(val ?? 'all')}>
          <SelectTrigger className="w-36">
            <SelectValue placeholder="Engine" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All engines</SelectItem>
            {DATABASE_ENGINE_FILTERS.map((eng) => (
              <SelectItem key={eng.value} value={eng.value}>
                {eng.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={status} onValueChange={(val) => setStatus(val ?? 'all')}>
          <SelectTrigger className="w-32">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            {DATABASE_STATUS_FILTERS.map((st) => (
              <SelectItem key={st.value} value={st.value}>
                {st.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FilterBar>

      <div className="rounded-lg border border-border overflow-hidden">
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Instance</TableHead>
                <TableHead>Project</TableHead>
                <TableHead>Server</TableHead>
                <TableHead>Storage</TableHead>
                <TableHead>Backups</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((db) => (
                <TableRow key={db.id} className="cursor-pointer">
                  <TableCell>
                    <Link href={`/databases/${db.id}`} className="flex flex-col gap-1 hover:underline">
                      <span className="font-medium text-foreground">{db.name}</span>
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <Badge variant="secondary" className="font-mono text-[10px]">
                          {db.type}
                        </Badge>
                        {db.version}
                      </span>
                    </Link>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-0.5">
                      <span className="text-foreground">{db.project}</span>
                      <span className="text-xs text-muted-foreground">{db.environment}</span>
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{db.server}</TableCell>
                  <TableCell>
                    <ResourceUsageBar
                      label=""
                      value={Math.round((db.storageUsedGb / db.storageTotalGb) * 100)}
                      detail={`${db.storageUsedGb} / ${db.storageTotalGb} GB`}
                      size="sm"
                      className="w-40"
                    />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-0.5 text-sm">
                      <span className="text-foreground">{db.backups} backups</span>
                      <span className="text-xs text-muted-foreground">Last: {db.lastBackup}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={db.status} showDot />
                  </TableCell>
                </TableRow>
              ))}
              {filtered.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="py-12">
                    <EmptyState
                      icon={Database}
                      title="No databases found"
                      description={
                        isFiltered
                          ? 'No managed databases match your current filter parameters.'
                          : 'No managed database instances configured yet.'
                      }
                      action={
                        isFiltered ? (
                          <Button variant="outline" size="sm" onClick={resetFilters}>
                            Reset filters
                          </Button>
                        ) : undefined
                      }
                      className="border-0"
                    />
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </div>
      </div>
    </div>
  )
}
