import { AlertTriangle } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { getDashboardPanels } from '@/lib/dashboard'

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
      <CardContent className="p-0">
        <ul className="divide-y divide-border">
          {incidents.map((incident) => (
            <li key={incident.id} className="flex items-start justify-between gap-3 px-4 py-2.5">
              <div className="min-w-0">
                <p className="text-sm text-pretty">{incident.title}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">Started {incident.startedAt}</p>
              </div>
              <Badge
                variant={incident.severity === 'critical' ? 'destructive' : 'secondary'}
                className="shrink-0"
              >
                {incident.status}
              </Badge>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
