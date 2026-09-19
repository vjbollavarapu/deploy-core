import type { ReactNode } from 'react'
import { AdminNav } from '@/components/platform/admin-nav'

export default function AdminLayout({ children }: { children: ReactNode }) {
  return <AdminNav>{children}</AdminNav>
}
