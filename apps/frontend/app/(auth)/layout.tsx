import type { ReactNode } from 'react'
import { DeployCoreLogo } from '@/components/platform/deploycore-logo'

export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-8 bg-background px-4 py-10">
      <div className="flex flex-col items-center gap-3 text-center">
        <DeployCoreLogo size={40} priority className="rounded-md" />
        <div className="flex flex-col gap-1">
          <h1 className="text-lg font-semibold tracking-tight">DeployCore</h1>
          <p className="text-sm text-muted-foreground">Infrastructure control plane</p>
        </div>
      </div>
      {children}
    </div>
  )
}
