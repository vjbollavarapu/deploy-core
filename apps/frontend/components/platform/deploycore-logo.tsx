'use client'

import Image from 'next/image'
import { cn } from '@/lib/utils'

interface DeployCoreLogoProps {
  /** Outer box size in pixels (logo is centered, aspect preserved). */
  size?: number
  className?: string
  priority?: boolean
}

/**
 * Official DeployCore brand mark. Uses /deploycore-logo.png without altering artwork.
 */
export function DeployCoreLogo({ size = 28, className, priority = false }: DeployCoreLogoProps) {
  return (
    <span
      className={cn('relative inline-flex shrink-0 items-center justify-center overflow-hidden', className)}
      style={{ width: size, height: size }}
    >
      <Image
        src="/deploycore-logo.png"
        alt="DeployCore"
        width={size}
        height={size}
        className="object-contain"
        priority={priority}
      />
    </span>
  )
}
