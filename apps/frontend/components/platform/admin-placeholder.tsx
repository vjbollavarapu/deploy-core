import type { LucideIcon } from 'lucide-react'
import { EmptyState } from '@/components/platform/empty-state'

interface AdminPlaceholderProps {
  icon: LucideIcon
  title: string
  description: string
}

export function AdminPlaceholder({ icon, title, description }: AdminPlaceholderProps) {
  return <EmptyState icon={icon} title={title} description={description} />
}
