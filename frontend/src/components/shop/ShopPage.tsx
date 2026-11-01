import { Link } from "@tanstack/react-router"
import { ClipboardCopy, ShoppingCart } from "lucide-react"
import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Switch } from "@/components/settings/controls"
import { Button } from "@/components/ui/button"
import { ApiError } from "@/lib/api/client"
import type { ShoppingList, ShoppingRange } from "@/lib/api/shopping"
import { useLocalStorageState } from "@/lib/hooks/useLocalStorageState"
import { useWakeLock } from "@/lib/hooks/useWakeLock"
import { colourBatches } from "@/lib/plan/batch-colours"
import { UNDO_MS, usePlan } from "@/lib/queries/plan"
import { useShoppingList } from "@/lib/queries/shopping"
import { slotLabel } from "@/lib/slot-label"
import { toast } from "@/lib/toast"
import { cn } from "@/lib/utils"

import { shoppingText } from "./CopyAsText"
import { RangePicker } from "./RangePicker"
import { SectionList } from "./SectionList"
import type { ShopBatch, UsedInContext } from "./ShoppingRow"

interface ShopPageProps {
  /** The range from the URL; without it the server's default for the
   *  anchored window. */
  range?: ShoppingRange
  /** Draft plates count too (`?drafts=1`); off by default (D26). */
  includeDrafts: boolean
  onRangeChange: (range: ShoppingRange) => void
  onIncludeDraftsChange: (includeDrafts: boolean) => void
}

/**
 * `/shop` (UX §5.7): everything to buy for a range in one walk through the
 * store. `GET /api/shopping-list` renders the list; `GET /api/plan` for the
 * range's window adds batch colours and slot names to the "used in" rows.
 */
