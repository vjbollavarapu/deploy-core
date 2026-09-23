import { ShieldAlert } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/platform/empty-state'
import { TONE_CLASSES } from '@/lib/status'
import type { StatusTone } from '@/lib/types'
import { getDashboardPanels } from '@/lib/dashboard'

function certTone(status: 'critical' | 'warning'): StatusTone {
  return status === 'critical' ? 'critical' : 'warning'
}

export function CertificateWarningsCard() {
  const { certificates } = getDashboardPanels()

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle className="flex items-center gap-2">
          <ShieldAlert className="size-3.5 text-muted-foreground" aria-hidden />
          Certificate Warnings
        </CardTitle>
      </CardHeader>
      <CardContent className={certificates.length === 0 ? 'py-4' : 'p-0'}>
        {certificates.length === 0 ? (
          <EmptyState
            icon={ShieldAlert}
            title="No certificate warnings"
            description="All TLS certificates are current."
            className="border-0 py-2"
          />
        ) : (
          <ul className="divide-y divide-border">
            {certificates.map((cert) => {
              const tone = TONE_CLASSES[certTone(cert.status)]
              return (
                <li
                  key={cert.domain}
                  className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm"
                >
                  <span className="truncate font-mono text-xs">{cert.domain}</span>
                  <span className={`shrink-0 text-xs font-medium ${tone.text}`}>
                    expires {cert.expiresIn}
                  </span>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
