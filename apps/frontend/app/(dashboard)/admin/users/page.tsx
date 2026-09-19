import { Card, CardContent } from '@/components/ui/card'
import { TeamMembersTable } from '@/components/deploycore/teams/team-members-table'
import { teamMembers } from '@/lib/mock-data'

export default function AdminUsersPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <TeamMembersTable members={teamMembers} />
      </CardContent>
    </Card>
  )
}
