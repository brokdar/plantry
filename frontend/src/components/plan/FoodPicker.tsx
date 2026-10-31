import { Check, Search, X } from "lucide-react"
import { useDeferredValue, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  defaultQuantityValueForFood,
  QuantityUnitInput,
  type QuantityUnitValue,
} from "@/components/component/QuantityUnitInput"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Food, FoodListParams, FoodRole } from "@/lib/api/foods"
import type { ComponentInput } from "@/lib/api/plates"
import { formatNumber, type Formatter } from "@/lib/plan/format"
import { useFood, useFoods } from "@/lib/queries/foods"
import {
  useAddComponent,
  useCreatePlate,
  useUpdateComponent,
} from "@/lib/queries/plates"
import { cn } from "@/lib/utils"

import { QuantityStepper } from "./QuantityStepper"

/** What the picker does with the chosen foods. */
export type PickerTarget =
  /** An empty cell: the staged foods become one new plate. */
  | { kind: "create"; date: string; slotId: number }
  /** An existing plate: the staged foods are added to it. */
  | { kind: "add"; plateId: number }
  /** One component: the first food picked replaces it. */
  | { kind: "swap"; plateId: number; componentId: number; name: string }

type PickerTab = "recipes" | "ingredients" | "favorites" | "recent"

const TABS: PickerTab[] = ["recipes", "ingredients", "favorites", "recent"]

const ROLES: FoodRole[] = [
  "main",
  "side_starch",
  "side_veg",
  "side_protein",
  "sauce",
  "dessert",
  "drink",
]

const LIMIT = 50

/** The staged quantity: portions per eater, or a leaf amount and unit. */
type StagedQuantity = { portions: number } | QuantityUnitValue

interface Staged {
  food: Food
  quantity: StagedQuantity
}

function listParams(
  tab: PickerTab,
  search: string,
  role: FoodRole | null
): FoodListParams {
  const q = search.trim() || undefined
  switch (tab) {
    case "recipes":
      return {
        kind: "composed",
        search: q,
        role: role ?? undefined,
        limit: LIMIT,
      }
    case "ingredients":
      return { kind: "leaf", search: q, limit: LIMIT }
    case "favorites":
      return { favorite: 1, search: q, limit: LIMIT }
    case "recent":
      return {
        kind: "composed",
        search: q,
        sort: "last_eaten",
        order: "desc",
        limit: LIMIT,
      }
  }
}

function toInput({ food, quantity }: Staged): ComponentInput {
  return "portions" in quantity
    ? { food_id: food.id, portions: quantity.portions }
    : { food_id: food.id, amount: quantity.amount, unit: quantity.unit }
}

interface FoodPickerProps {
  target: PickerTarget
  /** The plate (or swap) was written. */
  onDone: () => void
  /** Back without writing anything. */
  onCancel: () => void
}

/**
 * One sheet to find foods (UX §5.5): search, tabs for Recipes, Ingredients,
 * Favorites and Recent, and role chips for recipes. Tapping a row stages it
 * with a quantity; the staged foods are written in one go, so a new plate
 * is a single `POST /api/plates {components}`. In swap mode a tap replaces
 * the component at once.
 */
