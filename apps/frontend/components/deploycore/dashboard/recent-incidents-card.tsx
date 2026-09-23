import { AlertTriangle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/platform/empty-state'
import { TONE_CLASSES } from '@/lib/status'
import type { StatusTone } from '@/lib/types'
import { getDashboardPanels } from '@/lib/dashboard'

function severityTone(severity: 'critical' | 'warning'): StatusTone {
  return severity === 'critical' ? 'critical' : 'warning'
}

export function RecentIncidentsCard() {
  const { incidents } = getDashboardPanels()

  return (
    <Card size="sm">
      <CardHeader className="border-b">
        <CardTitle className="flex items-center gap-2">
          <AlertTriangle className="size-3.5 text-muted-foreground" aria-hidden />
          Recent Incidents
        </CardTitle>
      </CardHeader>
      <CardContent className={incidents.length === 0 ? 'py-4' : 'p-0'}>
        {incidents.length === 0 ? (
          <EmptyState
            icon={AlertTriangle}
            title="No recent incidents"
            description="No active or recent incidents detected."
            className="border-0 py-2"
          />
        ) : (
          <ul className="divide-y divide-border">
            {incidents.map((incident) => {
              const tone = TONE_CLASSES[severityTone(incident.severity)]
              return (
                <li
                  key={incident.id}
                  className="flex items-start justify-between gap-3 px-4 py-2.5"
                >
                  <div className="min-w-0">
                    <p className="text-sm text-pretty">{incident.title}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      Started {incident.startedAt}
                    </p>
                  </div>
                  <span
                    className={`inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium ${tone.bg} ${tone.text} ${tone.border}`}
                  >
                    <span className={`size-1.5 shrink-0 rounded-full ${tone.dot}`} />
                    {incident.status}
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
