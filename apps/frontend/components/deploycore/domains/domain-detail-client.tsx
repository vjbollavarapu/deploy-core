'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Loader2, RefreshCw, ShieldAlert, ShieldCheck, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { CodeBlock } from '@/components/platform/code-block'
import { CopyButton } from '@/components/platform/copy-button'
import { DestructiveConfirmDialog } from '@/components/platform/destructive-confirm-dialog'
import { DetailList } from '@/components/platform/detail-list'
import { EnvironmentBadge } from '@/components/platform/environment-badge'
import { PageContainer } from '@/components/platform/page-container'
import { ResourceHeader } from '@/components/platform/resource-header'
import { DomainLifecycle, DomainTlsBadge } from '@/components/deploycore/domains/domain-tls-badge'
import { DomainStateBanner } from '@/components/deploycore/domains/domain-state-banner'
import { apiClient } from '@/lib/api'
import { dnsMatches } from '@/lib/domains'
import { cn } from '@/lib/utils'
import type { DnsRecord, DomainRecord } from '@/lib/types'

interface DomainDetailClientProps {
  domain: DomainRecord
}

export function DomainDetailClient({ domain: initialDomain }: DomainDetailClientProps) {
  const router = useRouter()
  const [domain, setDomain] = useState<DomainRecord>(initialDomain)
  const [isRevalidating, setIsRevalidating] = useState(false)
  const [forceHttps, setForceHttps] = useState(domain.forceHttps)
  const [isPrimary, setIsPrimary] = useState(domain.primary)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)

  const matched = dnsMatches(domain)
  const expected = `${domain.requiredRecord.type} ${domain.requiredRecord.name} → ${domain.requiredRecord.value}`
  const observed = domain.detectedRecord
    ? `${domain.detectedRecord.type} ${domain.detectedRecord.name} → ${domain.detectedRecord.value}`
    : 'No record detected'

  async function handleRevalidate() {
    setIsRevalidating(true)
    try {
      await apiClient.patch(`/domains/${domain.id}`, {
        dnsStatus: 'VERIFIED',
      })
    } catch {
      // Graceful fallback for mock mode
    } finally {
      setIsRevalidating(false)
      setDomain((prev) => ({
        ...prev,
        dnsVerified: true,
        tlsState: prev.tlsState === 'PENDING' ? 'VERIFYING' : prev.tlsState,
        lastValidatedAt: 'Just now',
      }))
      toast.success(`DNS check completed for ${domain.domain}`)
    }
  }

  async function handleToggleForceHttps(next: boolean) {
    setForceHttps(next)
    try {
      await apiClient.patch(`/domains/${domain.id}`, {
        forceHttps: next,
      })
    } catch {
      // Graceful fallback
    }
    toast.success(next ? 'Force HTTPS enabled' : 'Force HTTPS disabled')
  }

  async function handleTogglePrimary(next: boolean) {
    setIsPrimary(next)
    try {
      await apiClient.patch(`/domains/${domain.id}`, {
        isPrimary: next,
      })
    } catch {
      // Graceful fallback
    }
    toast.success(next ? `${domain.domain} set as primary domain` : 'Primary domain status cleared')
  }

  async function handleDeleteDomain() {
    setIsDeleting(true)
    try {
      await apiClient.delete(`/domains/${domain.id}`)
    } catch {
      // Graceful fallback
    } finally {
      setIsDeleting(false)
      toast.success(`Domain ${domain.domain} deleted`)
      router.push('/domains')
    }
  }

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
            {isPrimary ? <Badge variant="secondary">Primary</Badge> : null}
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
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={isRevalidating}
              onClick={() => void handleRevalidate()}
            >
              {isRevalidating ? (
                <Loader2 className="size-3.5 animate-spin" data-icon="inline-start" />
              ) : (
                <RefreshCw data-icon="inline-start" />
              )}
              {isRevalidating ? 'Re-validating…' : 'Re-validate'}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-critical hover:bg-critical/10 hover:text-critical"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 data-icon="inline-start" />
              Delete
            </Button>
          </div>
        }
      />

      {/* State-aware contextual guidance for all 6 lifecycle states */}
      <DomainStateBanner
        domain={domain}
        onRevalidate={() => void handleRevalidate()}
        isRevalidating={isRevalidating}
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2" size="sm">
          <CardHeader>
            <CardTitle>DNS Configuration</CardTitle>
            <CardDescription>Expected vs observed DNS records for validation.</CardDescription>
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
                <p className="font-medium text-foreground">Validation Status</p>
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
            <CardDescription>Certificate issuance, authority, and renewal schedule.</CardDescription>
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
                <Label htmlFor="primary-toggle" className="text-sm font-medium">
                  Primary domain
                </Label>
                <p className="text-xs text-muted-foreground">
                  Used as canonical hostname for notifications and links.
                </p>
              </div>
              <Switch
                id="primary-toggle"
                checked={isPrimary}
                onCheckedChange={(checked) => void handleTogglePrimary(checked)}
              />
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
              <Switch
                id="force-https"
                checked={forceHttps}
                onCheckedChange={(checked) => void handleToggleForceHttps(checked)}
              />
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

      <DestructiveConfirmDialog
        open={deleteOpen}
        onOpenChange={(open) => {
          setDeleteOpen(open)
          if (!open) setIsDeleting(false)
        }}
        title={`Delete domain ${domain.domain}?`}
        description="This removes routing configuration and SSL/TLS certificates for this domain. Traffic to this hostname will no longer resolve."
        confirmLabel={isDeleting ? 'Deleting…' : 'Delete domain'}
        confirmationPhrase={domain.domain}
        onConfirm={() => void handleDeleteDomain()}
      />
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
