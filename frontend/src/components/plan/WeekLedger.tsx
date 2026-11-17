import { ArrowDown, ArrowUp, Check, ChevronDown, Info } from "lucide-react"
import { Fragment, useState } from "react"
import { useTranslation } from "react-i18next"

import type { SoftIssue, WeekLedger as Week } from "@/lib/api/plan"
import {
  buildLedger,
  type Ledger,
  type LedgerItem,
  type LedgerStatus,
} from "@/lib/plan/ledger"
import { cn } from "@/lib/utils"

import { useIsPhone } from "./use-is-phone"

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

/** The phone's one line: calm while everything is in range, else only what
 *  is off, each with an arrow (and a word for screen readers). */
function Summary({ items, dge }: { items: LedgerItem[]; dge: string[] }) {
  const { t } = useTranslation()
  const off = items.filter((i) => i.status === "low" || i.status === "high")
  if (off.length === 0 && dge.length === 0) {
    return (
      <span className="text-small text-ink">{t("plan.ledger.on_track")}</span>
    )
  }
  const parts = off.map((i) => {
    const Icon = i.status === "low" ? ArrowDown : ArrowUp
    return (
      <span
        key={i.key}
        className="inline-flex items-center gap-0.5 font-semibold text-warning"
      >
        {i.key === "kcal" ? `${i.label} ${i.value}` : i.label}
        <Icon className="size-3" aria-hidden="true" />
        <span className="sr-only">{i.statusText}</span>
      </span>
    )
  })
  if (dge.length > 0) {
    parts.push(
      <span key="dge" className="font-semibold text-warning">
        {t("plan.ledger.dge_short")}
      </span>
    )
  }
  return (
    <span className="flex flex-wrap items-center gap-x-1.5 text-small">
      {parts.map((part, i) => (
        <Fragment key={part.key}>
          {i > 0 && (
            <span aria-hidden="true" className="text-ink-muted">
              ·
            </span>
          )}
          {part}
        </Fragment>
      ))}
    </span>
  )
}

function LedgerBody({ ledger, title }: { ledger: Ledger; title: boolean }) {
  const { t } = useTranslation()
  return (
    <>
      <div className="flex flex-wrap items-baseline gap-x-6 gap-y-3">
        {title && (
          <h2 className="text-small font-semibold text-ink">
            {t("plan.ledger.name")}
          </h2>
        )}
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
    </>
  )
}

/**
 * The week ledger: the window's average nutrition per eater as one quiet
 * strip above the board. Values in range are plain; only what is off gets the
 * warning colour, an arrow and a word (never colour alone). The DGE rules
 * show only while one is missed; hints on single days wait in a closed
 * details element. Soft findings never block anything. On a phone it is one
 * summary line that opens on tap.
 */
export function WeekLedger({ week, soft }: { week: Week; soft: SoftIssue[] }) {
  const { t, i18n } = useTranslation()
  const ledger = buildLedger(week, soft, i18n)
  const phone = useIsPhone()
  const [open, setOpen] = useState(false)

  return (
    <section
      aria-label={t("plan.ledger.name")}
      data-testid="week-ledger"
      className="flex flex-col gap-2 rounded-xl border border-line bg-panel px-4 py-3 shadow-card"
    >
      {phone && (
        <button
          type="button"
          aria-expanded={open}
          data-testid="ledger-toggle"
          onClick={() => setOpen((o) => !o)}
          className="-my-1 flex min-h-11 w-full items-center justify-between gap-2 text-left"
        >
          <Summary items={ledger.items} dge={ledger.dge} />
          <ChevronDown
            className={cn(
              "size-4 shrink-0 text-ink-muted",
              open && "rotate-180"
            )}
            aria-hidden="true"
          />
        </button>
      )}
      {(!phone || open) && <LedgerBody ledger={ledger} title={!phone} />}
    </section>
  )
}
