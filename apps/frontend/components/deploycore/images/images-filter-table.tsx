'use client'

import { useMemo, useState } from 'react'
import { Package } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ImagesTable } from './images-table'
import type { ContainerImage } from '@/lib/types'

interface ImagesFilterTableProps {
  images: ContainerImage[]
}

export function ImagesFilterTable({ images }: ImagesFilterTableProps) {
  const [query, setQuery] = useState('')
  const [application, setApplication] = useState('all')

  const availableApplications = useMemo(
    () => Array.from(new Set(images.map((img) => img.application).filter(Boolean))),
    [images],
  )

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return images.filter((img) => {
      const matchesQuery =
        q === '' ||
        img.name.toLowerCase().includes(q) ||
        img.tag.toLowerCase().includes(q) ||
        img.digest.toLowerCase().includes(q) ||
        img.location.toLowerCase().includes(q) ||
        img.application.toLowerCase().includes(q)

      const matchesApp = application === 'all' || img.application === application

      return matchesQuery && matchesApp
    })
  }, [images, query, application])

  const hasFilters = query.trim() !== '' || application !== 'all'

  function clearFilters() {
    setQuery('')
    setApplication('all')
  }

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search images by repository, tag, digest…"
          searchValue={query}
          onSearchChange={setQuery}
          filters={
            <Select value={application} onValueChange={(v) => setApplication(v ?? 'all')}>
              <SelectTrigger className="w-48" aria-label="Filter by application">
                <SelectValue placeholder="Application" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All applications</SelectItem>
                {availableApplications.map((app) => (
                  <SelectItem key={app} value={app}>
                    {app}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          }
        />
      }
    >
      {filtered.length === 0 ? (
        <div className="p-8">
          <EmptyState
            icon={Package}
            title={hasFilters ? 'No images match your filters' : 'No images found'}
            description={
              hasFilters
                ? 'Try broadening your search term or clearing active filters.'
                : 'Built and registry-pulled container images will appear here.'
            }
            action={
              hasFilters ? (
                <Button size="sm" variant="outline" onClick={clearFilters}>
                  Clear filters
                </Button>
              ) : undefined
            }
            className="border-0"
          />
        </div>
      ) : (
        <div className="overflow-x-auto">
          <ImagesTable images={filtered} />
        </div>
      )}
    </DataTable>
  )
}
