'use client'

import { useMemo, useState } from 'react'
import { Globe } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/platform/data-table'
import { DataTableToolbar } from '@/components/platform/data-table-toolbar'
import { EmptyState } from '@/components/platform/empty-state'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DomainsTable } from './domains-table'
import { DNS_STATUS_FILTERS, DOMAIN_STATUS_FILTERS } from '@/lib/domains'
import type { DomainRecord } from '@/lib/types'

interface DomainsFilterTableProps {
  domains: DomainRecord[]
}

export function DomainsFilterTable({ domains }: DomainsFilterTableProps) {
  const [query, setQuery] = useState('')
  const [environment, setEnvironment] = useState('all')
  const [tlsState, setTlsState] = useState('all')
  const [dnsStatus, setDnsStatus] = useState('all')

  const availableEnvironments = useMemo(
    () => Array.from(new Set(domains.map((d) => d.environment).filter(Boolean))),
    [domains],
  )

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return domains.filter((d) => {
      const matchesQuery =
        q === '' ||
        d.domain.toLowerCase().includes(q) ||
        d.application.toLowerCase().includes(q) ||
        (d.certificateIssuer && d.certificateIssuer.toLowerCase().includes(q)) ||
        String(d.routingPort).includes(q)

      const matchesEnv = environment === 'all' || d.environment === environment
      const matchesTls = tlsState === 'all' || d.tlsState === tlsState
      const matchesDns =
        dnsStatus === 'all' ||
        (dnsStatus === 'verified' && d.dnsVerified) ||
        (dnsStatus === 'pending' && !d.dnsVerified)

      return matchesQuery && matchesEnv && matchesTls && matchesDns
    })
  }, [domains, query, environment, tlsState, dnsStatus])

  const hasFilters =
    query.trim() !== '' || environment !== 'all' || tlsState !== 'all' || dnsStatus !== 'all'

  function clearFilters() {
    setQuery('')
    setEnvironment('all')
    setTlsState('all')
    setDnsStatus('all')
  }

  return (
    <DataTable
      toolbar={
        <DataTableToolbar
          searchPlaceholder="Search domains by hostname, application, port…"
          searchValue={query}
          onSearchChange={setQuery}
          filters={
            <>
              <Select value={environment} onValueChange={(v) => setEnvironment(v ?? 'all')}>
                <SelectTrigger className="w-40" aria-label="Filter by environment">
                  <SelectValue placeholder="Environment" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All environments</SelectItem>
                  {availableEnvironments.map((env) => (
                    <SelectItem key={env} value={env}>
                      {env}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={tlsState} onValueChange={(v) => setTlsState(v ?? 'all')}>
                <SelectTrigger className="w-36" aria-label="Filter by TLS state">
                  <SelectValue placeholder="TLS state" />
                </SelectTrigger>
                <SelectContent>
                  {DOMAIN_STATUS_FILTERS.map((s) => (
                    <SelectItem key={s.value} value={s.value}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={dnsStatus} onValueChange={(v) => setDnsStatus(v ?? 'all')}>
                <SelectTrigger className="w-32" aria-label="Filter by DNS">
                  <SelectValue placeholder="DNS status" />
                </SelectTrigger>
                <SelectContent>
                  {DNS_STATUS_FILTERS.map((d) => (
                    <SelectItem key={d.value} value={d.value}>
                      {d.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </>
          }
        />
      }
    >
      {filtered.length === 0 ? (
        <div className="p-8">
          <EmptyState
            icon={Globe}
            title={hasFilters ? 'No domains match your filters' : 'No custom domains configured'}
            description={
              hasFilters
                ? 'Try broadening your search term or clearing active filters.'
                : 'Attach a domain to any of your applications to manage SSL/TLS routing.'
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
          <DomainsTable domains={filtered} />
        </div>
      )}
    </DataTable>
  )
}
