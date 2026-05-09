import { format, parseISO } from "date-fns"
import { de as deLocale } from "date-fns/locale"
import {
  Bookmark,
  Clock,
  Heart,
  Loader2,
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
import { toast } from "sonner"

import { PortionStepper } from "@/components/component/PortionStepper"
import {
  QuantityUnitInput,
  type QuantityUnitValue,
} from "@/components/component/QuantityUnitInput"
import {
  categoryForFood,
  FoodPlaceholder,
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
import type { MacrosResponse, PlateComponent } from "@/lib/api/plates"
import type { Template } from "@/lib/api/templates"
import { useFoodMacros, useFoods } from "@/lib/queries/foods"
import { useTemplates } from "@/lib/queries/templates"
import { imageURL } from "@/lib/image-url"
import { slotLabel } from "@/lib/slot-label"
import { cn } from "@/lib/utils"

import {
  DraftPlatePreview,
  type DraftPlatePreviewExistingItem,
} from "./DraftPlatePreview"

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

/**
 * TrayItem carries a kind-aware quantity for a staged food. The `quantity`
 * union mirrors the backend's quantity model (composed → integer portions,
 * leaf → amount + unit) so the tray reducer never has to invent translations.
 */
export type TrayQuantity =
  | { kind: "composed"; portions: number }
  | { kind: "leaf"; amount: number; unit: string }

export interface TrayItem {
  food: Food
  quantity: TrayQuantity
}

/** Commit payload shape sent to the parent. The discriminated union mirrors
 * the backend's add-component endpoint exactly. */
export type TrayCommitItem =
  | { food_id: number; portions: number }
  | { food_id: number; amount: number; unit: string }

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
  /** Components already on the plate the tray is editing. Passed through to
   *  `DraftPlatePreview` so the user can see what they're editing instead of
   *  staging on top of an invisible plate. Empty/omitted when opening on a
   *  fresh slot. */
  existingComponents?: PlateComponent[]
  /** Optional resolver: food_id → Food. Required only when `existingComponents`
   *  is non-empty so the preview can render names + images. The planner
   *  already keeps a foods catalog, so this is cheap to thread through. */
  foodById?: Map<number, Food>
  /** Per-slot kcal target (daily target ÷ slot count). Forwarded to the
   *  preview for the running-total tone treatment. */
  dayKcalTarget?: number | null
  side?: "right" | "bottom"
  onOpenChange: (open: boolean) => void
  onCommit: (
    items: TrayCommitItem[],
    context: TraySlotContext
  ) => Promise<TrayCommitResult | void> | TrayCommitResult | void
}

export function ComponentTraySheet({
  open,
  context,
  recentFoods,
  existingComponents,
  foodById,
  dayKcalTarget,
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
            existingComponents={existingComponents ?? []}
            foodById={foodById}
            dayKcalTarget={dayKcalTarget ?? null}
            previewCollapsible={sheetSide === "bottom"}
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
  existingComponents: PlateComponent[]
  foodById: Map<number, Food> | undefined
  dayKcalTarget: number | null
  previewCollapsible: boolean
  onClose: () => void
  onCommit: ComponentTraySheetProps["onCommit"]
}

function ComponentTraySheetBody({
  context,
  recentFoods,
  existingComponents,
  foodById,
  dayKcalTarget,
  previewCollapsible,
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

  // Hoisted from TrayFooter so the DraftPlatePreview and footer share a
  // single per-food macros query keyed on the staged tray ids. Deferred so
  // typing a quantity doesn't refetch on every keystroke; the batch endpoint
  // is keyed by the sorted id list and the deferred-tray-item shape stays
  // stable while the user types.
  const deferredTray = useDeferredValue(tray)
  const trayFoodIds = useMemo(
    () => deferredTray.map((it) => it.food.id),
    [deferredTray]
  )
  const { data: foodMacrosData } = useFoodMacros(trayFoodIds)
  const macrosByFood = useMemo(() => {
    const map = new Map<number, MacrosResponse>()
    for (const entry of foodMacrosData?.foods ?? []) {
      map.set(entry.food_id, entry.macros)
    }
    return map
  }, [foodMacrosData])

  // Resolve existing PlateComponent rows into the {pc, food} shape the
  // preview consumes. The food map is optional (parents that don't pass it
  // simply get pills without thumbnails / role labels).
  const existingPreviewItems = useMemo<DraftPlatePreviewExistingItem[]>(() => {
    return existingComponents.map((pc) => ({
      pc,
      food: foodById?.get(pc.food_id),
    }))
  }, [existingComponents, foodById])

  // Bump token: a monotonic counter scoped to the most-recently-bumped food
  // id. The DraftPlatePreview reads this and re-keys the matching pill so
  // the enter animation replays — turning the otherwise-invisible re-tap
  // bump into a visible "your bump landed" cue.
  const [bumpToken, setBumpToken] = useState<{
    foodId: number
    nonce: number
  } | null>(null)

  const handleStageFood = useCallback(
    (food: Food) => {
      const alreadyStaged = tray.some((it) => it.food.id === food.id)
      dispatch({ type: "stage", food })
      if (alreadyStaged) {
        setBumpToken((prev) => ({
          foodId: food.id,
          nonce: (prev?.nonce ?? 0) + 1,
        }))
      }
    },
    [dispatch, tray]
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
      const result = await onCommit(tray.map(toCommitItem), context)
      const failed = result?.failedFoodIds ?? []
      if (failed.length === 0) {
        onClose()
      } else {
        // Partial failure: keep only the failed items staged so the user
        // can see what didn't land and retry — closing here would silently
        // drop their intent on top of an already-half-built plate.
        dispatch({ type: "keepOnly", foodIds: new Set(failed) })
        toast.error(t("tray.partial_failure", { count: failed.length }))
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

      <DraftPlatePreview
        items={tray}
        macrosByFood={macrosByFood}
        existing={existingPreviewItems}
        dayKcalTarget={dayKcalTarget}
        collapsible={previewCollapsible}
        bumpToken={bumpToken}
      />

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
        onQuantity={(id, q) => dispatch({ type: "quantity", foodId: id, q })}
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
  const subLabel =
    food.kind === "leaf"
      ? t("ingredient.kind_label")
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
              category={categoryForFood(food)}
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
  onQuantity,
  onRemove,
  onCancel,
  onCommit,
  committing,
}: {
  items: TrayItem[]
  onQuantity: (foodId: number, q: TrayQuantity) => void
  onRemove: (foodId: number) => void
  onCancel: () => void
  onCommit: () => void
  committing: boolean
}) {
  const { t } = useTranslation()
  const total = items.length

  // The running total moved into `DraftPlatePreview` (Phase 4) so the
  // user's plate-being-built is the visual anchor instead of a chip
  // glued to the footer. The footer keeps only the staged-rows editor +
  // commit actions to avoid double-rendering kcal.

  return (
    <footer className="border-t border-outline-variant/40 bg-surface-container-low/60">
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
              onQuantity={(q) => onQuantity(it.food.id, q)}
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
  onQuantity,
  onRemove,
}: {
  item: TrayItem
  onQuantity: (q: TrayQuantity) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  return (
    <li
      data-testid={`tray-staged-${item.food.id}`}
      className="flex flex-col gap-1.5 rounded-lg bg-surface-container-lowest px-2 py-1.5"
    >
      <div className="flex items-center gap-2">
        <span className="grid size-7 shrink-0 place-items-center overflow-hidden rounded-md bg-surface-container">
          {item.food.image_path ? (
            <img
              src={imageURL(item.food.image_path)}
              alt=""
              className="h-full w-full object-cover"
            />
          ) : (
            <FoodPlaceholder
              category={categoryForFood(item.food)}
              size="sm"
              rounded="lg"
              className="h-full w-full"
            />
          )}
        </span>
        <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-on-surface">
          {item.food.name}
        </span>
        {item.quantity.kind === "composed" ? (
          <PortionStepper
            value={item.quantity.portions}
            onChange={(p) => onQuantity({ kind: "composed", portions: p })}
            size="sm"
          />
        ) : null}
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={t("common.remove")}
          onClick={onRemove}
          className="size-7 text-on-surface-variant hover:text-destructive"
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
      {item.quantity.kind === "leaf" && (
        <QuantityUnitInput
          food={item.food}
          value={{
            amount: item.quantity.amount,
            unit: item.quantity.unit,
          }}
          onChange={(q: QuantityUnitValue) =>
            onQuantity({ kind: "leaf", amount: q.amount, unit: q.unit })
          }
          compact
          className="pl-9"
        />
      )}
    </li>
  )
}

/** toCommitItem flattens a staged tray item into the wire shape expected by
 * the parent (and ultimately the backend). The two halves are mutually
 * exclusive — composed never carries amount/unit, leaf never carries
 * portions. Callers cannot accidentally send a mixed payload. */
function toCommitItem(it: TrayItem): TrayCommitItem {
  if (it.quantity.kind === "composed") {
    return { food_id: it.food.id, portions: it.quantity.portions }
  }
  return {
    food_id: it.food.id,
    amount: it.quantity.amount,
    unit: it.quantity.unit,
  }
}

/** initialQuantityFor picks the starting quantity for a fresh tray entry,
 * branching on `food.kind`:
 *  - composed → 1 portion
 *  - leaf with portions table → first portion entry, amount = 1
 *  - leaf without portions → 100 g */
function initialQuantityFor(food: Food): TrayQuantity {
  if (food.kind === "composed") {
    return { kind: "composed", portions: 1 }
  }
  if (food.portions && food.portions.length > 0) {
    return { kind: "leaf", amount: 1, unit: food.portions[0]!.unit }
  }
  return { kind: "leaf", amount: 100, unit: "g" }
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
  | { type: "quantity"; foodId: number; q: TrayQuantity }
  | { type: "remove"; foodId: number }
  | { type: "keepOnly"; foodIds: Set<number> }

export function trayReducer(state: TrayItem[], action: TrayAction): TrayItem[] {
  switch (action.type) {
    case "stage": {
      const existing = state.find((i) => i.food.id === action.food.id)
      if (existing) {
        // Re-tapping a result bumps the staged quantity. The bump rule is
        // kind-aware: composed → +1 portion, leaf-by-count → +1 unit,
        // leaf-by-mass-or-volume → +50 of the current unit.
        return state.map((i) =>
          i.food.id === action.food.id
            ? { ...i, quantity: bumpQuantity(i.quantity) }
            : i
        )
      }
      return [
        ...state,
        { food: action.food, quantity: initialQuantityFor(action.food) },
      ]
    }
    case "stageTemplate": {
      let next = state
      for (const tc of action.template.components) {
        const food = action.foodsById.get(tc.food_id)
        if (!food) continue
        // Templates still speak the legacy float-portions vocabulary. Translate
        // per kind: composed rounds up to ≥1 servings; leaf becomes
        // (portions × 100) g.
        const incoming: TrayQuantity =
          food.kind === "composed"
            ? {
                kind: "composed",
                portions: Math.max(1, Math.round(tc.portions)),
              }
            : {
                kind: "leaf",
                amount: Math.max(1, tc.portions * 100),
                unit: "g",
              }
        const existing = next.find((i) => i.food.id === food.id)
        if (existing) {
          next = next.map((i) =>
            i.food.id === food.id
              ? { ...i, quantity: addQuantities(i.quantity, incoming) }
              : i
          )
        } else {
          next = [...next, { food, quantity: incoming }]
        }
      }
      return next
    }
    case "quantity":
      return state.map((i) =>
        i.food.id === action.foodId ? { ...i, quantity: action.q } : i
      )
    case "remove":
      return state.filter((i) => i.food.id !== action.foodId)
    case "keepOnly":
      return state.filter((i) => action.foodIds.has(i.food.id))
    default:
      return state
  }
}

/** bumpQuantity nudges a staged quantity in response to re-tapping the same
 *  food in the picker. Per-kind:
 *   - composed   → +1 portion
 *   - leaf count → +1 unit (the portion table key)
 *   - leaf grams → +50 g (close enough to a "more" gesture without overshoot)
 *   - leaf other → +100 (volumes / unknown) */
function bumpQuantity(q: TrayQuantity): TrayQuantity {
  if (q.kind === "composed") {
    return { ...q, portions: Math.min(20, q.portions + 1) }
  }
  if (q.unit === "g") {
    return { ...q, amount: q.amount + 50 }
  }
  // Per the spec: count-like (portion table or count units) bump by 1; mass
  // and volume bump by their conventional steps.
  if (q.amount < 5) {
    return { ...q, amount: q.amount + 1 }
  }
  return { ...q, amount: q.amount + 50 }
}

/** addQuantities combines two same-shaped quantities. Used by the template
 *  applier so re-applying a template onto a tray that already has the food
 *  staged does the additive thing. Mismatched shapes (rare; mostly arises
 *  if a template referenced a food that has since changed kind) are
 *  resolved in favour of the incoming entry. */
function addQuantities(a: TrayQuantity, b: TrayQuantity): TrayQuantity {
  if (a.kind === "composed" && b.kind === "composed") {
    return { kind: "composed", portions: Math.min(20, a.portions + b.portions) }
  }
  if (a.kind === "leaf" && b.kind === "leaf" && a.unit === b.unit) {
    return { kind: "leaf", amount: a.amount + b.amount, unit: a.unit }
  }
  return b
}

function useTray(): readonly [TrayItem[], (action: TrayAction) => void] {
  const [state, setState] = useState<TrayItem[]>([])
  const dispatch = useCallback(
    (action: TrayAction) => setState((prev) => trayReducer(prev, action)),
    []
  )
  return [state, dispatch] as const
}
