type VitalTraceProps = {
  path: string
  variant?: 'stable' | 'alert'
  viewBox?: string
  duration?: string
  className?: string
}

/** An oscilloscope-style sparkline: a dim static trace with a bright sweep loop on top. */
export function VitalTrace({
  path,
  variant = 'stable',
  viewBox = '0 0 220 40',
  duration = '4s',
  className = '',
}: VitalTraceProps) {
  const traceColor = variant === 'alert' ? 'stroke-alert' : 'stroke-trace'

  return (
    <svg
      viewBox={viewBox}
      preserveAspectRatio="none"
      className={`h-10 w-full overflow-visible ${className}`}
      aria-hidden="true"
    >
      <path d={path} className="fill-none stroke-ink-dim/55" strokeWidth={1.5} />
      <path
        d={path}
        className={`fill-none ${traceColor}`}
        strokeWidth={1.75}
        strokeLinecap="round"
        style={{
          strokeDasharray: '36 900',
          animation: `trace-sweep ${duration} linear infinite`,
        }}
      />
    </svg>
  )
}
