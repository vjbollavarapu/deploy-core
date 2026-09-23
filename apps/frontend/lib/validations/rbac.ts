import { z } from 'zod'

export const DEFAULT_ROLE_KEYS = [
  'owner',
  'administrator',
  'devops',
  'developer',
  'support',
  'viewer',
] as const

export const DEFAULT_ROLE_NAMES = [
  'Owner',
  'Administrator',
  'DevOps',
  'Developer',
  'Support',
  'Viewer',
] as const

export const inviteMemberSchema = z.object({
  email: z
    .string()
    .min(1, 'Email is required')
    .email('Please enter a valid email address')
    .trim(),
  role: z.enum(DEFAULT_ROLE_NAMES),
})

export type InviteMemberValues = z.infer<typeof inviteMemberSchema>

export const updateMemberRoleSchema = z.object({
  role: z.enum(DEFAULT_ROLE_NAMES),
})

export type UpdateMemberRoleValues = z.infer<typeof updateMemberRoleSchema>

export const createTeamSchema = z.object({
  name: z
    .string()
    .min(2, 'Team name must be at least 2 characters')
    .max(50, 'Team name must be under 50 characters')
    .trim(),
  description: z
    .string()
    .max(200, 'Description must be under 200 characters')
    .optional(),
  lead: z.string().trim().optional(),
})

export type CreateTeamValues = z.infer<typeof createTeamSchema>
