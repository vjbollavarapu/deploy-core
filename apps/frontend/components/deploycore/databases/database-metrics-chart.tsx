'use client'

import { Area, AreaChart } from 'recharts'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'

const chartConfig = {
  cpu: { label: 'CPU', color: 'var(--chart-1)' },
  connections: { label: 'Connections', color: 'var(--chart-2)' },
  storage: { label: 'Storage %', color: 'var(--chart-3)' },
} satisfies ChartConfig

interface DatabaseMetricsChartProps {
  series: { t: string; cpu: number; connections: number; storage: number }[]
  heightClassName?: string
}

export function DatabaseMetricsChart({
  series,
  heightClassName = 'h-64',
}: DatabaseMetricsChartProps) {
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
          dataKey="connections"
          stroke="var(--color-connections)"
          fill="var(--color-connections)"
          fillOpacity={0.1}
          strokeWidth={1.5}
          isAnimationActive={false}
        />
        <Area
          type="monotone"
          dataKey="storage"
          stroke="var(--color-storage)"
          fill="var(--color-storage)"
          fillOpacity={0.08}
          strokeWidth={1.5}
          isAnimationActive={false}
        />
      </AreaChart>
    </ChartContainer>
  )
}
