'use client'

import { Analytics } from '@vercel/analytics/next'
import { sanitizeAnalyticsEvent } from '@/lib/github/callback'

export function DeployCoreAnalytics() {
  if (process.env.NODE_ENV !== 'production') return null
  return <Analytics beforeSend={sanitizeAnalyticsEvent} />
}
