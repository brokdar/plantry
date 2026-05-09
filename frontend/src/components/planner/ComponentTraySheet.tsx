import { format, parseISO } from "date-fns"
import { de as deLocale } from "date-fns/locale"
import {
  Bookmark,
  Clock,
  Heart,
  Loader2,
  Minus,
  Plus,
  Search,
  Sparkles,
  Utensils,
  X,
} from "lucide-react"
import {
  useCallback,
  useDeferredValue,
  useEffect,
  useId,
  useMemo,
  useState,
} from "react"
import { useTranslation } from "react-i18next"

import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import type { Food, FoodRole } from "@/lib/api/foods"
import type { Template } from "@/lib/api/templates"
import { useFoodMacros, useFoods } from "@/lib/queries/foods"
import { useTemplates } from "@/lib/queries/templates"
import { imageURL } from "@/lib/image-url"
import { slotLabel } from "@/lib/slot-label"
import { cn } from "@/lib/utils"

const DAY_KEYS = [
  "planner.day_mon",
  "planner.day_tue",
  "planner.day_wed",
  "planner.day_thu",
  "planner.day_fri",
  "planner.day_sat",
  "planner.day_sun",
] as const

const ROLE_FILTERS: { key: FoodRole; labelKey: string }[] = [
  { key: "main", labelKey: "component.role_main" },
  { key: "side_starch", labelKey: "component.role_side_starch" },
  { key: "side_veg", labelKey: "component.role_side_veg" },
  { key: "side_protein", labelKey: "component.role_side_protein" },
  { key: "sauce", labelKey: "component.role_sauce" },
  { key: "drink", labelKey: "component.role_drink" },
  { key: "dessert", labelKey: "component.role_dessert" },
]

type TabKey = "foods" | "templates" | "favorites" | "recent"

export interface TraySlotContext {
  slotId: number
  slotNameKey: string
  date: string
  weekday: number
}

export interface TrayItem {
  food: Food
  portions: number
}

/** Result of a tray commit. When some additions failed, the parent returns
 * the food_ids that didn't land; the sheet keeps those staged so the user
 * can retry without losing what they meant to add. */
export interface TrayCommitResult {
  failedFoodIds: number[]
}

interface ComponentTraySheetProps {
  open: boolean
  context: TraySlotContext | null
  /** Foods recently used in plates (last 20 unique, most-recent first). Frontend-derived. */
  recentFoods?: Food[]
  side?: "right" | "bottom"
  onOpenChange: (open: boolean) => void
  onCommit: (
    items: { food_id: number; portions: number }[],
    context: TraySlotContext
  ) => Promise<TrayCommitResult | void> | TrayCommitResult | void
}

export function ComponentTraySheet({
  open,
  context,
  recentFoods,
  side = "right",
  onOpenChange,
  onCommit,
}: ComponentTraySheetProps) {
  const { t } = useTranslation()
  const sheetSide = side === "bottom" ? "bottom" : "right"

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={sheetSide}
        showCloseButton={false}
        className={cn(
          "flex flex-col gap-0 border-outline-variant/40 bg-surface-container-lowest p-0",
          sheetSide === "right" &&
            "w-full sm:max-w-[480px] data-[side=right]:sm:max-w-[480px]",
          sheetSide === "bottom" && "max-h-[92dvh] rounded-t-3xl"
        )}
        data-testid="tray-sheet"
      >
        {context ? (
          // Remount on slot change so the staged tray and persisted role
          // filters reset cleanly to the new slot's last-used state.
          <ComponentTraySheetBody
            key={`${context.slotId}:${context.date}`}
            context={context}
            recentFoods={recentFoods ?? []}
            onClose={() => onOpenChange(false)}
            onCommit={onCommit}
          />
        ) : (
          <SheetHeader>
            <SheetTitle className="sr-only">{t("tray.title_empty")}</SheetTitle>
            <SheetDescription className="sr-only">
              {t("tray.title_empty")}
            </SheetDescription>
          </SheetHeader>
        )}
      </SheetContent>
    </Sheet>
  )
}

