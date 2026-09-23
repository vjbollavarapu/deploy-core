'use client'

import { useState } from 'react'
import { Eye, EyeOff, KeyRound, ShieldAlert, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { CopyButton } from '@/components/platform/copy-button'
import { SecretField } from '@/components/platform/secret-field'
import { apiClient, ApiError } from '@/lib/api'
import { buildConnectionUrl } from '@/lib/databases'
import { isDemoModeEnabled } from '@/lib/mock-isolation'
import type { DatabaseInstance } from '@/lib/types'

interface DatabaseConnectionPanelProps {
  database: DatabaseInstance
}

type RevealResponse = {
  databaseId?: string
  username?: string
  databaseName?: string
  password?: string
}

export function DatabaseConnectionPanel({ database }: DatabaseConnectionPanelProps) {
  const [permissionGranted, setPermissionGranted] = useState(false)
  const [revealed, setRevealed] = useState(false)
  const [revealing, setRevealing] = useState(false)
  const [revealedPassword, setRevealedPassword] = useState<string | null>(null)

  const policyAllows = database.credentialsRevealAllowed
  const canReveal = policyAllows && permissionGranted

  function clearRevealed() {
    setRevealed(false)
    setRevealedPassword(null)
  }

  async function handleRevealToggle() {
    if (revealed) {
      clearRevealed()
      return
    }

    if (!canReveal) {
      toast.error('Explicit permission confirmation is required to reveal credentials.')
      return
    }

    if (isDemoModeEnabled()) {
      toast.message('Demo mode: live credential reveal requires the Control Plane.')
      return
    }

    setRevealing(true)
    try {
      const res = await apiClient.post<RevealResponse>(
        `/databases/${database.id}/credentials/reveal`,
      )
      const password = typeof res.password === 'string' ? res.password : ''
      if (!password) {
        toast.error('Control Plane did not return a credential.')
        return
      }
      setRevealedPassword(password)
      setRevealed(true)
      toast.success('Connection credentials revealed — access recorded in audit trail')
    } catch (err) {
      toast.error(
        err instanceof ApiError ? err.message : 'Failed to reveal credentials from control plane',
      )
    } finally {
      setRevealing(false)
    }
  }

  const activePassword = revealed && revealedPassword ? revealedPassword : '••••••••••••'
  const url = buildConnectionUrl(database, activePassword)

  return (
    <div className="flex flex-col gap-4">
      {!policyAllows ? (
        <Alert variant="destructive">
          <ShieldAlert />
          <AlertTitle>Credentials locked by API policy</AlertTitle>
          <AlertDescription>
            This instance is configured with policy lock. Live connection credentials cannot be revealed through the UI or API even if you hold operator credentials.
          </AlertDescription>
        </Alert>
      ) : (
        <Alert>
          <ShieldCheck className="text-primary" />
          <AlertTitle>Credential Access Policy</AlertTitle>
          <AlertDescription>
            Connection credentials for {database.name} are envelope-encrypted. Revealing the password requires explicit permission confirmation and generates an immutable audit record.
          </AlertDescription>
        </Alert>
      )}

      <Card size="sm">
        <CardHeader>
          <CardTitle>Connection details</CardTitle>
          <CardDescription>
            Endpoint metadata is visible for application networking. Credentials are masked until explicitly unlocked.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <MetaRow label="Engine" value={`${database.type} ${database.version}`} />
          <MetaRow label="Host" value={database.connectionHost} copyable />
          <MetaRow label="Port" value={String(database.port)} copyable />
          <MetaRow label="Database name" value={database.dbName} copyable />
          <MetaRow label="Username" value={database.username} copyable />

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-muted-foreground">Password</span>
              {policyAllows && permissionGranted ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={revealing}
                  className="h-7 gap-1.5 px-2 text-xs"
                  onClick={() => void handleRevealToggle()}
                >
                  {revealed ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                  {revealed ? 'Hide password' : revealing ? 'Decrypting…' : 'Reveal password'}
                </Button>
              ) : null}
            </div>

            {canReveal && revealed && revealedPassword ? (
              <SecretField value={revealedPassword} />
            ) : (
              <SecretField value="••••••••••••" neverReveal />
            )}
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-muted-foreground">Connection string URI</span>
              {canReveal && revealed ? (
                <span className="text-[11px] text-muted-foreground">Unmasked</span>
              ) : (
                <span className="text-[11px] text-muted-foreground">Password masked</span>
              )}
            </div>
            <div className="flex items-center gap-1 rounded-md border border-border bg-secondary px-2.5 py-1.5">
              <span className="flex-1 truncate font-mono text-xs text-foreground">{url}</span>
              <CopyButton value={url} />
            </div>
          </div>

          {policyAllows ? (
            <div className="flex items-start gap-2.5 rounded-lg border border-border p-3.5 bg-muted/20">
              <Checkbox
                id="cred-permission"
                checked={permissionGranted}
                onCheckedChange={(checked) => {
                  const next = checked === true
                  setPermissionGranted(next)
                  if (!next) {
                    clearRevealed()
                  }
                }}
              />
              <div className="flex flex-col gap-0.5">
                <Label htmlFor="cred-permission" className="text-sm font-medium leading-snug cursor-pointer">
                  I confirm authorization to view live credentials for {database.name}.
                </Label>
                <p className="text-xs text-muted-foreground">
                  Access will be audited under your identity and recorded in the tenant security logs.
                </p>
                {permissionGranted && !revealed ? (
                  <Button
                    size="sm"
                    className="mt-2 w-fit"
                    disabled={revealing}
                    onClick={() => void handleRevealToggle()}
                  >
                    <KeyRound data-icon="inline-start" />
                    {revealing ? 'Decrypting via KMS…' : 'Reveal live credentials'}
                  </Button>
                ) : null}
              </div>
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
    <div className="flex items-center justify-between gap-2 border-b border-border/50 pb-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <div className="flex items-center gap-1.5">
        <span className="font-mono text-xs text-foreground">{value}</span>
        {copyable ? <CopyButton value={value} /> : null}
      </div>
    </div>
  )
}
