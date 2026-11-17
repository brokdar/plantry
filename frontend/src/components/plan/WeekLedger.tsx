import { ArrowDown, ArrowUp, Check, Info } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { SoftIssue, WeekLedger as Week } from "@/lib/api/plan"
import {
  buildLedger,
  type LedgerItem,
  type LedgerStatus,
} from "@/lib/plan/ledger"
import { cn } from "@/lib/utils"

const STATUS_ICON = { ok: Check, low: ArrowDown, high: ArrowUp } as const

function StatusMark({ status, text }: { status: LedgerStatus; text: string }) {
  if (status === "info") return null
  const Icon = STATUS_ICON[status]
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 text-caption",
        status === "ok" ? "text-ink-muted" : "font-semibold text-warning"
      )}
    >
      <Icon className="size-3" aria-hidden="true" />
      {text}
    </span>
  )
}

function Item({ item }: { item: LedgerItem }) {
  return (
    <div
      data-testid={`ledger-${item.key}`}
      data-status={item.status}
      className="flex min-w-0 flex-col"
    >
      <dt className="text-caption text-ink-muted">{item.label}</dt>
      <dd className="flex flex-wrap items-baseline gap-x-2">
        <span
          className={cn(
            "text-body font-semibold tabular-nums",
            item.status === "low" || item.status === "high"
              ? "text-warning"
              : "text-ink"
          )}
        >
          {item.value}
        </span>
        {item.statusText && (
          <StatusMark status={item.status} text={item.statusText} />
        )}
        <span className="text-caption text-ink-muted tabular-nums">
          {item.detail}
        </span>
      </dd>
    </div>
  )
}

/**
 * The week ledger: the window's average nutrition per eater as one quiet
 * strip above the board. Values in range are plain; only what is off gets the
 * warning colour, an arrow and a word (never colour alone). The DGE rules
 * show only while one is missed; hints on single days wait in a closed
 * details element. Soft findings never block anything.
 */
export function WeekLedger({ week, soft }: { week: Week; soft: SoftIssue[] }) {
  const { t, i18n } = useTranslation()
  const ledger = buildLedger(week, soft, i18n)

  return (
    <section
      aria-label={t("plan.ledger.name")}
      data-testid="week-ledger"
      className="flex flex-col gap-2 rounded-xl border border-line bg-panel px-4 py-3 shadow-card"
    >
      <div className="flex flex-wrap items-baseline gap-x-6 gap-y-3">
        <h2 className="text-small font-semibold text-ink">
          {t("plan.ledger.name")}
        </h2>
        <dl className="m-0 grid min-w-0 flex-1 grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-[repeat(auto-fit,minmax(9rem,max-content))]">
          {ledger.items.map((item) => (
            <Item key={item.key} item={item} />
          ))}
        </dl>
      </div>
      {ledger.dge.length > 0 && (
        <ul
          data-testid="ledger-dge"
          className="flex flex-col gap-0.5 text-small text-ink-muted"
        >
          {ledger.dge.map((line) => (
            <li key={line} className="flex items-start gap-1.5">
              <Info className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
              {line}
            </li>
          ))}
        </ul>
      )}
      {ledger.dayHints.length > 0 && (
        <details data-testid="ledger-day-hints" className="text-small">
          <summary className="min-h-6 cursor-pointer text-ink-muted">
            {t("plan.ledger.day_hints", { count: ledger.dayHints.length })}
          </summary>
          <ul className="mt-1 flex flex-col gap-0.5 text-ink-muted">
            {ledger.dayHints.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </details>
      )}
    </section>
  )
}
