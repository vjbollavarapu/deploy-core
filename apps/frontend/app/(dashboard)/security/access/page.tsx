import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { InvitationsTable } from '@/components/deploycore/teams/invitations-table'
import { PermissionMatrix } from '@/components/deploycore/teams/permission-matrix'
import { RolesList } from '@/components/deploycore/teams/roles-list'
import { TeamMembersTable } from '@/components/deploycore/teams/team-members-table'
import { TeamsGrid } from '@/components/deploycore/teams/teams-grid'
import {
  invitations,
  permissionMatrix,
  permissionResources,
  roleNames,
  teamMembers,
  teams,
} from '@/lib/mock-data'

export default function AccessControlPage() {
  return (
    <PageContainer density="wide">
      <PageHeader
        title="Access control"
        description="Users, teams, default roles, permission matrix, and invitations."
      />

      <Tabs defaultValue="users">
        <TabsList variant="line">
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="teams">Teams</TabsTrigger>
          <TabsTrigger value="roles">Roles</TabsTrigger>
          <TabsTrigger value="permissions">Permissions</TabsTrigger>
          <TabsTrigger value="invitations">Invitations</TabsTrigger>
        </TabsList>

        <TabsContent value="users" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Users</CardTitle>
              <CardDescription>Organization members and assigned default roles.</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <TeamMembersTable members={teamMembers} />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="teams" className="mt-4">
          <TeamsGrid teams={teams} />
        </TabsContent>

        <TabsContent value="roles" className="mt-4">
          <RolesList />
        </TabsContent>

        <TabsContent value="permissions" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Permission matrix</CardTitle>
              <CardDescription>
                Capability grants by role across {permissionResources.length} resources.
              </CardDescription>
            </CardHeader>
            <CardContent className="overflow-x-auto p-0">
              <PermissionMatrix
                roles={roleNames}
                resources={permissionResources}
                matrix={permissionMatrix}
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="invitations" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Invitations</CardTitle>
              <CardDescription>Pending and historical organization invites.</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <InvitationsTable invitations={invitations} />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </PageContainer>
  )
}
