import { notFound } from 'next/navigation'
import Link from 'next/link'
import { Check, RefreshCw, ShieldAlert, ShieldCheck, X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { CodeBlock } from '@/components/platform/code-block'
import { DetailList } from '@/components/platform/detail-list'
import { CopyButton } from '@/components/platform/copy-button'
import {
  DomainLifecycle,
  DomainTlsBadge,
} from '@/components/deploycore/domains/domain-tls-badge'
import { dnsMatches, findDomain } from '@/lib/domains'
import { domains } from '@/lib/mock-data'
import { cn } from '@/lib/utils'
import type { DnsRecord } from '@/lib/types'

export default async function DomainDetailPage({
  params,
}: {
  params: Promise<{ domainId: string }>
}) {
  const { domainId } = await params
  const domain = findDomain(domainId, domains)
  if (!domain) notFound()

  const matched = dnsMatches(domain)
  const expected = `${domain.requiredRecord.type} ${domain.requiredRecord.name} → ${domain.requiredRecord.value}`
  const observed = domain.detectedRecord
    ? `${domain.detectedRecord.type} ${domain.detectedRecord.name} → ${domain.detectedRecord.value}`
    : 'No record detected'

  return (
    <PageContainer density="wide">
      <ResourceHeader
        title={domain.domain}
        description={`Routes to ${domain.application} on port ${domain.routingPort}`}
        breadcrumbs={[
          { label: 'Domains', href: '/domains' },
          { label: domain.domain },
        ]}
        badges={
          <>
            <DomainTlsBadge state={domain.tlsState} />
            <EnvironmentBadge environment={domain.environment} />
            {domain.primary ? <Badge variant="secondary">Primary</Badge> : null}
          </>
        }
        meta={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <Link
              href={`/applications/${domain.applicationId}`}
              className="hover:text-foreground hover:underline"
            >
              {domain.application}
            </Link>
            <span>Port {domain.routingPort}</span>
            {domain.lastValidatedAt ? <span>Validated {domain.lastValidatedAt}</span> : null}
          </div>
        }
        actions={
          <Button size="sm" variant="outline">
            <RefreshCw data-icon="inline-start" />
            Re-validate
          </Button>
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2" size="sm">
          <CardHeader>
            <CardTitle>DNS</CardTitle>
            <CardDescription>Expected vs observed records for validation.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <DnsCard
                title="Expected DNS"
                record={domain.requiredRecord}
                tone="neutral"
              />
              <DnsCard
                title="Observed DNS"
                record={domain.detectedRecord}
                tone={domain.detectedRecord ? (matched ? 'success' : 'warning') : 'critical'}
                emptyMessage="No record detected yet. Propagation can take up to 48 hours."
              />
            </div>
            <div
              className={cn(
                'flex items-start gap-2 rounded-lg border px-3 py-2.5 text-sm',
                matched
                  ? 'border-success/30 bg-success/10 text-success'
                  : 'border-warning/30 bg-warning/10 text-warning',
              )}
            >
              {matched ? (
                <ShieldCheck className="mt-0.5 size-4 shrink-0" />
              ) : (
                <ShieldAlert className="mt-0.5 size-4 shrink-0" />
              )}
              <div>
                <p className="font-medium text-foreground">Validation</p>
                <p className="text-muted-foreground">{domain.validationMessage}</p>
              </div>
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              <CodeBlock code={expected} label="Expected" />
              <CodeBlock code={observed} label="Observed" />
            </div>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Lifecycle</CardTitle>
            <CardDescription>
              PENDING → VERIFYING → ISSUING → ACTIVE (plus EXPIRING / FAILED).
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DomainLifecycle state={domain.tlsState} />
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card size="sm">
          <CardHeader>
            <CardTitle>Certificate</CardTitle>
            <CardDescription>Issuance and renewal metadata.</CardDescription>
          </CardHeader>
          <CardContent>
            <DetailList
              columns={2}
              items={[
                {
                  label: 'Issuer',
                  value: domain.certificateIssuer ?? '—',
                },
                {
                  label: 'Issued',
                  value: domain.certificateIssuedAt ?? '—',
                },
                {
                  label: 'Expires',
                  value:
                    domain.certExpiry === '—'
                      ? '—'
                      : `${domain.certExpiry} (${domain.certExpiryDays}d)`,
                },
                {
                  label: 'Next renewal',
                  value: domain.nextRenewalAt ?? '—',
                },
                {
                  label: 'HTTPS',
                  value: domain.https ? 'Enabled' : 'Disabled',
                },
                {
                  label: 'TLS state',
                  value: <DomainTlsBadge state={domain.tlsState} />,
                },
              ]}
            />
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle>Routing</CardTitle>
            <CardDescription>Primary domain, HTTPS enforcement, and redirects.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2.5">
              <div>
                <Label className="text-sm font-medium">Primary domain</Label>
                <p className="text-xs text-muted-foreground">
                  Used as the canonical hostname for this application.
                </p>
              </div>
              <div className="flex items-center gap-2">
                {domain.primary ? (
                  <Check className="size-4 text-success" />
                ) : (
                  <X className="size-4 text-muted-foreground" />
                )}
                <span className="text-sm">{domain.primary ? 'Yes' : 'No'}</span>
              </div>
            </div>
            <div className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2.5">
              <div>
                <Label htmlFor="force-https" className="text-sm font-medium">
                  Force HTTPS
                </Label>
                <p className="text-xs text-muted-foreground">
                  Redirect HTTP traffic to HTTPS when a certificate is active.
                </p>
              </div>
              <Switch id="force-https" checked={domain.forceHttps} disabled />
            </div>
            <div>
              <p className="mb-2 text-xs font-medium text-muted-foreground">Redirect rules</p>
              {domain.redirectRules.length === 0 ? (
                <p className="text-sm text-muted-foreground">No redirect rules configured.</p>
              ) : (
                <ul className="divide-y divide-border rounded-lg border border-border">
                  {domain.redirectRules.map((rule) => (
                    <li
                      key={`${rule.from}-${rule.to}-${rule.code}`}
                      className="flex flex-wrap items-center gap-2 px-3 py-2 text-sm"
                    >
                      <span className="font-mono text-xs">{rule.from}</span>
                      <span className="text-muted-foreground">→</span>
                      <span className="font-mono text-xs">{rule.to}</span>
                      <Badge variant="secondary" className="text-[10px]">
                        {rule.code}
                      </Badge>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </CardContent>
        </Card>
      </div>
    </PageContainer>
  )
}

function DnsCard({
  title,
  record,
  tone,
  emptyMessage,
}: {
  title: string
  record: DnsRecord | null
  tone: 'neutral' | 'success' | 'warning' | 'critical'
  emptyMessage?: string
}) {
  return (
    <div
      className={cn(
        'rounded-lg border p-3',
        tone === 'neutral' && 'border-border',
        tone === 'success' && 'border-success/30 bg-success/5',
        tone === 'warning' && 'border-warning/30 bg-warning/5',
        tone === 'critical' && 'border-critical/30 bg-critical/5',
      )}
    >
      <p className="mb-2 text-xs font-medium text-muted-foreground">{title}</p>
      {record ? (
        <dl className="space-y-1.5 font-mono text-xs">
          <div className="flex justify-between gap-2">
            <dt className="text-muted-foreground">Type</dt>
            <dd className="flex items-center gap-1">
              {record.type}
              <CopyButton value={record.type} className="size-6" />
            </dd>
          </div>
          <div className="flex justify-between gap-2">
            <dt className="text-muted-foreground">Name</dt>
            <dd className="flex items-center gap-1">
              {record.name}
              <CopyButton value={record.name} className="size-6" />
            </dd>
          </div>
          <div className="flex justify-between gap-2">
            <dt className="text-muted-foreground">Value</dt>
            <dd className="flex max-w-[70%] items-center gap-1">
              <span className="truncate">{record.value}</span>
              <CopyButton value={record.value} className="size-6" />
            </dd>
          </div>
        </dl>
      ) : (
        <p className="text-xs text-critical">{emptyMessage}</p>
      )}
    </div>
  )
}
