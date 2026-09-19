import Link from 'next/link'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { DetailList } from '@/components/platform/detail-list'
import { StatusBadge } from '@/components/platform/status-badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { DatabaseInstance, DomainRecord, SecretItem, Volume } from '@/lib/types'
import type { DockerNetwork } from '@/lib/types'

interface ProjectResourcesPanelProps {
  databases: DatabaseInstance[]
  volumes: Volume[]
  networks: DockerNetwork[]
  domains: DomainRecord[]
  secrets: SecretItem[]
}

export function ProjectResourcesPanel({
  databases,
  volumes,
  networks,
  domains,
  secrets,
}: ProjectResourcesPanelProps) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card size="sm">
        <CardHeader>
          <CardTitle>Databases</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {databases.length === 0 ? (
            <p className="px-4 pb-4 text-sm text-muted-foreground">No databases linked.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Instance</TableHead>
                  <TableHead>Env</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {databases.map((db) => (
                  <TableRow key={db.id}>
                    <TableCell>
                      <Link href={`/databases/${db.id}`} className="hover:underline">
                        {db.name}
                      </Link>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{db.environment}</TableCell>
                    <TableCell>
                      <StatusBadge status={db.status} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Domains</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {domains.length === 0 ? (
            <p className="px-4 pb-4 text-sm text-muted-foreground">No domains linked.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Domain</TableHead>
                  <TableHead>App</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {domains.map((domain) => (
                  <TableRow key={domain.id}>
                    <TableCell className="font-mono text-xs">{domain.domain}</TableCell>
                    <TableCell className="text-muted-foreground">{domain.application}</TableCell>
                    <TableCell>
                      <StatusBadge status={domain.status} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Secrets metadata</CardTitle>
        </CardHeader>
        <CardContent>
          {secrets.length === 0 ? (
            <p className="text-sm text-muted-foreground">No secrets referenced.</p>
          ) : (
            <DetailList
              items={secrets.map((secret) => ({
                label: secret.name,
                value: `${secret.scope} · updated ${secret.updatedAt}`,
              }))}
            />
          )}
        </CardContent>
      </Card>

      <Card size="sm">
        <CardHeader>
          <CardTitle>Volumes & networks</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <DetailList
            items={[
              {
                label: 'Volumes',
                value: volumes.length === 0 ? 'None' : volumes.map((v) => v.name).join(', '),
              },
              {
                label: 'Networks',
                value: networks.length === 0 ? 'None' : networks.map((n) => n.name).join(', '),
              },
            ]}
          />
        </CardContent>
      </Card>
    </div>
  )
}
