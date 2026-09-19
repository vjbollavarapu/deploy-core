import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Users } from 'lucide-react'
import type { Team } from '@/lib/types'

interface TeamsGridProps {
  teams: Team[]
}

export function TeamsGrid({ teams }: TeamsGridProps) {
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      {teams.map((team) => (
        <Card key={team.id}>
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle className="text-base">{team.name}</CardTitle>
            <Badge variant="secondary" className="gap-1 text-[10px]">
              <Users data-icon="inline-start" className="size-3" />
              {team.memberCount}
            </Badge>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-1.5">
              {team.members.map((m) => (
                <Badge key={m} variant="outline" className="text-[10px]">
                  {m}
                </Badge>
              ))}
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
