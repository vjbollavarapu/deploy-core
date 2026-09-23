'use client'

import { useState } from 'react'
import { Plus, Users } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { EmptyState } from '@/components/platform/empty-state'
import type { Team } from '@/lib/types'

interface TeamsGridProps {
  teams: Team[]
  onCreateTeam?: (name: string, description?: string) => void
}

export function TeamsGrid({ teams, onCreateTeam }: TeamsGridProps) {
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  const handleCreate = () => {
    if (!name.trim()) return
    onCreateTeam?.(name.trim(), description.trim() || undefined)
    setName('')
    setDescription('')
    setCreateOpen(false)
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">Functional Teams</h3>
          <p className="text-xs text-muted-foreground">
            Group members to coordinate project scopes, notifications, and resource ownership.
          </p>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)} className="gap-1.5">
          <Plus className="size-4" />
          Create Team
        </Button>
      </div>

      {teams.length === 0 ? (
        <Card>
          <CardContent className="p-8">
            <EmptyState
              icon={Users}
              title="No functional teams"
              description="Create teams (e.g., Infrastructure, Core Platform, Data, Frontend) to segment projects and deployments."
              action={
                <Button size="sm" onClick={() => setCreateOpen(true)} className="gap-1.5">
                  <Plus className="size-4" />
                  Create Team
                </Button>
              }
            />
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {teams.map((team) => (
            <Card key={team.id}>
              <CardHeader className="flex-row items-center justify-between pb-2">
                <div className="flex flex-col gap-0.5">
                  <CardTitle className="text-base">{team.name}</CardTitle>
                  <CardDescription className="text-xs">
                    {team.members.length} active member{team.members.length === 1 ? '' : 's'}
                  </CardDescription>
                </div>
                <Badge variant="secondary" className="gap-1 text-[10px]">
                  <Users data-icon="inline-start" className="size-3" />
                  {team.memberCount}
                </Badge>
              </CardHeader>
              <CardContent>
                <div className="flex flex-col gap-2">
                  <span className="text-xs font-medium text-muted-foreground">Members</span>
                  <div className="flex flex-wrap gap-1.5">
                    {team.members.map((m) => (
                      <Badge key={m} variant="outline" className="text-[10px]">
                        {m}
                      </Badge>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Team</DialogTitle>
            <DialogDescription>
              Create a functional team to organize members and project access.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3 py-2">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="team-name">Team Name</Label>
              <Input
                id="team-name"
                placeholder="e.g. SRE Platform, Core API"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="team-desc">Description (optional)</Label>
              <Input
                id="team-desc"
                placeholder="Team mission or scope"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button disabled={!name.trim()} onClick={handleCreate}>
              Create Team
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