export function ShopPage({
  range,
  includeDrafts,
  onRangeChange,
  onIncludeDraftsChange,
}: ShopPageProps) {
  const { t } = useTranslation()
  const query = useShoppingList(range, includeDrafts)

  if (query.isPending) {
    return (
      <Frame>
        <div
          role="status"
          aria-label={t("shop.loading")}
          className="flex flex-col gap-8"
          data-testid="shop-skeleton"
        >
          <div className="flex flex-col gap-2">
            <Block className="h-9 w-72" />
            <Block className="h-4 w-40" />
          </div>
          {[0, 1].map((s) => (
            <div key={s} className="flex flex-col gap-2">
              <Block className="h-6 w-32" />
              {[0, 1, 2].map((i) => (
                <Block key={i} className="h-11" />
              ))}
            </div>
          ))}
        </div>
      </Frame>
    )
  }
  if (query.isError) {
    return (
      <ShopError
        error={query.error}
        range={range}
        onRangeChange={onRangeChange}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return (
    <ShopLoaded
      list={query.data}
      includeDrafts={includeDrafts}
      onRangeChange={onRangeChange}
      onIncludeDraftsChange={onIncludeDraftsChange}
    />
  )
}

/**
 * The list failed. A refused range (4xx) names the problem and has no Retry,
 * since retrying cannot help; the range picker stays, its presets anchored
 * on the current window, so another range is one tap away.
 */
function ShopError({
  error,
  range,
  onRangeChange,
  onRetry,
}: {
  error: Error
  range?: ShoppingRange
  onRangeChange: (range: ShoppingRange) => void
  onRetry: () => void
}) {
  const { t } = useTranslation()
  const current = usePlan().data?.window
  const refused =
    error instanceof ApiError && error.status >= 400 && error.status < 500
  const planWindow = current && { from: current.start, to: current.end }

  return (
    <Frame>
      <div className="flex flex-col gap-6">
        {planWindow && (
          <header>
            <RangePicker
              range={range ?? planWindow}
              window={planWindow}
              onChange={onRangeChange}
            />
          </header>
        )}
        <div role="alert" className="flex flex-col items-start gap-3">
          <p className="text-body text-ink">
            {refused ? t(error.messageKey) : t("shop.error")}
          </p>
          {!refused && (
            <Button variant="outline" onClick={onRetry}>
              {t("common.retry")}
            </Button>
          )}
        </div>
      </div>
    </Frame>
  )
}

function ShopLoaded({
  list,
  includeDrafts,
  onRangeChange,
  onIncludeDraftsChange,
}: {
  list: ShoppingList
  includeDrafts: boolean
  onRangeChange: (range: ShoppingRange) => void
  onIncludeDraftsChange: (includeDrafts: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const { range } = list
  const planWindow = { from: range.window_start, to: range.window_end }
  const [ticked, toggle, clearTicked] = useTicked(list)
  const [inStore, setInStore] = useState(false)
  useWakeLock(inStore)
  const usedIn = useUsedIn(range.window_start)

  const tickedCount = ticked.size
  const allDone = list.count > 0 && tickedCount === list.count

  async function copy() {
    try {
      await navigator.clipboard.writeText(
        shoppingText(list.sections, ticked, i18n)
      )
      toast.success(t("shop.copied"))
    } catch {
      toast.error(t("shop.copy_failed"))
    }
  }

  return (
    <Frame>
      <div className="flex flex-col gap-6">
        <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
          <div className="flex min-w-0 flex-col gap-1">
            <RangePicker
              range={{ from: range.from, to: range.to }}
              window={planWindow}
              onChange={onRangeChange}
            />
            {list.has_drafts && (
              <div className="flex items-center gap-1">
                <Switch
                  id="shop-include-drafts"
                  checked={includeDrafts}
                  onChange={onIncludeDraftsChange}
                />
                <label
                  htmlFor="shop-include-drafts"
                  className="cursor-pointer text-small text-ink"
                >
                  {t("shop.include_drafts")}
                </label>
              </div>
            )}
            <p
              className="text-small text-ink-muted tabular-nums"
              data-testid="shop-count"
            >
              {t("shop.items", { count: list.count })}
              {list.count > 0 &&
                ` · ${t("shop.ticked", { count: tickedCount })}`}
            </p>
          </div>
          {list.count > 0 && (
            <div className="flex items-center gap-2">
              <Button
                variant={inStore ? "default" : "outline"}
                aria-pressed={inStore}
                onClick={() => setInStore((v) => !v)}
                className="h-11 gap-2 px-3 lg:hidden"
              >
                <ShoppingCart strokeWidth={1.5} aria-hidden="true" />
                {t("shop.in_store")}
              </Button>
              <Button
                variant="outline"
                onClick={() => void copy()}
                className="hidden h-11 gap-2 px-3 md:inline-flex md:pointer-fine:h-9"
              >
                <ClipboardCopy strokeWidth={1.5} aria-hidden="true" />
                {t("shop.copy")}
              </Button>
            </div>
          )}
        </header>

        {list.count === 0 && list.has_drafts && !includeDrafts ? (
          <div
            className="flex flex-col items-start gap-3 rounded-lg border border-dashed border-line bg-panel p-6"
            data-testid="shop-draft-empty"
          >
            <p className="font-heading text-h3 text-ink">
              {t("shop.draft_empty.title")}
            </p>
            <p className="text-body text-ink-muted">
              {t("shop.draft_empty.body")}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() => onIncludeDraftsChange(true)}
                className="h-11 md:pointer-fine:h-9"
              >
                {t("shop.draft_empty.include")}
              </Button>
              <Button
                asChild
                variant="outline"
                className="h-11 md:pointer-fine:h-9"
              >
                <Link to="/plan" search={{ start: range.window_start }}>
                  {t("shop.draft_empty.review")}
                </Link>
              </Button>
            </div>
          </div>
        ) : list.count === 0 ? (
          <div
            className="flex flex-col items-start gap-3 rounded-lg border border-line bg-panel p-6"
            data-testid="shop-empty"
          >
            <p className="font-heading text-h3 text-ink">{t("shop.empty")}</p>
            <Button asChild className="h-11 md:pointer-fine:h-9">
              <Link to="/plan" search={{ start: range.window_start }}>
                {t("shop.go_to_plan")}
              </Link>
            </Button>
          </div>
        ) : (
          <>
            {allDone && (
              <p
                role="status"
                className="font-heading text-h3 text-success"
                data-testid="shop-all-done"
              >
                {t("shop.all_done", { count: list.count })}
              </p>
            )}
            <SectionList
              sections={list.sections}
              ticked={ticked}
              onToggle={toggle}
              usedIn={usedIn}
              large={inStore}
            />
            <div
              className={cn(
                "flex items-center gap-2",
                inStore &&
                  "sticky bottom-[var(--tab-bar-inset)] -mx-4 border-t border-line bg-ground px-4 py-3"
              )}
            >
              {tickedCount > 0 && (
                <Button
                  variant="outline"
                  onClick={clearTicked}
                  className="h-11 md:pointer-fine:h-9"
                >
                  {t("shop.clear_ticked")}
                </Button>
              )}
              {inStore && (
                <Button
                  onClick={() => setInStore(false)}
                  className="ml-auto h-11"
                >
                  {t("shop.in_store_exit")}
                </Button>
              )}
            </div>
          </>
        )}
      </div>
    </Frame>
  )
}

/**
 * The ticked food ids of this list's range, kept in localStorage under
 * `plantry:purchased:range:{from}:{to}`. Ids no longer on the list are
 * ignored and dropped on the next write.
 */
function useTicked(
  list: ShoppingList
): [ReadonlySet<number>, (foodId: number) => void, () => void] {
  const { t } = useTranslation()
  const key = `plantry:purchased:range:${list.range.from}:${list.range.to}`
  const [raw, setRaw] = useLocalStorageState<string>(key, "[]")

  const ticked = useMemo(() => {
    const onList = new Set(
      list.sections.flatMap((s) => s.items.map((i) => i.food_id))
    )
    return new Set(parseIds(raw).filter((id) => onList.has(id)))
  }, [raw, list])

  function write(ids: Iterable<number>) {
    setRaw(JSON.stringify([...ids]))
  }

  // Ticks off the list stay saved: an item only on drafts comes back when
  // the draft toggle is on again.
  function toggle(foodId: number) {
    const next = new Set(parseIds(raw))
    if (next.has(foodId)) next.delete(foodId)
    else next.add(foodId)
    write(next)
  }

  function clear() {
    const before = parseIds(raw)
    write([])
    toast(t("shop.cleared"), {
      duration: UNDO_MS,
      action: { label: t("common.undo"), onClick: () => write(before) },
    })
  }

  return [ticked, toggle, clear]
}

function parseIds(raw: string): number[] {
  try {
    const value: unknown = JSON.parse(raw)
    return Array.isArray(value)
      ? value.filter((v): v is number => typeof v === "number")
      : []
  } catch {
    return []
  }
}

/** Batch colours and slot names of the window the range starts in. Until
 *  the plan loads, and for batches outside that window, rows show no badge. */
function useUsedIn(windowStart: string): UsedInContext {
  const { t } = useTranslation()
  const plan = usePlan(windowStart).data
  return useMemo(() => {
    const batches = new Map<number, ShopBatch>()
    const slots = new Map<number, string>()
    if (plan) {
      for (const { session, colour } of colourBatches(
        plan.sessions,
        plan.slots
      )) {
        batches.set(session.id, { colour, portions: session.portions })
      }
      for (const slot of plan.slots) {
        slots.set(slot.id, slotLabel(t, slot.name_key))
      }
    }
    return { batches, slotName: (id) => slots.get(id) ?? null }
  }, [plan, t])
}

function Frame({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto w-full max-w-5xl px-4 py-6 md:px-8 md:py-8">
      {children}
    </div>
  )
}

function Block({ className }: { className: string }) {
  return (
    <div
      className={cn("animate-pulse rounded-cell bg-panel-sunken", className)}
    />
  )
}
