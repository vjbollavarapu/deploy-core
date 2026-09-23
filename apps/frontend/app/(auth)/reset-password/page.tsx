import { Suspense } from 'react'
import { ResetPasswordForm } from '@/components/deploycore/auth/auth-forms'

export default function ResetPasswordPage() {
  return (
    <Suspense fallback={<div className="text-sm text-muted-foreground">Loading…</div>}>
      <ResetPasswordForm />
    </Suspense>
  )
}
