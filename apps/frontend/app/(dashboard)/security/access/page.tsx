'use client'

import { useCallback, useState } from 'react'
import { toast } from 'sonner'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageContainer } from '@/components/platform/page-container'
import { PageHeader } from '@/components/platform/page-header'
import { LoadingState } from '@/components/platform/loading-state'
import { ErrorState } from '@/components/platform/error-state'
import { InvitationsTable } from '@/components/deploycore/teams/invitations-table'
import { PermissionMatrix } from '@/components/deploycore/teams/permission-matrix'
import { RolesList } from '@/components/deploycore/teams/roles-list'
import { TeamMembersTable } from '@/components/deploycore/teams/team-members-table'
import { TeamsGrid } from '@/components/deploycore/teams/teams-grid'
import { useOrganization } from '@/lib/auth-context'
import { useApiQuery } from '@/hooks/use-api-query'
import {
  createOrganizationInvitation,
  DEFAULT_ROLES,
  fetchOrganizationInvitations,
  fetchOrganizationMembers,
  mapWireInvitationToInvitation,
  mapWireMemberToTeamMember,
  removeMember,
  revokeOrganizationInvitation,
  updateMemberRoles,
} from '@/lib/rbac'
import {
  invitations as rawInvitations,
  permissionMatrix,
  permissionResources,
  roleNames,
  teamMembers as rawTeamMembers,
  teams as rawTeams,
} from '@/lib/mock-data'
import { getDemoFixtures, isDemoModeEnabled } from '@/lib/mock-isolation'
import type { Invitation, Team, TeamMember } from '@/lib/types'

