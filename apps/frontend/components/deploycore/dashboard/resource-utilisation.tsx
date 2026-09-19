'use client'

import { Area, AreaChart } from 'recharts'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import { Metric } from '@/components/platform/metric'
import { cn } from '@/lib/utils'
import { getResourceUtilisation } from '@/lib/dashboard'

const chartConfig = {
  value: { label: 'Utilisation', color: 'var(--color-primary)' },
} satisfies ChartConfig

function toneClass(value: number) {
  if (value >= 90) return 'text-critical'
  if (value >= 75) return 'text-warning'
  return 'text-foreground'
}

function UtilisationTile({
  label,
  current,
  series,
}: {
  label: string
  current: number
  series: { t: string; value: number }[]
}) {
  return (
    <Card size="sm" className="min-w-0">
      <CardHeader className="pb-0">
        <div className="flex items-start justify-between gap-2">
          <CardTitle>{label}</CardTitle>
          <Metric
            label="Fleet avg"
            value={`${current}%`}
            align="end"
            valueClassName={cn('text-lg font-semibold', toneClass(current))}
          />
        </div>
      </CardHeader>
      <CardContent className="pt-2">
        <ChartContainer
          config={chartConfig}
          className="aspect-auto h-16 w-full"
          initialDimension={{ width: 240, height: 64 }}
        >
          <AreaChart data={series} margin={{ top: 4, right: 0, left: 0, bottom: 0 }}>
            <Area
              type="monotone"
              dataKey="value"
              stroke="var(--color-value)"
              fill="var(--color-value)"
              fillOpacity={0.15}
              strokeWidth={1.5}
              isAnimationActive={false}
              dot={false}
            />
          </AreaChart>
        </ChartContainer>
        <p className="mt-1 text-[11px] text-muted-foreground">Last 12 hours</p>
      </CardContent>
    </Card>
  )
}

export function ResourceUtilisation() {
  const utilisation = getResourceUtilisation()

  return (
    <section aria-labelledby="resource-util-heading" className="flex flex-col gap-3">
      <h2 id="resource-util-heading" className="text-sm font-medium text-foreground">
        Resource Utilisation
      </h2>
      <div className="grid gap-2 sm:grid-cols-3">
        <UtilisationTile label="CPU" current={utilisation.cpu.current} series={utilisation.cpu.series} />
        <UtilisationTile label="RAM" current={utilisation.ram.current} series={utilisation.ram.series} />
        <UtilisationTile
          label="Storage"
          current={utilisation.storage.current}
          series={utilisation.storage.series}
        />
      </div>
    </section>
  )
}
