import type { LucideIcon } from 'lucide-react'
import { EmptyState } from '@/components/platform/empty-state'

interface ApplicationSectionPlaceholderProps {
  icon: LucideIcon
  title: string
  description: string
}

export function ApplicationSectionPlaceholder({
  icon,
  title,
  description,
}: ApplicationSectionPlaceholderProps) {
  return <EmptyState icon={icon} title={title} description={description} />
}
