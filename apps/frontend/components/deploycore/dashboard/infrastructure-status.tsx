import {
  AlertTriangle,
  Database,
  GitBranch,
  Radio,
  Rocket,
  Server,
} from 'lucide-react'
import { MetricCard } from '@/components/platform/metric-card'
import { getInfrastructureStatus } from '@/lib/dashboard'

export function InfrastructureStatus() {
  const status = getInfrastructureStatus()

  return (
    <section aria-labelledby="infra-status-heading" className="flex flex-col gap-3">
      <h2 id="infra-status-heading" className="text-sm font-medium text-foreground">
        Infrastructure Status
      </h2>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6">
        <MetricCard
          label="Servers Online"
          value={String(status.serversOnline.value)}
          detail={`of ${status.serversOnline.total}`}
          icon={Server}
          tone={status.serversOnline.value < status.serversOnline.total ? 'warning' : 'default'}
        />
        <MetricCard
          label="Applications Running"
          value={String(status.applicationsRunning.value)}
          detail={`of ${status.applicationsRunning.total}`}
          icon={Rocket}
        />
        <MetricCard
          label="Active Deployments"
          value={String(status.activeDeployments.value)}
          icon={GitBranch}
        />
        <MetricCard
          label="Databases Healthy"
          value={String(status.databasesHealthy.value)}
          detail={`of ${status.databasesHealthy.total}`}
          icon={Database}
          tone={
            status.databasesHealthy.value < status.databasesHealthy.total ? 'warning' : 'default'
          }
        />
        <MetricCard
          label="Failures"
          value={String(status.failures.value)}
          icon={AlertTriangle}
          tone={status.failures.value > 0 ? 'critical' : 'default'}
        />
        <MetricCard
          label="Agent Connections"
          value={String(status.agentConnections.value)}
          detail={`of ${status.agentConnections.total}`}
          icon={Radio}
          tone={
            status.agentConnections.value < status.agentConnections.total ? 'warning' : 'success'
          }
        />
      </div>
    </section>
  )
}