interface BodyProps {
  context: TraySlotContext
  recentFoods: Food[]
  onClose: () => void
  onCommit: ComponentTraySheetProps["onCommit"]
}

function ComponentTraySheetBody({
  context,
  recentFoods,
  onClose,
  onCommit,
}: BodyProps) {
  const { t, i18n } = useTranslation()

  const slotName = slotLabel(t, context.slotNameKey)
  const dayKey = DAY_KEYS[context.weekday] ?? DAY_KEYS[0]
  const dayLabel = t(dayKey)
  const dateLocale = i18n.language?.startsWith("de") ? deLocale : undefined
  const dateLabel = format(parseISO(context.date), "MMM d", {
    locale: dateLocale,
  })

  const [tab, setTab] = useState<TabKey>("foods")
  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [roles, setRoles] = useSlotRoleFilter(context.slotId)
  const [tray, dispatch] = useTray()
  const [committing, setCommitting] = useState(false)

  const handleStageFood = useCallback(
    (food: Food) => dispatch({ type: "stage", food }),
    [dispatch]
  )
  const handleStageTemplate = useCallback(
    (tpl: Template, foodsById: Map<number, Food>) =>
      dispatch({ type: "stageTemplate", template: tpl, foodsById }),
    [dispatch]
  )

  async function commit() {
    if (tray.length === 0 || committing) return
    setCommitting(true)
    try {
      const result = await onCommit(
        tray.map((it) => ({ food_id: it.food.id, portions: it.portions })),
        context
      )
      const failed = result?.failedFoodIds ?? []
      if (failed.length === 0) {
        onClose()
      } else {
        // Partial failure: keep only the failed items staged so the user
        // can see what didn't land and retry — closing here would silently
        // drop their intent on top of an already-half-built plate.
        dispatch({ type: "keepOnly", foodIds: new Set(failed) })
      }
    } finally {
      setCommitting(false)
    }
  }

  return (
    <>
      <SheetHeader className="gap-2 border-b border-outline-variant/40 bg-surface-container-low/40 px-5 py-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex flex-col gap-1">
            <span
              className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase"
              data-testid="tray-eyebrow"
            >
              {dayLabel} · {dateLabel}
            </span>
            <SheetTitle className="font-heading text-[22px] leading-tight font-bold tracking-tight text-on-surface">
              {t("tray.title", { slot: slotName })}
            </SheetTitle>
            <SheetDescription className="text-[12.5px] text-on-surface-variant">
              {t("tray.subtitle")}
            </SheetDescription>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onClose}
            aria-label={t("common.close")}
            className="text-on-surface-variant"
            data-testid="tray-close"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      </SheetHeader>

      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-5 pt-4 pb-3">
        <SearchField value={search} onChange={setSearch} />

        <BrowserTabs value={tab} onChange={setTab} />

        {/* Templates aren't role-tagged so the chips are hidden there.
          Foods/Favorites/Recent all respect the persisted role filter — the
          chips stay visible across them so the user can always widen, even
          when a stored filter from a previous open is silently narrowing. */}
        {tab !== "templates" && <RoleChips value={roles} onChange={setRoles} />}

        <div className="min-h-0 flex-1">
          {tab === "templates" ? (
            <TemplateResults onStage={handleStageTemplate} />
          ) : (
            <FoodResults
              tab={tab}
              search={deferredSearch}
              roles={roles}
              recentFoods={recentFoods}
              onStage={handleStageFood}
            />
          )}
        </div>
      </div>

      <TrayFooter
        items={tray}
        onPortion={(id, p) => dispatch({ type: "portion", foodId: id, p })}
        onRemove={(id) => dispatch({ type: "remove", foodId: id })}
        onCancel={onClose}
        onCommit={commit}
        committing={committing}
      />
    </>
  )
}

function SearchField({
  value,
  onChange,
}: {
  value: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className="relative">
      <Search
        className="absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-on-surface-variant/70"
        aria-hidden
      />
      <Input
        id={id}
        type="search"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t("picker.search.placeholder")}
        aria-label={t("picker.search.placeholder")}
        data-testid="tray-search"
        className="h-9 border-outline-variant/40 bg-surface-container-low/60 pl-9 text-[13px] focus-visible:border-primary/60 focus-visible:bg-surface-container-lowest"
      />
    </div>
  )
}

