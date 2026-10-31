import { Plus, Trash2 } from "lucide-react"
import { useId, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  PRICE_OVERRIDES_KEY,
  type ModelPrice,
  type PriceInfo,
  type PriceOverrides,
} from "@/lib/api/prices"
import { formatDate, type Formatter } from "@/lib/plan/format"
import { useModelPrice } from "@/lib/queries/runs"
import { cn } from "@/lib/utils"

import { formatUSD } from "./insights/format"
import { SavedNote, SettingsGroup } from "./controls"
import { useSettingField } from "./setting-field"
import { SettingRow } from "./SettingRow"

const FIELDS = ["in", "out", "cache_read", "cache_write"] as const
type Field = (typeof FIELDS)[number]

interface Row {
  key: number
  model: string
  /** The model id this row is stored under; null until saved. */
  saved: string | null
  /** Typed text per price, USD per 1M tokens; "" = not set. */
  prices: Record<Field, string>
}

function parseRows(raw: string): Row[] {
  let parsed: PriceOverrides = {}
  try {
    parsed = JSON.parse(raw || "{}") as PriceOverrides
  } catch {
    // An unreadable stored value starts the editor empty.
  }
  return Object.entries(parsed).map(([model, price], i) => ({
    key: i,
    model,
    saved: model,
    prices: {
      in: String(price.in),
      out: String(price.out),
      cache_read: price.cache_read == null ? "" : String(price.cache_read),
      cache_write: price.cache_write == null ? "" : String(price.cache_write),
    },
  }))
}

function price(text: string): number | null {
  if (text.trim() === "") return null
  const n = Number(text)
  return Number.isFinite(n) && n >= 0 ? n : NaN
}

/** A row is complete with a model id and valid in/out prices; cache
 *  prices are optional. */
function rowPrice(row: Row): ModelPrice | null {
  const [inP, outP, read, write] = FIELDS.map((f) => price(row.prices[f]))
  if (row.model.trim() === "" || inP === null || outP === null) return null
  if ([inP, outP, read, write].some((v) => Number.isNaN(v))) return null
  return { in: inP!, out: outP!, cache_read: read, cache_write: write }
}

type RowIssue = "incomplete" | "duplicate"

/** Why a row can't be saved, or null. */
function rowIssue(row: Row, rows: Row[]): RowIssue | null {
  if (rowPrice(row) === null) return "incomplete"
  const id = row.model.trim()
  return rows.some((r) => r !== row && r.model.trim() === id)
    ? "duplicate"
    : null
}

function serialize(rows: Row[]): string {
  const out: PriceOverrides = {}
  for (const row of rows) {
    const p = rowPrice(row)
    if (p) out[row.model.trim()] = p
  }
  return JSON.stringify(out)
}

function sameOverrides(a: string, b: string): boolean {
  try {
    return (
      JSON.stringify(JSON.parse(a || "{}")) ===
      JSON.stringify(JSON.parse(b || "{}"))
    )
  } catch {
    return false
  }
}

/** Settings → Assistant → Prices (D38): the configured model's built-in
 *  price and the override rows, saved as `ai.price_overrides` on blur. */
export function PricesEditor() {
  const { t } = useTranslation()
  const model = useSettingField("ai.model", "")
  const overrides = useSettingField(PRICE_OVERRIDES_KEY, "{}")

  return (
    <SettingsGroup
      title={t("settings_page.assistant.prices_group")}
      description={t("settings_page.assistant.prices_effect")}
    >
      <SettingRow label={t("settings_page.assistant.prices_current_label")}>
        <CurrentPrice model={model.value} />
      </SettingRow>
      {/* The rows start from the stored value, so wait for it. */}
      {overrides.item && (
        <OverrideRows
          stored={overrides.value}
          save={overrides.save}
          saved={overrides.saved}
        />
      )}
    </SettingsGroup>
  )
}

function priceLine(p: ModelPrice, f: Formatter): string {
  return f.t("settings_page.assistant.prices_line", {
    in: formatUSD(p.in, f),
    out: formatUSD(p.out, f),
  })
}

function CurrentPrice({ model }: { model: string }) {
  const { t, i18n } = useTranslation()
  const info = useModelPrice(model)
  if (model === "") {
    return (
      <p className="text-body text-ink-muted">
        {t("settings_page.assistant.prices_no_model")}
      </p>
    )
  }
  if (info.isError) {
    return (
      <p className="text-body text-ink" data-testid="current-price">
        {t("settings_page.assistant.prices_error")}
      </p>
    )
  }
  if (!info.data) return <p className="text-body text-ink-muted">…</p>
  return (
    <p className="text-body text-ink" data-testid="current-price">
      {currentPriceText(info.data, i18n)}
    </p>
  )
}

function currentPriceText(info: PriceInfo, f: Formatter): string {
  const asOf = formatDate(info.as_of, f, { dateStyle: "medium" })
  if (!info.price) {
    return f.t("settings_page.assistant.prices_none", { model: info.model })
  }
  return f.t(
    info.source === "override"
      ? "settings_page.assistant.prices_override"
      : "settings_page.assistant.prices_builtin",
    { model: info.model, price: priceLine(info.price, f), date: asOf }
  )
}

