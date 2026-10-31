const WIDTH = 300
const HEIGHT = 40
const PAD = 5

export interface SparkPoint {
  /** Stable key (the week's start date). */
  key: string
  /** The point's name in text, e.g. "Week of 26 Oct". */
  label: string
  /** null = no data that week; the line breaks there. */
  value: number | null
  /** The value as text ("50 %", "$0.04"); ignored when `value` is null. */
  text: string
}

/**
 * A one-series trend line in hand-written SVG. The title names the series,
 * the latest value sits beside it as text, and the SVG's accessible name
 * reads every week, so nothing depends on seeing the line.
 */
export function Sparkline({
  title,
  points,
  latest,
  emptyText,
  note,
  testId,
}: {
  title: string
  points: SparkPoint[]
  /** The most recent value as text, shown large. */
  latest: string
  /** Read for a week without data. */
  emptyText: string
  /** A line under the plot (e.g. which runs the cost leaves out). */
  note?: string
  testId?: string
}) {
  const values = points.flatMap((p) => (p.value === null ? [] : [p.value]))
  const max = Math.max(...values, 0)
  const min = Math.min(...values, 0)
  const span = max - min || 1
  const step = points.length > 1 ? (WIDTH - 2 * PAD) / (points.length - 1) : 0
  const x = (i: number) => (points.length > 1 ? PAD + i * step : WIDTH / 2)
  const y = (v: number) =>
    HEIGHT - PAD - ((v - min) / span) * (HEIGHT - 2 * PAD)

  // Consecutive weeks with data form one segment; a gap breaks the line.
  const segments: string[] = []
  let current: string[] = []
  points.forEach((p, i) => {
    if (p.value === null) {
      if (current.length > 0) segments.push(current.join(" "))
      current = []
      return
    }
    current.push(`${x(i).toFixed(1)},${y(p.value).toFixed(1)}`)
  })
  if (current.length > 0) segments.push(current.join(" "))

  const description = points
    .map((p) => `${p.label}: ${p.value === null ? emptyText : p.text}`)
    .join("; ")

  return (
    <figure
      className="flex flex-col gap-2 rounded-cell border border-line bg-panel p-4"
      data-testid={testId}
    >
      <figcaption className="flex items-baseline justify-between gap-3">
        <span className="text-small text-ink-muted">{title}</span>
        <span className="text-h3 font-semibold text-ink tabular-nums">
          {latest}
        </span>
      </figcaption>
      <svg
        role="img"
        aria-label={`${title}. ${description}`}
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        className="h-10 w-full overflow-visible"
      >
        <line
          x1={0}
          x2={WIDTH}
          y1={HEIGHT - PAD}
          y2={HEIGHT - PAD}
          className="stroke-line"
          strokeWidth={1}
        />
        {segments.map((pts) => (
          <polyline
            key={pts}
            points={pts}
            fill="none"
            className="stroke-accent"
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        ))}
        {points.map((p, i) =>
          p.value === null ? null : (
            <circle
              key={p.key}
              cx={x(i)}
              cy={y(p.value)}
              r={i === points.length - 1 ? 4 : 2.5}
              className="fill-accent stroke-panel"
              strokeWidth={2}
            >
              <title>{`${p.label}: ${p.text}`}</title>
            </circle>
          )
        )}
      </svg>
      {note && <p className="text-caption text-ink-muted">{note}</p>}
    </figure>
  )
}
