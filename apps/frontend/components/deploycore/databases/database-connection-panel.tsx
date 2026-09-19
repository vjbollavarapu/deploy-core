'use client'

import { useState } from 'react'
import { Eye, EyeOff, ShieldAlert, ShieldCheck } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { CopyButton } from '@/components/platform/copy-button'
import { SecretField } from '@/components/platform/secret-field'
import { buildConnectionUrl } from '@/lib/databases'
import type { DatabaseInstance } from '@/lib/types'

const MOCK_PASSWORD = 'dc_mock_p4ssw0rd'

interface DatabaseConnectionPanelProps {
  database: DatabaseInstance
}

export function DatabaseConnectionPanel({ database }: DatabaseConnectionPanelProps) {
  const [permissionGranted, setPermissionGranted] = useState(false)
  const [revealed, setRevealed] = useState(false)

  const policyAllows = database.credentialsRevealAllowed
  const canReveal = policyAllows && permissionGranted
  const password = canReveal && revealed ? MOCK_PASSWORD : '••••••••'
  const url = buildConnectionUrl(database, password)

  return (
    <div className="flex flex-col gap-4">
      {!policyAllows ? (
        <Alert variant="destructive">
          <ShieldAlert />
          <AlertTitle>Credentials locked by API policy</AlertTitle>
          <AlertDescription>
            This instance does not allow credential reveal. Connection secrets stay masked even if
            you hold operator permission.
          </AlertDescription>
        </Alert>
      ) : (
        <Alert>
          <ShieldCheck />
          <AlertTitle>Reveal requires explicit permission</AlertTitle>
          <AlertDescription>
            Confirm you are authorised to view live credentials for {database.name}. Secrets are
            only shown after policy and permission both allow it.
          </AlertDescription>
        </Alert>
      )}

      <Card size="sm">
        <CardHeader>
          <CardTitle>Connection</CardTitle>
          <CardDescription>
            Endpoint metadata is always visible. Password and full URL reveal only when allowed.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <MetaRow label="Engine" value={`${database.type} ${database.version}`} />
          <MetaRow label="Host" value={database.connectionHost} copyable />
          <MetaRow label="Port" value={String(database.port)} copyable />
          <MetaRow label="Database" value={database.dbName} copyable />
          <MetaRow label="Username" value={database.username} copyable />

          <div className="flex flex-col gap-2">
            <span className="text-xs text-muted-foreground">Password</span>
            {canReveal ? (
              <SecretField value={MOCK_PASSWORD} />
            ) : (
              <SecretField value={MOCK_PASSWORD} neverReveal />
            )}
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-muted-foreground">Connection URL</span>
              {canReveal ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 gap-1.5 px-2 text-xs"
                  onClick={() => setRevealed((v) => !v)}
                >
                  {revealed ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                  {revealed ? 'Hide password' : 'Show password'}
                </Button>
              ) : null}
            </div>
            <div className="flex items-center gap-1 rounded-md border border-border bg-secondary px-2.5 py-1.5">
              <span className="flex-1 truncate font-mono text-xs text-foreground">{url}</span>
              {canReveal && revealed ? <CopyButton value={buildConnectionUrl(database, MOCK_PASSWORD)} /> : null}
            </div>
          </div>

          {policyAllows ? (
            <div className="flex items-start gap-2 rounded-md border border-border p-3">
              <Checkbox
                id="cred-permission"
                checked={permissionGranted}
                onCheckedChange={(checked) => {
                  const next = checked === true
                  setPermissionGranted(next)
                  if (!next) setRevealed(false)
                }}
              />
              <Label htmlFor="cred-permission" className="text-sm font-normal leading-snug">
                I have permission to view connection credentials for this database.
              </Label>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}

function MetaRow({
  label,
  value,
  copyable,
}: {
  label: string
  value: string
  copyable?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <div className="flex items-center gap-1.5">
        <span className="font-mono text-xs text-foreground">{value}</span>
        {copyable ? <CopyButton value={value} /> : null}
      </div>
    </div>
  )
}
