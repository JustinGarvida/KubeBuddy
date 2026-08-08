import { VitalTrace } from './VitalTrace'

type MonitorCardProps = {
  namespace: string
  pod: string
  metric: string
  value: string
  status: 'stable' | 'alert'
  path: string
  duration?: string
}

export function MonitorCard({
  namespace,
  pod,
  metric,
  value,
  status,
  path,
  duration,
}: MonitorCardProps) {
  const isAlert = status === 'alert'

  return (
    <div
      className={`rounded-lg border p-4 transition-colors ${
        isAlert ? 'border-alert/40 bg-alert/5' : 'border-grid bg-panel'
      }`}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate font-mono text-[11px] tracking-widest text-ink-faint uppercase">
            {namespace}
          </p>
          <p className="truncate font-mono text-sm text-ink">{pod}</p>
        </div>
        <span
          className={`h-2 w-2 shrink-0 rounded-full ${
            isAlert ? 'animate-pulse bg-alert' : 'bg-stable'
          }`}
        />
      </div>

      <VitalTrace path={path} variant={status} duration={duration} className="mt-3" />

      <div className="mt-2 flex items-baseline justify-between">
        <span className="font-mono text-xs text-ink-dim">{metric}</span>
        <span className={`font-mono text-sm ${isAlert ? 'text-alert' : 'text-ink'}`}>
          {value}
        </span>
      </div>
      {isAlert && (
        <p className="mt-1 font-mono text-[10px] tracking-widest text-alert uppercase">
          deviation flagged
        </p>
      )}
    </div>
  )
}