function OverrideRows({
  stored,
  save,
  saved,
}: {
  stored: string
  save: (value: string) => void
  saved: boolean
}) {
  const { t } = useTranslation()
  // Local rows keep half-typed entries; complete rows save on blur.
  const [rows, setRows] = useState<Row[]>(() => parseRows(stored))
  const [nextKey, setNextKey] = useState(rows.length)

  // A blur saves only when every row is complete and unique, so editing
  // a saved row never drops its stored price; only Remove deletes.
  function commit() {
    if (rows.some((r) => rowIssue(r, rows) !== null)) return
    const value = serialize(rows)
    if (sameOverrides(value, stored)) return
    save(value)
    setRows((rs) => rs.map((r) => ({ ...r, saved: r.model.trim() })))
  }

  function update(key: number, patch: (row: Row) => Row) {
    setRows((rs) => rs.map((r) => (r.key === key ? patch(r) : r)))
  }

  /** Drops the row and, if it was saved, only its stored price. */
  function remove(key: number) {
    const row = rows.find((r) => r.key === key)
    setRows((rs) => rs.filter((r) => r.key !== key))
    if (!row?.saved) return
    let current: PriceOverrides = {}
    try {
      current = JSON.parse(stored || "{}") as PriceOverrides
    } catch {
      // Unreadable: the save below replaces it.
    }
    delete current[row.saved]
    const value = JSON.stringify(current)
    if (!sameOverrides(value, stored)) save(value)
  }

  function add() {
    setRows((rs) => [
      ...rs,
      {
        key: nextKey,
        model: "",
        saved: null,
        prices: { in: "", out: "", cache_read: "", cache_write: "" },
      },
    ])
    setNextKey((k) => k + 1)
  }

  return (
    <div className="flex flex-col gap-3 py-5">
      <div className="flex flex-wrap items-baseline gap-2">
        <p className="text-body font-semibold text-ink">
          {t("settings_page.assistant.prices_overrides_label")}
        </p>
        <SavedNote saved={saved} />
      </div>
      <p className="text-small text-ink-muted">
        {t("settings_page.assistant.prices_overrides_hint")}
      </p>
      {rows.length > 0 && (
        <ul className="flex flex-col gap-3">
          {rows.map((row) => (
            <OverrideRow
              key={row.key}
              row={row}
              issue={rowIssue(row, rows)}
              onChange={(patch) => update(row.key, patch)}
              onCommit={commit}
              onRemove={() => remove(row.key)}
            />
          ))}
        </ul>
      )}
      <Button
        type="button"
        variant="outline"
        className="self-start"
        onClick={add}
        data-testid="price-override-add"
      >
        <Plus strokeWidth={1.5} aria-hidden />
        {t("settings_page.assistant.prices_add")}
      </Button>
    </div>
  )
}

const FIELD_LABELS: Record<Field, string> = {
  in: "settings_page.assistant.prices_in",
  out: "settings_page.assistant.prices_out",
  cache_read: "settings_page.assistant.prices_cache_read",
  cache_write: "settings_page.assistant.prices_cache_write",
}

function OverrideRow({
  row,
  issue,
  onChange,
  onCommit,
  onRemove,
}: {
  row: Row
  issue: RowIssue | null
  onChange: (patch: (row: Row) => Row) => void
  onCommit: () => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const name = row.model || t("settings_page.assistant.prices_new_row")

  return (
    <li
      className="grid grid-cols-2 gap-2 rounded-cell border border-line bg-panel p-3 sm:grid-cols-[minmax(10rem,1.5fr)_repeat(4,minmax(5rem,1fr))_auto] sm:items-end"
      data-testid="price-override-row"
    >
      <label className="col-span-2 flex flex-col gap-1 sm:col-span-1">
        <span className="text-caption text-ink-muted">
          {t("settings_page.assistant.prices_model")}
        </span>
        <Input
          value={row.model}
          onChange={(e) => {
            const model = e.target.value
            onChange((r) => ({ ...r, model }))
          }}
          onBlur={onCommit}
          aria-describedby={issue ? `${id}-hint` : undefined}
          aria-invalid={issue === "duplicate" || undefined}
          className="pointer-coarse:h-11"
          data-testid="price-override-model"
        />
      </label>
      {FIELDS.map((field) => (
        <label key={field} className="flex flex-col gap-1">
          <span className="text-caption text-ink-muted">
            {t(FIELD_LABELS[field])}
          </span>
          <Input
            type="number"
            inputMode="decimal"
            min={0}
            step="any"
            value={row.prices[field]}
            onChange={(e) => {
              const value = e.target.value
              onChange((r) => ({
                ...r,
                prices: { ...r.prices, [field]: value },
              }))
            }}
            onBlur={onCommit}
            className="tabular-nums pointer-coarse:h-11"
            data-testid={`price-override-${field}`}
          />
        </label>
      ))}
      <Button
        type="button"
        variant="ghost"
        className="col-span-2 size-11 justify-self-end sm:col-span-1 md:pointer-fine:size-9"
        aria-label={t("settings_page.assistant.prices_remove", { model: name })}
        onClick={onRemove}
      >
        <Trash2 strokeWidth={1.5} aria-hidden />
      </Button>
      {issue && (
        <p
          id={`${id}-hint`}
          className={cn(
            "col-span-full text-caption",
            issue === "duplicate" ? "text-danger" : "text-ink-muted"
          )}
        >
          {t(
            issue === "duplicate"
              ? "settings_page.assistant.prices_duplicate"
              : "settings_page.assistant.prices_incomplete"
          )}
        </p>
      )}
    </li>
  )
}
