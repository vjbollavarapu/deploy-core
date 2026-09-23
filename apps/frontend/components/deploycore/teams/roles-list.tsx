import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ShieldCheck, ShieldAlert, Cpu, Terminal, LifeBuoy, Eye } from 'lucide-react'
import { DEFAULT_ROLES } from '@/lib/rbac'

const roleIcons: Record<string, React.ElementType> = {
  Owner: ShieldAlert,
  Administrator: ShieldCheck,
  DevOps: Cpu,
  Developer: Terminal,
  Support: LifeBuoy,
  Viewer: Eye,
}

export function RolesList() {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {DEFAULT_ROLES.map((role) => {
        const Icon = roleIcons[role.name] || ShieldCheck
        return (
          <Card key={role.name} size="sm" className="flex flex-col justify-between">
            <CardHeader>
              <div className="flex items-start justify-between gap-2">
                <div className="flex items-center gap-2">
                  <div className="flex size-8 items-center justify-center rounded-md bg-secondary text-foreground">
                    <Icon className="size-4" />
                  </div>
                  <div>
                    <CardTitle className="text-base">{role.name}</CardTitle>
                    <span className="text-[11px] font-medium text-muted-foreground">{role.tier}</span>
                  </div>
                </div>
                <Badge variant="secondary" className="text-[10px]">
                  Default
                </Badge>
              </div>
              <CardDescription className="pt-2 text-xs leading-relaxed">
                {role.description}
              </CardDescription>
            </CardHeader>
            <CardContent className="pt-0">
              <div className="flex items-center justify-between border-t border-border/60 pt-3 text-xs text-muted-foreground">
                <span>Capabilities</span>
                <span className="font-mono text-xs font-semibold text-foreground">
                  {role.capabilityCount} / 45 granted
                </span>
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