function BrowserTabs({
  value,
  onChange,
}: {
  value: TabKey
  onChange: (k: TabKey) => void
}) {
  const { t } = useTranslation()
  const items: { key: TabKey; label: string; Icon: typeof Heart }[] = [
    { key: "foods", label: t("tray.tab.foods"), Icon: Utensils },
    { key: "templates", label: t("tray.tab.templates"), Icon: Bookmark },
    { key: "favorites", label: t("tray.tab.favorites"), Icon: Heart },
    { key: "recent", label: t("tray.tab.recent"), Icon: Clock },
  ]
  return (
    <div
      role="tablist"
      aria-label={t("tray.tabs_label")}
      className="-mx-1 flex gap-1 overflow-x-auto"
    >
      {items.map(({ key, label, Icon }) => {
        const active = value === key
        return (
          <button
            key={key}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(key)}
            data-testid={`tray-tab-${key}`}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 font-heading text-[11px] font-bold tracking-[0.14em] uppercase transition-colors",
              active
                ? "border-transparent bg-on-surface text-surface-container-lowest"
                : "border-outline-variant/50 bg-surface-container-lowest text-on-surface-variant hover:border-primary/40 hover:text-on-surface"
            )}
          >
            <Icon
              className="h-3 w-3"
              fill={active && key === "favorites" ? "currentColor" : "none"}
              aria-hidden
            />
            <span>{label}</span>
          </button>
        )
      })}
    </div>
  )
}

function RoleChips({
  value,
  onChange,
}: {
  value: Set<FoodRole>
  onChange: (next: Set<FoodRole>) => void
}) {
  const { t } = useTranslation()

  function toggle(role: FoodRole) {
    const next = new Set(value)
    if (next.has(role)) next.delete(role)
    else next.add(role)
    onChange(next)
  }

  function clear() {
    if (value.size === 0) return
    onChange(new Set())
  }

  return (
    <div className="-mx-1 flex flex-wrap gap-1.5 overflow-x-auto px-1">
      <button
        type="button"
        onClick={clear}
        aria-pressed={value.size === 0}
        data-testid="tray-chip-all"
        className={cn(
          "rounded-full border px-2.5 py-1 font-heading text-[10px] font-bold tracking-[0.14em] uppercase transition-colors",
          value.size === 0
            ? "border-primary/60 bg-primary/12 text-primary"
            : "border-outline-variant/40 bg-surface-container-lowest text-on-surface-variant hover:border-primary/40"
        )}
      >
        {t("component.all_roles")}
      </button>
      {ROLE_FILTERS.map(({ key, labelKey }) => {
        const active = value.has(key)
        return (
          <button
            key={key}
            type="button"
            onClick={() => toggle(key)}
            aria-pressed={active}
            data-testid={`tray-chip-${key}`}
            className={cn(
              "rounded-full border px-2.5 py-1 font-heading text-[10px] font-bold tracking-[0.14em] uppercase transition-colors",
              active
                ? "border-primary/60 bg-primary/12 text-primary"
                : "border-outline-variant/40 bg-surface-container-lowest text-on-surface-variant hover:border-primary/40"
            )}
          >
            {t(labelKey)}
          </button>
        )
      })}
    </div>
  )
}

