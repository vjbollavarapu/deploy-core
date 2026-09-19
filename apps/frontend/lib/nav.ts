import {
  Activity,
  Archive,
  BarChart3,
  Bell,
  Box,
  Boxes,
  Database,
  FileClock,
  GitBranch,
  GitCommitVertical,
  Globe,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LayoutGrid,
  Network,
  Package,
  ScrollText,
  Server,
  Settings,
  Shapes,
  ShieldCheck,
  Users,
  Variable,
  Webhook,
} from 'lucide-react'

export interface NavItem {
  title: string
  url: string
  icon: typeof LayoutDashboard
}

export interface NavGroup {
  title: string
  items: NavItem[]
}

/** Primary organization navigation. Super Admin is excluded — rendered separately. */
export const navGroups: NavGroup[] = [
  {
    title: 'Overview',
    items: [
      { title: 'Overview', url: '/dashboard', icon: LayoutDashboard },
      { title: 'Projects', url: '/projects', icon: Boxes },
    ],
  },
  {
    title: 'Applications',
    items: [
      { title: 'Applications', url: '/applications', icon: LayoutGrid },
      { title: 'Deployments', url: '/deployments', icon: GitBranch },
      { title: 'Revisions', url: '/revisions', icon: GitCommitVertical },
      { title: 'Domains', url: '/domains', icon: Globe },
    ],
  },
  {
    title: 'Infrastructure',
    items: [
      { title: 'Servers', url: '/servers', icon: Server },
      { title: 'Containers', url: '/containers', icon: Box },
      { title: 'Images', url: '/images', icon: Package },
      { title: 'Databases', url: '/databases', icon: Database },
      { title: 'Backups', url: '/backups', icon: Archive },
      { title: 'Volumes', url: '/volumes', icon: HardDrive },
      { title: 'Networks', url: '/networks', icon: Network },
    ],
  },
  {
    title: 'Observability',
    items: [
      { title: 'Logs', url: '/logs', icon: ScrollText },
      { title: 'Metrics', url: '/metrics', icon: BarChart3 },
      { title: 'Events', url: '/events', icon: Activity },
    ],
  },
  {
    title: 'Integrations',
    items: [
      { title: 'Git Providers', url: '/integrations/git', icon: GitBranch },
      { title: 'Registries', url: '/integrations/registries', icon: Shapes },
      { title: 'Notifications', url: '/integrations/notifications', icon: Bell },
      { title: 'Webhooks', url: '/integrations/webhooks', icon: Webhook },
    ],
  },
  {
    title: 'Security',
    items: [
      { title: 'Environment Variables', url: '/environment-variables', icon: Variable },
      { title: 'Secrets', url: '/security/secrets', icon: KeyRound },
      { title: 'Access Control', url: '/security/access', icon: Users },
      { title: 'Audit Logs', url: '/security/audit', icon: FileClock },
    ],
  },
  {
    title: 'Settings',
    items: [{ title: 'Settings', url: '/settings', icon: Settings }],
  },
]

/** Single entry into the separately authorised admin area. Detail nav lives in /admin layout. */
export const adminNavEntry: NavItem = {
  title: 'Super Admin',
  url: '/admin/organizations',
  icon: ShieldCheck,
}

export const adminNavItems: NavItem[] = [
  { title: 'Organizations', url: '/admin/organizations', icon: Boxes },
  { title: 'Users', url: '/admin/users', icon: Users },
  { title: 'Servers', url: '/admin/servers', icon: Server },
  { title: 'Agents', url: '/admin/agents', icon: Activity },
  { title: 'Health', url: '/admin/health', icon: ShieldCheck },
  { title: 'Jobs', url: '/admin/jobs', icon: Activity },
  { title: 'Features', url: '/admin/features', icon: Settings },
  { title: 'Versions', url: '/admin/versions', icon: LayoutGrid },
  { title: 'Audit', url: '/admin/audit', icon: FileClock },
]

export const allNavItems: NavItem[] = [
  ...navGroups.flatMap((group) => group.items),
  adminNavEntry,
  ...adminNavItems,
]
