import { useState } from "react"
import { useTranslation } from "react-i18next"
import {
  Bug,
  ChevronDown,
  ChevronRight,
  Copy,
  Check,
  Database,
  Globe,
  Layers,
  Sparkles,
  Code,
} from "lucide-react"

import { cn } from "@/lib/utils"
import type { TraceEntry, TraceLevel } from "@/lib/api/lookup"

interface LookupDebugPanelProps {
  trace: TraceEntry[]
}

const LEVEL_BORDER: Record<TraceLevel, string> = {
  info: "border-l-outline/50",
  success: "border-l-primary",
  warning: "border-l-amber-400",
  error: "border-l-destructive",
}

const LEVEL_DOT: Record<TraceLevel, string> = {
  info: "bg-outline/60",
  success: "bg-primary",
  warning: "bg-amber-400",
  error: "bg-destructive",
}

function iconFor(step: string) {
  if (step.startsWith("ai.")) return Sparkles
  if (step.startsWith("off.")) return Globe
  if (step.startsWith("fdc.")) return Database
  if (step.startsWith("resolve.")) return Layers
  return Code
}

const STEP_LABELS: Record<string, string> = {
  "ai.translate": "AI Translate",
  "ai.pick_best": "AI Pick Best",
  "off.lookup_barcode": "OFF Barcode",
  "off.search": "OFF Search",
  "fdc.search": "FDC Search",
  "fdc.search_barcode": "FDC Barcode",
}

function stepLabel(step: string) {
  return STEP_LABELS[step] ?? step
}

/**
 * Debug panel rendered below the lookup results when `?debug=true` is set.
 * Each entry is a level-accented row with step icon, humanized label,
 * summary, and a duration pill; clicking expands a collapsible detail pane
 * with the raw trace payload. Only rendered when at least one entry exists.
 */
export function LookupDebugPanel({ trace }: LookupDebugPanelProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const [copied, setCopied] = useState<string | null>(null)

  if (trace.length === 0) return null

  function toggle(i: number) {
    setExpanded((prev) => ({ ...prev, [i]: !prev[i] }))
  }

  async function copy(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(key)
      setTimeout(() => setCopied(null), 1500)
    } catch {
      // clipboard may be unavailable (file://, iframe, etc.) — ignore silently
    }
  }

  const allJSON = JSON.stringify(trace, null, 2)

  return (
    <div className="overflow-hidden rounded-2xl border border-outline-variant/60 bg-surface-container-low font-mono text-xs">
      <header className="flex items-center justify-between border-b border-outline-variant/40 bg-surface-container/60 px-4 py-2.5">
        <div className="flex items-center gap-2 text-on-surface-variant">
          <Bug className="size-3.5" aria-hidden />
          <span className="text-[10px] font-bold tracking-[0.22em] uppercase">
            {t("lookup.debug_title")}
          </span>
          <span className="rounded bg-surface-container-high px-1.5 py-0.5 text-[10px] font-semibold">
            {trace.length} {t("lookup.debug_events")}
          </span>
        </div>
        <button
          type="button"
          onClick={() => copy(allJSON, "__all__")}
          className="flex items-center gap-1.5 rounded px-2 py-1 text-[10px] text-on-surface-variant transition-colors hover:bg-surface-container-high"
          title={t("lookup.debug_copy_all")}
        >
          {copied === "__all__" ? (
            <>
              <Check className="size-3 text-primary" aria-hidden />
              {t("lookup.debug_copied")}
            </>
          ) : (
            <>
              <Copy className="size-3" aria-hidden />
              {t("lookup.debug_copy_all")}
            </>
          )}
        </button>
      </header>

      <ol className="divide-y divide-outline-variant/30">
        {trace.map((entry, idx) => {
          const Icon = iconFor(entry.step)
          const isOpen = !!expanded[idx]
          const hasDetail =
            entry.detail != null &&
            (typeof entry.detail !== "object" ||
              Object.keys(entry.detail as object).length > 0)
          const detailJSON = hasDetail
            ? JSON.stringify(entry.detail, null, 2)
            : ""
          return (
            <li key={`${entry.step}-${idx}`}>
              <button
                type="button"
                onClick={() => hasDetail && toggle(idx)}
                disabled={!hasDetail}
                aria-expanded={hasDetail ? isOpen : undefined}
                className={cn(
                  "flex w-full items-center gap-3 border-l-2 px-4 py-2 text-left transition-colors",
                  LEVEL_BORDER[entry.level],
                  hasDetail
                    ? "cursor-pointer hover:bg-surface-container"
                    : "cursor-default"
                )}
              >
                <Icon
                  className="size-3.5 shrink-0 text-on-surface-variant"
                  aria-hidden
                />
                <span
                  aria-hidden
                  className={cn(
                    "inline-block size-1.5 shrink-0 rounded-full",
                    LEVEL_DOT[entry.level]
                  )}
                />
                <span className="min-w-[7rem] shrink-0 text-[10px] font-bold tracking-[0.14em] text-on-surface-variant uppercase">
                  {stepLabel(entry.step)}
                </span>
                <span className="min-w-0 flex-1 truncate text-[11px] text-on-surface">
                  {entry.summary}
                </span>
                {entry.duration_ms != null && (
                  <span className="shrink-0 rounded bg-surface-container-high px-1.5 py-0.5 text-[10px] text-on-surface-variant tabular-nums">
                    {entry.duration_ms} ms
                  </span>
                )}
                {hasDetail && (
                  <span className="shrink-0 text-on-surface-variant">
                    {isOpen ? (
                      <ChevronDown className="size-3.5" aria-hidden />
                    ) : (
                      <ChevronRight className="size-3.5" aria-hidden />
                    )}
                  </span>
                )}
              </button>
              {isOpen && hasDetail && (
                <div className="relative bg-surface-container/50 px-4 py-3">
                  <button
                    type="button"
                    onClick={() => copy(detailJSON, `d-${idx}`)}
                    className="absolute top-2 right-2 rounded p-1 text-on-surface-variant transition-colors hover:bg-surface-container-high"
                    title={t("lookup.debug_copy")}
                  >
                    {copied === `d-${idx}` ? (
                      <Check className="size-3 text-primary" aria-hidden />
                    ) : (
                      <Copy className="size-3" aria-hidden />
                    )}
                  </button>
                  <pre className="overflow-x-auto pr-8 text-[10px] leading-relaxed whitespace-pre-wrap text-on-surface-variant">
                    {detailJSON}
                  </pre>
                </div>
              )}
            </li>
          )
        })}
      </ol>
    </div>
  )
}