function FoodResults({
  tab,
  search,
  roles,
  recentFoods,
  onStage,
}: {
  tab: TabKey
  search: string
  roles: Set<FoodRole>
  recentFoods: Food[]
  onStage: (food: Food) => void
}) {
  const { t } = useTranslation()

  // The Foods/Favorites tabs query the API. The Recent tab works off the
  // frontend-derived list and applies search/role filters in memory.
  const queryEnabled = tab !== "recent"
  // When exactly one role is selected, narrow on the server. Multi-select is
  // applied client-side over a single fetch (BE supports only one role param).
  const singleRole = roles.size === 1 ? [...roles][0]! : undefined
  const apiSearch = search.trim() || undefined

  const query = useFoods(
    queryEnabled
      ? {
          search: apiSearch,
          role: singleRole,
          favorite: tab === "favorites" ? 1 : undefined,
          limit: 100,
        }
      : { limit: 1 } // disabled but useFoods always runs; cheapest call
  )

  const items = useMemo(() => {
    if (tab === "recent") {
      return filterFoods(recentFoods, search, roles)
    }
    const data = query.data?.items ?? []
    if (roles.size <= 1) return data
    // Client-side multi-role narrow when more than one role is selected.
    return data.filter((f) =>
      f.kind === "composed" && f.role ? roles.has(f.role) : false
    )
  }, [tab, query.data, recentFoods, search, roles])

  if (queryEnabled && query.isLoading) {
    return (
      <p className="px-1 py-6 text-[12.5px] text-on-surface-variant">
        {t("common.loading")}
      </p>
    )
  }

  if (queryEnabled && query.isError) {
    return (
      <p
        className="px-1 py-6 text-[12.5px] text-destructive"
        data-testid="tray-error"
        role="alert"
      >
        {t("tray.error_load_foods")}
      </p>
    )
  }

  if (items.length === 0) {
    return (
      <p
        className="px-1 py-6 text-[12.5px] text-on-surface-variant"
        data-testid="tray-empty"
      >
        {t(`tray.empty.${tab}`)}
      </p>
    )
  }

  return (
    <ul
      className="grid grid-cols-1 gap-1.5"
      role="list"
      data-testid="tray-results"
    >
      {items.map((food) => (
        <FoodResultRow key={food.id} food={food} onStage={onStage} />
      ))}
    </ul>
  )
}

function FoodResultRow({
  food,
  onStage,
}: {
  food: Food
  onStage: (f: Food) => void
}) {
  const { t } = useTranslation()
  const role = food.kind === "composed" ? (food.role ?? null) : null
  const subLabel =
    food.kind === "leaf"
      ? t("ingredient.kind_label", { defaultValue: "Ingredient" })
      : food.role
        ? t(`component.role_${food.role}`)
        : null

  return (
    <li role="listitem">
      <button
        type="button"
        onClick={() => onStage(food)}
        data-testid={`tray-result-${food.id}`}
        className="group flex w-full items-center gap-3 rounded-xl border border-transparent bg-surface-container-low/40 px-2 py-1.5 text-left transition-colors hover:border-outline-variant/40 hover:bg-surface-container-low"
      >
        <span className="grid size-10 shrink-0 place-items-center overflow-hidden rounded-lg bg-surface-container">
          {food.image_path ? (
            <img
              src={imageURL(food.image_path)}
              alt=""
              className="h-full w-full object-cover"
            />
          ) : (
            <FoodPlaceholder
              category={(role ?? "main") as FoodPlaceholderCategory}
              size="sm"
              rounded="lg"
              className="h-full w-full"
            />
          )}
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-[13px] font-medium text-on-surface">
            {food.name}
          </span>
          {subLabel && (
            <span className="font-heading text-[9px] font-bold tracking-[0.18em] text-on-surface-variant uppercase">
              {subLabel}
            </span>
          )}
        </span>
        <span
          aria-hidden
          className="grid size-7 shrink-0 place-items-center rounded-full border border-outline-variant/40 bg-surface-container-lowest text-on-surface-variant transition-colors group-hover:border-primary/60 group-hover:text-primary"
        >
          <Plus className="h-3.5 w-3.5" />
        </span>
      </button>
    </li>
  )
}

