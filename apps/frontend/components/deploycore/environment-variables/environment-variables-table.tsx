import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { SecretField } from '@/components/platform/secret-field'
import type { EnvVarEntry } from '@/lib/types'

interface EnvironmentVariablesTableProps {
  variables: EnvVarEntry[]
}

export function EnvironmentVariablesTable({ variables }: EnvironmentVariablesTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Key</TableHead>
          <TableHead>Scope</TableHead>
          <TableHead>Source</TableHead>
          <TableHead>Value</TableHead>
          <TableHead>Overridden</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {variables.map((v) => (
          <TableRow key={v.id}>
            <TableCell>
              <div className="flex items-center gap-1.5">
                <span className="font-mono text-xs font-medium text-foreground">{v.key}</span>
                {v.secret && (
                  <Badge variant="outline" className="text-[10px]">
                    Secret
                  </Badge>
                )}
              </div>
            </TableCell>
            <TableCell>
              <Badge variant="secondary" className="text-[10px]">
                {v.scope}
              </Badge>
            </TableCell>
            <TableCell className="text-foreground">{v.source}</TableCell>
            <TableCell>
              {v.secret ? (
                <SecretField value={v.value} className="w-56" />
              ) : (
                <span className="font-mono text-xs text-muted-foreground">{v.value}</span>
              )}
            </TableCell>
            <TableCell className="text-sm text-muted-foreground">{v.overridden ? 'Yes' : 'No'}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