export function FoodPicker({ target, onDone, onCancel }: FoodPickerProps) {
  const { t, i18n } = useTranslation()
  const [tab, setTab] = useState<PickerTab>("recipes")
  const [search, setSearch] = useState("")
  const [role, setRole] = useState<FoodRole | null>(null)
  const [staged, setStaged] = useState<Staged[]>([])
  const deferred = useDeferredValue(search)
  const foods = useFoods(listParams(tab, deferred, role))
  const createPlate = useCreatePlate()
  const addComponent = useAddComponent()
  const updateComponent = useUpdateComponent()
  const busy =
    createPlate.isPending || addComponent.isPending || updateComponent.isPending

  const items = (foods.data?.items ?? []).filter(
    (f) => tab !== "recent" || (f.kind === "composed" && f.last_eaten)
  )
  const swap = target.kind === "swap"

  function toggle(food: Food) {
    if (target.kind === "swap") {
      updateComponent.mutate(
        {
          plateId: target.plateId,
          componentId: target.componentId,
          input: { food_id: food.id },
        },
        { onSuccess: onDone }
      )
      return
    }
    setStaged((list) =>
      list.some((s) => s.food.id === food.id)
        ? list.filter((s) => s.food.id !== food.id)
        : [
            ...list,
            {
              food,
              quantity:
                food.kind === "composed"
                  ? { portions: 1 }
                  : defaultQuantityValueForFood(food),
            },
          ]
    )
  }

  function setQuantity(foodId: number, quantity: StagedQuantity) {
    setStaged((list) =>
      list.map((s) => (s.food.id === foodId ? { ...s, quantity } : s))
    )
  }

  async function commit() {
    if (target.kind === "create") {
      createPlate.mutate(
        {
          date: target.date,
          slot_id: target.slotId,
          components: staged.map(toInput),
        },
        { onSuccess: onDone }
      )
    } else if (target.kind === "add") {
      try {
        for (const item of staged) {
          await addComponent.mutateAsync({
            plateId: target.plateId,
            input: toInput(item),
          })
          // Saved: a retry after a later failure must not add it again.
          setStaged((list) => list.filter((s) => s.food.id !== item.food.id))
        }
        onDone()
      } catch {
        // Toasted by the mutation; the unsaved foods stay for another try.
      }
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="food-picker">
      <div className="flex flex-col gap-3 px-4 pb-3">
        <label className="relative block">
          <span className="sr-only">{t("plan.picker.search")}</span>
          <Search
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-muted"
            strokeWidth={1.5}
            aria-hidden
          />
          <Input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("plan.picker.search")}
            className="h-11 pl-9 text-body md:pointer-fine:h-9"
            data-testid="food-picker-search"
            autoFocus
          />
        </label>
        <Tabs value={tab} onValueChange={(v) => setTab(v as PickerTab)}>
          <TabsList variant="line" className="w-full">
            {TABS.map((id) => (
              <TabsTrigger
                key={id}
                value={id}
                className="min-h-11 md:pointer-fine:min-h-8"
              >
                {t(`plan.picker.tab_${id}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {tab === "recipes" && (
          <div
            role="group"
            aria-label={t("component.role")}
            className="-mx-4 flex gap-1.5 overflow-x-auto px-4"
          >
            {[null, ...ROLES].map((r) => (
              <button
                key={r ?? "all"}
                type="button"
                aria-pressed={role === r}
                onClick={() => setRole(r)}
                className={cn(
                  "inline-flex min-h-11 shrink-0 items-center rounded-full border px-3 text-small transition-colors duration-150 ease-out focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-panel focus-visible:outline-none md:pointer-fine:min-h-8",
                  role === r
                    ? "border-accent bg-accent text-on-accent"
                    : "border-line text-ink hover:bg-panel-sunken"
                )}
              >
                {r ? t(`component.role_${r}`) : t("plan.picker.role_all")}
              </button>
            ))}
          </div>
        )}
      </div>

      <ul
        className="min-h-0 flex-1 overflow-y-auto border-t border-line"
        aria-label={t("plan.picker.results")}
        aria-busy={foods.isFetching || undefined}
      >
        {items.map((food) => {
          const picked = staged.some((s) => s.food.id === food.id)
          return (
            <li key={food.id}>
              <button
                type="button"
                onClick={() => toggle(food)}
                disabled={busy}
                aria-pressed={swap ? undefined : picked}
                data-testid="food-picker-row"
                className={cn(
                  "flex min-h-11 w-full items-center gap-3 px-4 py-2 text-left transition-colors duration-150 ease-out hover:bg-panel-sunken focus-visible:bg-panel-sunken focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-accent",
                  picked && "bg-panel-sunken"
                )}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-body text-ink">
                    {food.name}
                  </span>
                  <span className="block truncate text-caption text-ink-muted">
                    {foodMeta(food, i18n)}
                  </span>
                </span>
                {picked && (
                  <Check
                    className="size-4 text-accent"
                    strokeWidth={2}
                    aria-hidden
                  />
                )}
              </button>
            </li>
          )
        })}
        {foods.isSuccess && items.length === 0 && (
          <li className="px-4 py-6 text-small text-ink-muted">
            {t("plan.picker.empty")}
          </li>
        )}
      </ul>

      {swap ? (
        <div className="border-t border-line p-4">
          <Button
            type="button"
            variant="outline"
            className="h-11 w-full md:pointer-fine:h-9"
            onClick={onCancel}
          >
            {t("common.back")}
          </Button>
        </div>
      ) : (
        <div className="flex flex-col gap-3 border-t border-line p-4">
          {staged.length > 0 && (
            <ul
              className="flex max-h-64 flex-col gap-3 overflow-y-auto"
              aria-label={t("plan.picker.staged")}
              data-testid="food-picker-staged"
            >
              {staged.map((s) => (
                <StagedRow
                  key={s.food.id}
                  staged={s}
                  onQuantity={(q) => setQuantity(s.food.id, q)}
                  onRemove={() => toggle(s.food)}
                />
              ))}
            </ul>
          )}
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              className="h-11 md:pointer-fine:h-9"
              onClick={onCancel}
            >
              {t("common.back")}
            </Button>
            <Button
              type="button"
              className="h-11 flex-1 md:pointer-fine:h-9"
              disabled={staged.length === 0 || busy}
              onClick={() => void commit()}
              data-testid="food-picker-commit"
            >
              {t("plan.picker.commit", { count: staged.length })}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

function foodMeta(food: Food, f: Formatter): string {
  const { t } = f
  if (food.kind === "composed") {
    const role = food.role ? t(`component.role_${food.role}`) : null
    const kcal = t("plan.picker.kcal_portion", {
      kcal: formatNumber(food.per_portion.kcal, f),
    })
    return role ? `${role}, ${kcal}` : kcal
  }
  return food.kcal_100g != null
    ? t("plan.picker.kcal_100g", { kcal: formatNumber(food.kcal_100g, f) })
    : t("plan.picker.ingredient")
}

function StagedRow({
  staged,
  onQuantity,
  onRemove,
}: {
  staged: Staged
  onQuantity: (q: StagedQuantity) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const { food, quantity } = staged
  // The list omits a leaf's portion table; the detail has its units.
  const detail = useFood(food.kind === "leaf" ? food.id : 0)
  return (
    <li className="flex flex-col gap-1.5" data-testid="food-picker-staged-row">
      <div className="flex items-center gap-2">
        <span className="min-w-0 flex-1 truncate text-small font-semibold text-ink">
          {food.name}
        </span>
        <Button
          type="button"
          variant="ghost"
          className="size-11 md:pointer-fine:size-8"
          aria-label={t("plan.picker.unstage", { name: food.name })}
          onClick={onRemove}
        >
          <X strokeWidth={1.5} aria-hidden />
        </Button>
      </div>
      {"portions" in quantity ? (
        <QuantityStepper
          label={t("plan.sheet.portions_per_eater_of", { name: food.name })}
          value={quantity.portions}
          min={0.25}
          max={10}
          step={0.25}
          onChange={(portions) => onQuantity({ portions })}
        />
      ) : (
        <QuantityUnitInput
          food={detail.data ?? food}
          value={quantity}
          onChange={onQuantity}
          compact
        />
      )}
    </li>
  )
}