function TemplateResults({
  onStage,
}: {
  onStage: (tpl: Template, foodsById: Map<number, Food>) => void
}) {
  const { t } = useTranslation()
  // Slot scope only: day/week templates have multi-slot structure that
  // doesn't make sense to flatten into a single slot's tray.
  const templates = useTemplates("slot")
  // Fetch foods so we can hydrate template entries into Food objects when
  // staging into the tray. 200 is the existing convention in the planner.
  const foodsQuery = useFoods({ limit: 200 })

  const foodsById = useMemo(() => {
    const map = new Map<number, Food>()
    for (const f of foodsQuery.data?.items ?? []) map.set(f.id, f)
    return map
  }, [foodsQuery.data])

  if (templates.isLoading) {
    return (
      <p className="px-1 py-6 text-[12.5px] text-on-surface-variant">
        {t("common.loading")}
      </p>
    )
  }
  if (templates.isError) {
    return (
      <p
        className="px-1 py-6 text-[12.5px] text-destructive"
        data-testid="tray-error"
        role="alert"
      >
        {t("tray.error_load_templates")}
      </p>
    )
  }
  const items = templates.data ?? []
  if (items.length === 0) {
    return (
      <p
        className="px-1 py-6 text-[12.5px] text-on-surface-variant"
        data-testid="tray-empty"
      >
        {t("tray.empty.templates")}
      </p>
    )
  }
  return (
    <ul className="grid grid-cols-1 gap-1.5" role="list">
      {items.map((tpl) => (
        <li key={tpl.id} role="listitem">
          <button
            type="button"
            onClick={() => onStage(tpl, foodsById)}
            data-testid={`tray-template-${tpl.id}`}
            className="group flex w-full items-center gap-3 rounded-xl border border-transparent bg-surface-container-low/40 px-3 py-2 text-left transition-colors hover:border-outline-variant/40 hover:bg-surface-container-low"
          >
            <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-secondary/15 text-secondary">
              <Bookmark className="h-3.5 w-3.5" aria-hidden />
            </span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-[13px] font-medium text-on-surface">
                {tpl.name}
              </span>
              <span className="font-heading text-[9px] font-bold tracking-[0.18em] text-on-surface-variant uppercase">
                {t("template.components_count", {
                  count: tpl.components.length,
                })}
              </span>
            </span>
            <span
              aria-hidden
              className="grid size-7 shrink-0 place-items-center rounded-full border border-outline-variant/40 bg-surface-container-lowest text-on-surface-variant transition-colors group-hover:border-primary/60 group-hover:text-primary"
            >
              <Plus className="h-3.5 w-3.5" />
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

function TrayFooter({
  items,
  onPortion,
  onRemove,
  onCancel,
  onCommit,
  committing,
}: {
  items: TrayItem[]
  onPortion: (foodId: number, p: number) => void
  onRemove: (foodId: number) => void
  onCancel: () => void
  onCommit: () => void
  committing: boolean
}) {
  const { t } = useTranslation()
  const total = items.length

  // Deferred so changing portions doesn't refetch on every keystroke; the
  // batch endpoint is keyed by the sorted id list and the deferred-tray-item
  // shape stays stable while the user types.
  const deferredItems = useDeferredValue(items)
  const foodIds = useMemo(
    () => deferredItems.map((it) => it.food.id),
    [deferredItems]
  )
  const { data: foodMacrosData } = useFoodMacros(foodIds)
  const macrosByFood = useMemo(() => {
    const map = new Map<number, number>() // food_id → kcal
    for (const entry of foodMacrosData?.foods ?? []) {
      map.set(entry.food_id, entry.macros.kcal)
    }
    return map
  }, [foodMacrosData])
  const runningKcal = useMemo(() => {
    let sum = 0
    for (const it of deferredItems) {
      const k = macrosByFood.get(it.food.id)
      if (k != null) sum += k * it.portions
    }
    return Math.round(sum)
  }, [deferredItems, macrosByFood])

  return (
    <footer className="border-t border-outline-variant/40 bg-surface-container-low/60">
      {items.length > 0 && (
        <div
          className="sticky top-0 z-[1] flex items-center justify-between gap-2 border-b border-outline-variant/30 bg-surface-container-low/80 px-5 py-2 backdrop-blur"
          data-testid="tray-running-total"
        >
          <span className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase">
            {t("tray.running_total")}
          </span>
          <span
            className="font-mono text-[12.5px] font-semibold text-on-surface tabular-nums"
            data-testid="tray-running-kcal"
          >
            {runningKcal} {t("macro.kcal")}
          </span>
        </div>
      )}
      {items.length > 0 && (
        <ul
          className="flex max-h-44 flex-col gap-1 overflow-y-auto px-3 py-2"
          aria-label={t("tray.staged_label")}
          data-testid="tray-staged-list"
        >
          {items.map((it) => (
            <StagedRow
              key={it.food.id}
              item={it}
              onPortion={(p) => onPortion(it.food.id, p)}
              onRemove={() => onRemove(it.food.id)}
            />
          ))}
        </ul>
      )}
      <div className="flex items-center justify-between gap-2 border-t border-outline-variant/30 px-5 py-3">
        <span
          className="font-heading text-[10.5px] font-bold tracking-[0.22em] text-on-surface-variant uppercase"
          data-testid="tray-count"
        >
          {total === 0
            ? t("tray.empty_count")
            : t("tray.staged_count", { count: total })}
        </span>
        <div className="flex items-center gap-1.5">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onCancel}
            data-testid="tray-cancel"
            className="h-8 text-on-surface-variant hover:text-on-surface"
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            size="sm"
            onClick={onCommit}
            disabled={total === 0 || committing}
            data-testid="tray-commit"
            aria-busy={committing}
            className="h-8 gap-1.5"
          >
            {committing ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden />
            ) : (
              <Sparkles className="h-3.5 w-3.5" aria-hidden />
            )}
            {committing
              ? t("tray.committing")
              : t("tray.commit", { count: total })}
          </Button>
        </div>
      </div>
    </footer>
  )
}

function StagedRow({
  item,
  onPortion,
  onRemove,
}: {
  item: TrayItem
  onPortion: (p: number) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const role = item.food.kind === "composed" ? (item.food.role ?? null) : null
  return (
    <li
      data-testid={`tray-staged-${item.food.id}`}
      className="flex items-center gap-2 rounded-lg bg-surface-container-lowest px-2 py-1.5"
    >
      <span className="grid size-7 shrink-0 place-items-center overflow-hidden rounded-md bg-surface-container">
        {item.food.image_path ? (
          <img
            src={imageURL(item.food.image_path)}
            alt=""
            className="h-full w-full object-cover"
          />
        ) : (
          <FoodPlaceholder
            category={(role ?? "main") as FoodPlaceholderCategory}
            size="sm"
            rounded="lg"
            className="h-full w-full"
          />
        )}
      </span>
      <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-on-surface">
        {item.food.name}
      </span>
      <PortionStepper value={item.portions} onChange={onPortion} />
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t("common.remove", { defaultValue: "Remove" })}
        onClick={onRemove}
        className="size-7 text-on-surface-variant hover:text-destructive"
      >
        <X className="h-3.5 w-3.5" />
      </Button>
    </li>
  )
}

function PortionStepper({
  value,
  onChange,
}: {
  value: number
  onChange: (n: number) => void
}) {
  const { t } = useTranslation()
  function bump(delta: number) {
    const next = Math.max(0.25, Math.round((value + delta) * 4) / 4)
    if (next !== value) onChange(next)
  }
  return (
    <div
      className="flex items-center gap-0 rounded-full border border-outline-variant/50 bg-surface-container-lowest"
      role="group"
      aria-label={t("plate.portions")}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={`${t("plate.portions")} −0.25`}
        onClick={() => bump(-0.25)}
        className="size-6 rounded-full text-on-surface-variant"
        disabled={value <= 0.25}
      >
        <Minus className="h-3 w-3" />
      </Button>
      <span className="min-w-[2.2rem] text-center font-mono text-[11.5px] font-semibold text-on-surface tabular-nums">
        ×{formatPortions(value)}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={`${t("plate.portions")} +0.25`}
        onClick={() => bump(0.25)}
        className="size-6 rounded-full text-on-surface-variant"
      >
        <Plus className="h-3 w-3" />
      </Button>
    </div>
  )
}

function formatPortions(n: number): string {
  if (Number.isInteger(n)) return `${n}.0`
  return n.toFixed(2).replace(/0$/, "")
}

// ── Helpers ──────────────────────────────────────────────────────────

function filterFoods(
  foods: Food[],
  search: string,
  roles: Set<FoodRole>
): Food[] {
  const s = search.trim().toLowerCase()
  return foods.filter((f) => {
    if (s && !f.name.toLowerCase().includes(s)) return false
    if (roles.size > 0) {
      if (f.kind !== "composed" || !f.role || !roles.has(f.role)) return false
    }
    return true
  })
}

function slotRolesKey(slotId: number) {
  return `plantry:tray:roles:slot-${slotId}`
}

/** Per-slot role chip selection. Persisted in localStorage so the user's
 * filter survives sheet open/close and reload. */
function useSlotRoleFilter(
  slotId: number
): readonly [Set<FoodRole>, (next: Set<FoodRole>) => void] {
  const [roles, setRoles] = useState<Set<FoodRole>>(() => readRoles(slotId))

  useEffect(() => {
    setRoles(readRoles(slotId))
  }, [slotId])

  const update = useCallback(
    (next: Set<FoodRole>) => {
      setRoles(next)
      try {
        if (next.size === 0) localStorage.removeItem(slotRolesKey(slotId))
        else
          localStorage.setItem(slotRolesKey(slotId), JSON.stringify([...next]))
      } catch {
        // localStorage unavailable; in-memory value still works for the session.
      }
    },
    [slotId]
  )
  return [roles, update] as const
}

function readRoles(slotId: number): Set<FoodRole> {
  try {
    const raw = localStorage.getItem(slotRolesKey(slotId))
    if (!raw) return new Set()
    const arr = JSON.parse(raw) as unknown
    if (!Array.isArray(arr)) return new Set()
    return new Set(arr.filter((x): x is FoodRole => typeof x === "string"))
  } catch {
    return new Set()
  }
}

// ── Tray reducer ─────────────────────────────────────────────────────

export type TrayAction =
  | { type: "stage"; food: Food }
  | { type: "stageTemplate"; template: Template; foodsById: Map<number, Food> }
  | { type: "portion"; foodId: number; p: number }
  | { type: "remove"; foodId: number }
  | { type: "keepOnly"; foodIds: Set<number> }

export function trayReducer(state: TrayItem[], action: TrayAction): TrayItem[] {
  switch (action.type) {
    case "stage": {
      const existing = state.find((i) => i.food.id === action.food.id)
      if (existing) {
        // Re-tapping a result bumps the portion by 0.25 — useful for adding
        // multiple servings of the same item without leaving the result row.
        return state.map((i) =>
          i.food.id === action.food.id
            ? { ...i, portions: clampPortion(i.portions + 0.25) }
            : i
        )
      }
      return [...state, { food: action.food, portions: 1 }]
    }
    case "stageTemplate": {
      let next = state
      for (const tc of action.template.components) {
        const food = action.foodsById.get(tc.food_id)
        if (!food) continue
        const existing = next.find((i) => i.food.id === food.id)
        if (existing) {
          next = next.map((i) =>
            i.food.id === food.id
              ? { ...i, portions: clampPortion(i.portions + tc.portions) }
              : i
          )
        } else {
          next = [...next, { food, portions: clampPortion(tc.portions) }]
        }
      }
      return next
    }
    case "portion":
      return state.map((i) =>
        i.food.id === action.foodId
          ? { ...i, portions: clampPortion(action.p) }
          : i
      )
    case "remove":
      return state.filter((i) => i.food.id !== action.foodId)
    case "keepOnly":
      return state.filter((i) => action.foodIds.has(i.food.id))
    default:
      return state
  }
}

function clampPortion(p: number): number {
  return Math.max(0.25, Math.round(p * 4) / 4)
}

function useTray(): readonly [TrayItem[], (action: TrayAction) => void] {
  const [state, setState] = useState<TrayItem[]>([])
  const dispatch = useCallback(
    (action: TrayAction) => setState((prev) => trayReducer(prev, action)),
    []
  )
  return [state, dispatch] as const
}
