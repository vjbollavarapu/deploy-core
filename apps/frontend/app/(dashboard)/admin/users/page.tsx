import { Card, CardContent } from '@/components/ui/card'
import { TeamMembersTable } from '@/components/deploycore/teams/team-members-table'
import { teamMembers as rawTeamMembers } from '@/lib/mock-data'
import { getDemoFixtures } from '@/lib/mock-isolation'

const teamMembers = getDemoFixtures(rawTeamMembers)

export default function AdminUsersPage() {
  return (
    <Card>
      <CardContent className="p-0">
        <TeamMembersTable members={teamMembers} />
      </CardContent>
    </Card>
  )
}
