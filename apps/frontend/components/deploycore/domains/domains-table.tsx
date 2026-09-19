import Link from 'next/link'
import { ShieldAlert, ShieldCheck } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { DomainTlsBadge } from '@/components/deploycore/domains/domain-tls-badge'
import { cn } from '@/lib/utils'
import { expiryToneClass } from '@/lib/domains'
import type { DomainRecord } from '@/lib/types'

interface DomainsTableProps {
  domains: DomainRecord[]
}

export function DomainsTable({ domains }: DomainsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Domain</TableHead>
          <TableHead>App</TableHead>
          <TableHead>Environment</TableHead>
          <TableHead>Port</TableHead>
          <TableHead>DNS</TableHead>
          <TableHead>HTTPS</TableHead>
          <TableHead>Certificate</TableHead>
          <TableHead>Expiry</TableHead>
          <TableHead>Status</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {domains.map((domain) => (
          <TableRow key={domain.id}>
            <TableCell>
              <div className="flex flex-wrap items-center gap-2">
                <Link
                  href={`/domains/${domain.id}`}
                  className="font-mono text-sm font-medium text-foreground hover:underline"
                >
                  {domain.domain}
                </Link>
                {domain.primary ? (
                  <Badge variant="secondary" className="text-[10px]">
                    Primary
                  </Badge>
                ) : null}
              </div>
            </TableCell>
            <TableCell>
              <Link
                href={`/applications/${domain.applicationId}`}
                className="text-foreground hover:underline"
              >
                {domain.application}
              </Link>
            </TableCell>
            <TableCell>
              <EnvironmentBadge environment={domain.environment} />
            </TableCell>
            <TableCell className="tabular text-muted-foreground">{domain.routingPort}</TableCell>
            <TableCell>
              <span
                className={cn(
                  'inline-flex items-center gap-1.5 text-sm',
                  domain.dnsVerified ? 'text-foreground' : 'text-muted-foreground',
                )}
              >
                {domain.dnsVerified ? (
                  <ShieldCheck className="size-3.5 text-success" />
                ) : (
                  <ShieldAlert className="size-3.5 text-warning" />
                )}
                {domain.dnsVerified ? 'Verified' : 'Pending'}
              </span>
            </TableCell>
            <TableCell>
              <Badge variant={domain.https ? 'secondary' : 'outline'} className="text-[10px]">
                {domain.https ? 'On' : 'Off'}
              </Badge>
            </TableCell>
            <TableCell className="text-sm text-muted-foreground">
              {domain.certificateIssuer ?? '—'}
            </TableCell>
            <TableCell>
              {domain.certExpiry === '—' ? (
                <span className="text-sm text-muted-foreground">—</span>
              ) : (
                <div className="flex flex-col gap-0.5">
                  <span
                    className={cn(
                      'text-sm',
                      expiryToneClass(domain.certExpiryDays, domain.https),
                    )}
                  >
                    {domain.certExpiryDays}d
                  </span>
                  <span className="text-[11px] text-muted-foreground">{domain.certExpiry}</span>
                </div>
              )}
            </TableCell>
            <TableCell>
              <DomainTlsBadge state={domain.tlsState} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
