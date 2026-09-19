'use client'

import { Area, AreaChart } from 'recharts'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'

const chartConfig = {
  cpu: { label: 'CPU', color: 'var(--chart-1)' },
  memory: { label: 'Memory', color: 'var(--chart-2)' },
  disk: { label: 'Disk', color: 'var(--chart-3)' },
} satisfies ChartConfig

interface ServerMetricsChartProps {
  series: { t: string; cpu: number; memory: number; disk?: number }[]
  heightClassName?: string
  showDisk?: boolean
}

export function ServerMetricsChart({
  series,
  heightClassName = 'h-64',
  showDisk = true,
}: ServerMetricsChartProps) {
  return (
    <ChartContainer config={chartConfig} className={`aspect-auto w-full ${heightClassName}`}>
      <AreaChart data={series} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
        <Area
          type="monotone"
          dataKey="cpu"
          stroke="var(--color-cpu)"
          fill="var(--color-cpu)"
          fillOpacity={0.12}
          strokeWidth={1.5}
          isAnimationActive={false}
        />
        <Area
          type="monotone"
          dataKey="memory"
          stroke="var(--color-memory)"
          fill="var(--color-memory)"
          fillOpacity={0.1}
          strokeWidth={1.5}
          isAnimationActive={false}
        />
        {showDisk ? (
          <Area
            type="monotone"
            dataKey="disk"
            stroke="var(--color-disk)"
            fill="var(--color-disk)"
            fillOpacity={0.08}
            strokeWidth={1.5}
            isAnimationActive={false}
          />
        ) : null}
      </AreaChart>
    </ChartContainer>
  )
}