export default function AccessControlPage() {
  const { activeOrg } = useOrganization()
  const orgId = activeOrg?.id || ''
  const isDemo = isDemoModeEnabled()

  // Local functional teams state (with mock isolation)
  const [teams, setTeams] = useState<Team[]>(() => getDemoFixtures(rawTeams))

  // 1. Members Query
  const loadMembers = useCallback(async (): Promise<TeamMember[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawTeamMembers) : []
    try {
      const res = await fetchOrganizationMembers(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map(mapWireMemberToTeamMember)
      }
      return isDemo ? getDemoFixtures(rawTeamMembers) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawTeamMembers)
      throw err
    }
  }, [orgId, isDemo])

  const {
    data: membersData,
    isLoading: isMembersLoading,
    error: membersError,
    reload: reloadMembers,
  } = useApiQuery(loadMembers)

  const members = membersData || []

  // 2. Invitations Query
  const loadInvitations = useCallback(async (): Promise<Invitation[]> => {
    if (!orgId) return isDemo ? getDemoFixtures(rawInvitations) : []
    try {
      const res = await fetchOrganizationInvitations(orgId)
      if (res?.items && res.items.length > 0) {
        return res.items.map(mapWireInvitationToInvitation)
      }
      return isDemo ? getDemoFixtures(rawInvitations) : []
    } catch (err) {
      if (isDemo) return getDemoFixtures(rawInvitations)
      throw err
    }
  }, [orgId, isDemo])

  const {
    data: invitationsData,
    isLoading: isInvitationsLoading,
    error: invitationsError,
    reload: reloadInvitations,
  } = useApiQuery(loadInvitations)

  const invitations = invitationsData || []

  // Actions: Member role change
  const handleRoleChange = async (memberId: string, roleName: string) => {
    const roleConfig = DEFAULT_ROLES.find((r) => r.name === roleName)
    const roleKey = roleConfig?.key || roleName.toLowerCase()

    if (!orgId || isDemo) {
      toast.success(`Role updated to ${roleName}`)
      return
    }

    try {
      await updateMemberRoles(orgId, memberId, [roleKey])
      toast.success(`Role updated to ${roleName}`)
      await reloadMembers()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update member role')
      throw err
    }
  }

  // Actions: Member remove
  const handleRemoveMember = async (memberId: string) => {
    if (!orgId || isDemo) {
      toast.success('Member removed from organization')
      return
    }

    try {
      await removeMember(orgId, memberId)
      toast.success('Member removed from organization')
      await reloadMembers()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to remove member')
      throw err
    }
  }

  // Actions: Send Invitation
  const handleInvite = async (email: string, roleName: string) => {
    const roleConfig = DEFAULT_ROLES.find((r) => r.name === roleName)
    const roleKey = roleConfig?.key || roleName.toLowerCase()

    if (!orgId || isDemo) {
      toast.success(`Invitation sent to ${email} as ${roleName}`)
      return
    }

    try {
      await createOrganizationInvitation(orgId, email, [roleKey])
      toast.success(`Invitation sent to ${email}`)
      await reloadInvitations()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to send invitation')
      throw err
    }
  }

  // Actions: Revoke Invitation
  const handleRevokeInvitation = async (invitationId: string) => {
    if (!orgId || isDemo) {
      toast.success('Invitation revoked')
      return
    }

    try {
      await revokeOrganizationInvitation(orgId, invitationId)
      toast.success('Invitation revoked')
      await reloadInvitations()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to revoke invitation')
      throw err
    }
  }

  // Actions: Create Team
  const handleCreateTeam = (name: string) => {
    const newTeam: Team = {
      id: `team-${Date.now()}`,
      name,
      memberCount: 0,
      members: [],
    }
    setTeams((prev) => [newTeam, ...prev])
    toast.success(`Team "${name}" created`)
  }

  return (
    <PageContainer density="wide">
      <PageHeader
        title="Access Control"
        description="Manage organization users, teams, default roles, granular capability permissions, and invitations."
      />

      <Tabs defaultValue="users">
        <TabsList variant="line">
          <TabsTrigger value="users">
            Users {members.length > 0 ? `(${members.length})` : ''}
          </TabsTrigger>
          <TabsTrigger value="teams">
            Teams {teams.length > 0 ? `(${teams.length})` : ''}
          </TabsTrigger>
          <TabsTrigger value="roles">Default Roles (6)</TabsTrigger>
          <TabsTrigger value="permissions">Permission Matrix</TabsTrigger>
          <TabsTrigger value="invitations">
            Invitations {invitations.length > 0 ? `(${invitations.length})` : ''}
          </TabsTrigger>
        </TabsList>

        {/* Users Tab */}
        <TabsContent value="users" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Organization Members</CardTitle>
              <CardDescription>
                Assigned default roles, activity status, and team associations.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              {isMembersLoading ? (
                <div className="py-12">
                  <LoadingState label="Loading organization members…" />
                </div>
              ) : membersError && !isDemo ? (
                <div className="p-6">
                  <ErrorState
                    title="Failed to load members"
                    message={membersError}
                    onRetry={reloadMembers}
                  />
                </div>
              ) : (
                <TeamMembersTable
                  members={members}
                  onRoleChange={handleRoleChange}
                  onRemoveMember={handleRemoveMember}
                />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Teams Tab */}
        <TabsContent value="teams" className="mt-4">
          <TeamsGrid teams={teams} onCreateTeam={handleCreateTeam} />
        </TabsContent>

        {/* Roles Tab */}
        <TabsContent value="roles" className="mt-4">
          <RolesList />
        </TabsContent>

        {/* Permissions Tab */}
        <TabsContent value="permissions" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Permission Matrix by Capability</CardTitle>
              <CardDescription>
                Comprehensive capability grants across the 6 default roles. Switch views to inspect by fine-grained capability or resource level.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <PermissionMatrix
                roles={roleNames}
                resources={permissionResources}
                matrix={permissionMatrix}
              />
            </CardContent>
          </Card>
        </TabsContent>

        {/* Invitations Tab */}
        <TabsContent value="invitations" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle>Invitations</CardTitle>
              <CardDescription>
                Pending and historical organization invites. Revoke open links anytime.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              {isInvitationsLoading ? (
                <div className="py-12">
                  <LoadingState label="Loading invitations…" />
                </div>
              ) : invitationsError && !isDemo ? (
                <div className="p-6">
                  <ErrorState
                    title="Failed to load invitations"
                    message={invitationsError}
                    onRetry={reloadInvitations}
                  />
                </div>
              ) : (
                <InvitationsTable
                  invitations={invitations}
                  onInvite={handleInvite}
                  onRevoke={handleRevokeInvitation}
                />
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </PageContainer>
  )
}
