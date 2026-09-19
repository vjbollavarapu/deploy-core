import { ShieldAlert } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { getDashboardPanels } from '@/lib/dashboard'

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
      <CardContent className="p-0">
        <ul className="divide-y divide-border">
          {certificates.map((cert) => (
            <li
              key={cert.domain}
              className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm"
            >
              <span className="truncate font-mono text-xs">{cert.domain}</span>
              <span
                className={cn(
                  'shrink-0 text-xs font-medium',
                  cert.status === 'critical' ? 'text-critical' : 'text-warning',
                )}
              >
                expires {cert.expiresIn}
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
