import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { DEFAULT_ROLES } from '@/lib/rbac'

export function RolesList() {
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {DEFAULT_ROLES.map((role) => (
        <Card key={role.name} size="sm">
          <CardHeader>
            <div className="flex items-center justify-between gap-2">
              <CardTitle className="text-base">{role.name}</CardTitle>
              <Badge variant="secondary" className="text-[10px]">
                Default
              </Badge>
            </div>
            <CardDescription>{role.description}</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-xs text-muted-foreground">
              Capability grants are defined in the Permissions matrix.
            </p>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
