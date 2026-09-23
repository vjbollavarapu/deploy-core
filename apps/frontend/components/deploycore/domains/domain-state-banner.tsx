import { AlertTriangle, CheckCircle2, Clock, Globe, Loader2, Lock, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { DomainRecord } from '@/lib/types'

interface DomainStateBannerProps {
  domain: DomainRecord
  onRevalidate?: () => void
  isRevalidating?: boolean
}

export function DomainStateBanner({
  domain,
  onRevalidate,
  isRevalidating,
}: DomainStateBannerProps) {
  const { tlsState } = domain

  if (tlsState === 'PENDING') {
    return (
      <div className="flex flex-col gap-3 rounded-lg border border-warning/40 bg-warning/10 p-4 text-sm text-foreground">
        <div className="flex items-start gap-3">
          <Clock className="mt-0.5 size-5 shrink-0 text-warning" />
          <div className="flex-1 space-y-1">
            <p className="font-semibold text-foreground">DNS Verification Required</p>
            <p className="text-muted-foreground">
              To activate SSL/TLS encryption for <span className="font-mono text-foreground">{domain.domain}</span>,
              create the required DNS record with your DNS provider. Verification will begin once the record propagates.
            </p>
          </div>
          {onRevalidate ? (
            <Button
              size="sm"
              variant="outline"
              disabled={isRevalidating}
              onClick={onRevalidate}
              className="shrink-0"
            >
              {isRevalidating ? <Loader2 className="size-3.5 animate-spin" data-icon="inline-start" /> : <Globe data-icon="inline-start" />}
              {isRevalidating ? 'Checking…' : 'Check DNS'}
            </Button>
          ) : null}
        </div>
      </div>
    )
  }

  if (tlsState === 'VERIFYING') {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-info/40 bg-info/10 p-4 text-sm text-foreground">
        <Loader2 className="mt-0.5 size-5 shrink-0 animate-spin text-info" />
        <div className="flex-1 space-y-1">
          <p className="font-semibold text-foreground">Verifying DNS Propagation</p>
          <p className="text-muted-foreground">
            DeployCore edge resolvers are actively querying authoritative nameservers for{' '}
            <span className="font-mono text-foreground">{domain.domain}</span>. This usually completes in 1–5 minutes.
          </p>
        </div>
      </div>
    )
  }

  if (tlsState === 'ISSUING') {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-info/40 bg-info/10 p-4 text-sm text-foreground">
        <Loader2 className="mt-0.5 size-5 shrink-0 animate-spin text-info" />
        <div className="flex-1 space-y-1">
          <p className="font-semibold text-foreground">Issuing TLS Certificate</p>
          <p className="text-muted-foreground">
            DNS verification succeeded. ACME challenge in flight with Let&apos;s Encrypt certificate authority.
            Your certificate is being generated and installed on edge load balancers.
          </p>
        </div>
      </div>
    )
  }

  if (tlsState === 'ACTIVE') {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-success/40 bg-success/10 p-4 text-sm text-foreground">
        <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-success" />
        <div className="flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-semibold text-foreground">Certificate Active &amp; Encrypted</p>
            <span className="inline-flex items-center gap-1 text-xs text-success">
              <Lock className="size-3" /> TLS 1.3
            </span>
          </div>
          <p className="text-muted-foreground">
            Traffic to <span className="font-mono text-foreground">{domain.domain}</span> is securely encrypted.
            Automatic renewal is managed by DeployCore before expiration.
          </p>
        </div>
      </div>
    )
  }

  if (tlsState === 'EXPIRING') {
    return (
      <div className="flex flex-col gap-3 rounded-lg border border-warning/40 bg-warning/10 p-4 text-sm text-foreground sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <AlertTriangle className="mt-0.5 size-5 shrink-0 text-warning" />
          <div className="space-y-1">
            <p className="font-semibold text-foreground">Certificate Expiring Soon</p>
            <p className="text-muted-foreground">
              Certificate expires in <span className="font-semibold text-foreground">{domain.certExpiryDays} days</span> ({domain.certExpiry}).
              Next automated renewal check is scheduled for {domain.nextRenewalAt}.
            </p>
          </div>
        </div>
        {onRevalidate ? (
          <Button
            size="sm"
            variant="outline"
            disabled={isRevalidating}
            onClick={onRevalidate}
            className="shrink-0"
          >
            {isRevalidating ? <Loader2 className="size-3.5 animate-spin" data-icon="inline-start" /> : <Lock data-icon="inline-start" />}
            {isRevalidating ? 'Renewing…' : 'Trigger renewal now'}
          </Button>
        ) : null}
      </div>
    )
  }

  if (tlsState === 'FAILED') {
    return (
      <div className="flex flex-col gap-3 rounded-lg border border-critical/40 bg-critical/10 p-4 text-sm text-foreground">
        <div className="flex items-start gap-3">
          <XCircle className="mt-0.5 size-5 shrink-0 text-critical" />
          <div className="flex-1 space-y-1">
            <p className="font-semibold text-foreground">Certificate Issuance Failed</p>
            <p className="text-muted-foreground">{domain.validationMessage}</p>
            <ul className="mt-2 list-disc space-y-1 pl-4 text-xs text-muted-foreground">
              <li>Ensure the DNS record matches the required value exactly without typographical errors.</li>
              <li>Verify that CAA records on your domain allow Let&apos;s Encrypt (<code className="font-mono">letsencrypt.org</code>).</li>
              <li>If using Cloudflare, disable &quot;Proxy mode&quot; (orange cloud) during initial verification.</li>
            </ul>
          </div>
          {onRevalidate ? (
            <Button
              size="sm"
              variant="outline"
              disabled={isRevalidating}
              onClick={onRevalidate}
              className="shrink-0"
            >
              {isRevalidating ? <Loader2 className="size-3.5 animate-spin" data-icon="inline-start" /> : <Globe data-icon="inline-start" />}
              {isRevalidating ? 'Retrying…' : 'Retry validation'}
            </Button>
          ) : null}
        </div>
      </div>
    )
  }

  return null
}
