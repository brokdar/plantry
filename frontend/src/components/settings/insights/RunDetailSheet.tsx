import { Link } from "@tanstack/react-router"
import { X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { useIsPhone } from "@/components/plan/use-is-phone"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import type { RunDetail, RunStep } from "@/lib/api/runs"
import { formatNumber, type Formatter } from "@/lib/plan/format"
import { useRunDetail } from "@/lib/queries/runs"
import { cn } from "@/lib/utils"

import {
  formatCost,
  formatDateTime,
  formatPercent,
  modelText,
  scoreText,
} from "./format"

/** A run's detail in a side sheet: its facts, then its steps in plain
 *  language with the raw JSON behind a disclosure. */
export function RunDetailSheet({
  runId,
  onClose,
}: {
  /** undefined = closed. */
  runId: number | undefined
  onClose: () => void
}) {
  const { t } = useTranslation()
  const phone = useIsPhone()
  const run = useRunDetail(runId)

  return (
    <Sheet open={runId !== undefined} onOpenChange={(o) => !o && onClose()}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        showCloseButton={false}
        className={cn(
          "flex flex-col gap-0 bg-panel p-0 text-ink shadow-sheet",
          phone ? "max-h-[90dvh] rounded-t-sheet" : "w-full sm:max-w-lg"
        )}
        data-testid="run-detail-sheet"
      >
        <SheetClose asChild>
          <Button
            type="button"
            variant="ghost"
            className="absolute top-3 right-3 size-11 md:pointer-fine:size-8"
            aria-label={t("common.close")}
          >
            <X strokeWidth={1.5} aria-hidden />
          </Button>
        </SheetClose>
        {run.data && run.data.id === runId ? (
          <Detail run={run.data} />
        ) : (
          <>
            <SheetHeader className="px-5 pt-5 pr-14 pb-3">
              <SheetTitle className="text-h3 font-semibold text-ink">
                {t("settings_page.insights.detail_title")}
              </SheetTitle>
              <SheetDescription className="sr-only">
                {t("common.loading")}
              </SheetDescription>
            </SheetHeader>
            <div className="flex flex-col gap-3 px-5">
              {run.isError ? (
                <p className="text-body text-ink">
                  {t("settings_page.insights.detail_error")}
                </p>
              ) : (
                <>
                  <Skeleton className="h-6 w-2/3" />
                  <Skeleton className="h-32 w-full" />
                </>
              )}
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

function Detail({ run }: { run: RunDetail }) {
  const { t, i18n } = useTranslation()
  const facts: [label: string, value: string, testId?: string][] = [
    [t("settings_page.insights.col.model"), modelText(run, i18n)],
    [t("settings_page.insights.col.prompt_version"), run.prompt_version ?? "–"],
    [
      t("settings_page.insights.col.tokens"),
      t("settings_page.insights.tokens_detail", {
        input: formatNumber(run.tokens.input, i18n),
        output: formatNumber(run.tokens.output, i18n),
      }) +
        (run.tokens.cache_read + run.tokens.cache_write > 0
          ? ` · ${t("settings_page.insights.tokens_cache", {
              read: formatNumber(run.tokens.cache_read, i18n),
              write: formatNumber(run.tokens.cache_write, i18n),
            })}`
          : "") +
        (run.tokens.reasoning > 0
          ? ` · ${t("settings_page.insights.tokens_reasoning", {
              count: run.tokens.reasoning,
            })}`
          : ""),
    ],
    [
      t("settings_page.insights.duration"),
      run.duration_ms === null
        ? "–"
        : t("settings_page.insights.duration_value", {
            ms: formatNumber(run.duration_ms, i18n),
          }),
    ],
    [
      t("settings_page.insights.col.cost"),
      formatCost(run.cost_estimate_usd, i18n),
    ],
    ...(run.web_search_requests > 0
      ? [
          [
            t("settings_page.insights.web_searches"),
            formatNumber(run.web_search_requests, i18n),
            "run-detail-searches",
          ] as [string, string, string],
        ]
      : []),
    [
      t("settings_page.insights.col.status"),
      t(`settings_page.insights.status.${run.status}`),
    ],
    [t("settings_page.insights.col.score"), scoreText(run, i18n)],
    [
      t("settings_page.insights.col.outcome"),
      run.outcome ? t(`settings_page.insights.outcome.${run.outcome}`) : "–",
    ],
    [
      t("settings_page.insights.correction_rate"),
      run.correction_rate === null
        ? "–"
        : formatPercent(run.correction_rate, i18n),
    ],
    [
      t("settings_page.insights.col.edits"),
      run.edits === null ? "–" : formatNumber(run.edits, i18n),
      "run-detail-edits",
    ],
  ]

  return (
    <>
      <SheetHeader className="px-5 pt-5 pr-14 pb-3">
        <SheetTitle className="text-h3 font-semibold text-ink">
          {t(`settings_page.insights.kind.${run.kind}`)}
        </SheetTitle>
        <SheetDescription className="text-small text-ink-muted tabular-nums">
          {formatDateTime(run.started_at, i18n)}
        </SheetDescription>
      </SheetHeader>
      <div className="flex flex-col gap-6 overflow-y-auto px-5 pb-6">
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-small">
          {facts.map(([label, value, testId]) => (
            <div key={label} data-testid={testId} className="contents">
              <dt className="text-ink-muted">{label}</dt>
              <dd className="text-ink tabular-nums">{value}</dd>
            </div>
          ))}
        </dl>

        {run.error_key && (
          <p className="text-small text-danger" role="note">
            {t(run.error_key, {
              defaultValue: t("settings_page.insights.error_generic"),
            })}
          </p>
        )}
        {run.instruction && (
          <Block title={t("settings_page.insights.instruction")}>
            <p className="text-body whitespace-pre-wrap">{run.instruction}</p>
          </Block>
        )}
        {run.notes && (
          <Block title={t("settings_page.insights.notes")}>
            <p className="text-body whitespace-pre-wrap">{run.notes}</p>
          </Block>
        )}

        <Block title={t("settings_page.insights.steps_title")}>
          {run.steps.length === 0 ? (
            <p className="text-small text-ink-muted">
              {t("settings_page.insights.steps_empty")}
            </p>
          ) : (
            <ol className="flex flex-col divide-y divide-line">
              {run.steps.map((step) => (
                <Step key={step.seq} step={step} />
              ))}
            </ol>
          )}
        </Block>

        {run.prompt_text && (
          <details className="text-small">
            <summary className="cursor-pointer text-ink-muted">
              {t("settings_page.insights.prompt_text")}
            </summary>
            <Raw text={run.prompt_text} />
          </details>
        )}
      </div>
    </>
  )
}

function Block({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-body font-semibold text-ink">{title}</h3>
      {children}
    </section>
  )
}

/** A step in plain language: "Asked the assistant (fake, 1,500 tokens)" or
 *  the tool's label ("Looked up your recipes"). */
function stepText(step: RunStep, f: Formatter): string {
  if (step.type === "llm_call") {
    return f.t("settings_page.insights.step.llm_call", {
      model: step.name,
      tokens: formatNumber(step.tokens.input + step.tokens.output, f),
    })
  }
  return f.t(`settings_page.insights.tool.${step.name}`, {
    defaultValue: f.t("settings_page.insights.step.tool_call", {
      name: step.name,
    }),
  })
}

/** The run a `regenerate` tool step started: its result is JSON with a
 *  `run_id`. Null for other steps, a failed call or cut-off JSON. */
function regenerateRunId(step: RunStep): number | null {
  if (step.name !== "regenerate" || step.is_error || !step.result) return null
  try {
    const id = (JSON.parse(step.result) as { run_id?: unknown }).run_id
    return typeof id === "number" && Number.isInteger(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

/** The extra numbers of a model call, only those that are present. */
function stepMetrics(step: RunStep, t: Formatter["t"], f: Formatter): string {
  const parts: string[] = []
  if (step.ttft_ms !== null)
    parts.push(
      t("settings_page.insights.step.ttft", {
        ms: formatNumber(step.ttft_ms, f),
      })
    )
  if (step.tokens.cache_read + step.tokens.cache_write > 0)
    parts.push(
      t("settings_page.insights.step.cache", {
        read: formatNumber(step.tokens.cache_read, f),
        write: formatNumber(step.tokens.cache_write, f),
      })
    )
  if (step.tokens.reasoning > 0)
    parts.push(
      t("settings_page.insights.step.reasoning", {
        count: step.tokens.reasoning,
      })
    )
  if (step.thinking_chars > 0)
    parts.push(
      t("settings_page.insights.step.thinking", { count: step.thinking_chars })
    )
  if (step.served_model && step.served_model !== step.name)
    parts.push(
      t("settings_page.insights.step.served_model", {
        model: step.served_model,
      })
    )
  return parts.join(" · ")
}

/** The reasoning summary of a model call: the text of the thinking blocks in
 *  its response JSON. A cut-off response (not valid JSON) falls back to the
 *  complete `"thinking"` strings in it. */
function thinkingSummary(step: RunStep): string {
  if (step.type !== "llm_call" || step.thinking_chars <= 0 || !step.result)
    return ""
  try {
    const content = (JSON.parse(step.result) as { content?: unknown }).content
    if (Array.isArray(content))
      return content
        .filter((b) => b?.type === "thinking" && typeof b.thinking === "string")
        .map((b) => b.thinking as string)
        .join("\n\n")
  } catch {
    const parts: string[] = []
    for (const m of step.result.matchAll(/"thinking":("(?:[^"\\]|\\.)*")/g)) {
      try {
        parts.push(JSON.parse(m[1]) as string)
      } catch {
        // an incomplete string at the cut
      }
    }
    return parts.join("\n\n")
  }
  return ""
}

function Step({ step }: { step: RunStep }) {
  const { t, i18n } = useTranslation()
  const linkedRun = regenerateRunId(step)
  const thinking = thinkingSummary(step)
  return (
    <li className="flex flex-col gap-1 py-2.5" data-testid="run-step">
      <div className="flex items-baseline justify-between gap-3">
        <span className={cn("text-body", step.is_error && "text-danger")}>
          {stepText(step, i18n)}
          {step.is_error && ` · ${t("settings_page.insights.step.failed")}`}
        </span>
        <span className="shrink-0 text-caption text-ink-muted tabular-nums">
          {t("settings_page.insights.step.duration", {
            ms: formatNumber(step.duration_ms, i18n),
          })}
        </span>
      </div>
      {step.type === "llm_call" && (
        <p
          className="text-caption text-ink-muted tabular-nums"
          data-testid="step-metrics"
        >
          {stepMetrics(step, t, i18n)}
        </p>
      )}
      {thinking && (
        <details className="text-small" data-testid="step-thinking">
          <summary className="cursor-pointer text-ink-muted">
            {t("settings_page.insights.step.thinking_summary")}
          </summary>
          <Raw text={thinking} />
        </details>
      )}
      {linkedRun !== null && (
        <Link
          to="/settings/assistant/insights"
          search={{ run: linkedRun }}
          className="inline-flex min-h-11 w-fit items-center text-small text-accent underline-offset-2 hover:underline md:pointer-fine:min-h-6"
          data-testid="step-run-link"
        >
          {t("settings_page.insights.open_run")} #{linkedRun}
        </Link>
      )}
      {(step.args !== null || step.result !== null) && (
        <details className="text-small">
          <summary className="cursor-pointer text-ink-muted">
            {t("settings_page.insights.raw_json")}
          </summary>
          {step.args !== null && (
            <Raw
              label={t("settings_page.insights.raw_args")}
              text={step.args}
              truncated={step.args_truncated}
            />
          )}
          {step.result !== null && (
            <Raw
              label={t("settings_page.insights.raw_result")}
              text={step.result}
              truncated={step.result_truncated}
            />
          )}
        </details>
      )}
    </li>
  )
}

function Raw({
  label,
  text,
  truncated,
}: {
  label?: string
  text: string
  truncated?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className="mt-2 flex flex-col gap-1">
      {label && <span className="text-caption text-ink-muted">{label}</span>}
      <pre className="max-h-64 overflow-auto rounded-cell bg-panel-sunken p-2 font-mono text-caption whitespace-pre-wrap text-ink">
        {text}
      </pre>
      {truncated && (
        <span className="text-caption text-ink-muted">
          {t("settings_page.insights.truncated")}
        </span>
      )}
    </div>
  )
}
