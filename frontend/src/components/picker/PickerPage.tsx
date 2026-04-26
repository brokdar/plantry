import { ChevronLeft, Search, UtensilsCrossed } from "lucide-react"
import { useDeferredValue, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { ComposedFood, Food, FoodRole } from "@/lib/api/foods"
import { useFoods, useSetFoodFavorite } from "@/lib/queries/foods"
import { useAddPlateComponent, useSetPlateSkipped } from "@/lib/queries/plates"
import { findPlateAt } from "@/lib/queries/plate-patches"
import { useTimeSlots } from "@/lib/queries/slots"
import { useCreatePlate, useWeek } from "@/lib/queries/weeks"
import { slotLabel } from "@/lib/slot-label"
import { toastError, toast } from "@/lib/toast"
import { ApplyTemplateSection } from "@/components/templates/ApplyTemplateSection"

import { ComposingTray, type TrayItem } from "./ComposingTray"
import { PickerCard } from "./PickerCard"
import { PickerFilters, type PickerPreset } from "./PickerFilters"

interface PickerPageProps {
  weekId: number
  day: number
  slotId: number
  onBack: () => void
}

const PRESET_ROLES: Record<PickerPreset, string[] | undefined> = {
  all: undefined,
  favorites: undefined,
  recents: undefined,
  mains: ["main"],
  sides: ["side_starch", "side_veg", "side_protein"],
  snacks: ["standalone"],
  sauces: ["sauce"],
}

export function PickerPage({ weekId, day, slotId, onBack }: PickerPageProps) {
  const { t } = useTranslation()

  const slotsQuery = useTimeSlots(true)
  const slot = slotsQuery.data?.items.find((s) => s.id === slotId)

  // The picker still needs the current week to know whether a plate already
  // exists at (day, slot). If it does, Save adds to it; otherwise it creates.
  // We avoid piping the year/week through the URL by pulling the week by id
  // from the existing cache/query layer.
  const week = useWeekFromCache(weekId)
  const existingPlate = week ? findPlateAt(week, day, slotId) : undefined

  const [kindTab, setKindTab] = useState<"composed" | "leaf">("composed")
  const [query, setQuery] = useState("")
  const [preset, setPreset] = useState<PickerPreset>("all")
  const [roleFilter, setRoleFilter] = useState<string>("all")
  const [sort, setSort] = useState<"name" | "recent" | "kcal">("name")
  const [tray, setTray] = useState<TrayItem[]>([])

  const deferredQuery = useDeferredValue(query)

  const componentsQuery = useFoods({
    kind: kindTab,
    limit: 200,
    search: deferredQuery || undefined,
    favorite: preset === "favorites" ? 1 : undefined,
    role:
      kindTab === "composed" && roleFilter !== "all"
        ? (roleFilter as FoodRole)
        : undefined,
    sort:
      sort === "kcal"
        ? "kcal"
        : sort === "recent" && kindTab === "composed"
          ? "last_cooked_at"
          : "name",
    order: sort === "recent" && kindTab === "composed" ? "desc" : "asc",
  })

  const items = useMemo(() => {
    const raw = componentsQuery.data?.items ?? []
    if (kindTab === "leaf") return raw
    const composed = raw.filter((c): c is ComposedFood => c.kind === "composed")
    const presetRoles = PRESET_ROLES[preset]
    if (presetRoles)
      return composed.filter(
        (c) => c.role != null && presetRoles.includes(c.role)
      )
    return composed
  }, [componentsQuery.data, preset, kindTab])

  const counts = useMemo(() => {
    const raw = componentsQuery.data?.items ?? []
    const composed = raw.filter((c): c is ComposedFood => c.kind === "composed")
    return {
      all: raw.length,
      favorites: raw.filter((c) => c.favorite).length,
      recents: raw.filter((c) => c.last_cooked_at).length,
      mains: composed.filter((c) => c.role === "main").length,
      sides: composed.filter(
        (c) =>
          c.role === "side_starch" ||
          c.role === "side_veg" ||
          c.role === "side_protein"
      ).length,
      snacks: composed.filter((c) => c.role === "standalone").length,
      sauces: composed.filter((c) => c.role === "sauce").length,
    }
  }, [componentsQuery.data])

  const createPlateMut = useCreatePlate(weekId)
  const addCompMut = useAddPlateComponent(weekId)
  const setSkippedMut = useSetPlateSkipped(weekId)
  const favoriteMut = useSetFoodFavorite()
  function addToTray(component: Food) {
    setTray((prev) =>
      prev.some((t) => t.component.id === component.id)
        ? prev
        : [...prev, { component, portions: 1 }]
    )
  }

  function changePortion(id: number, portions: number) {
    setTray((prev) =>
      prev.map((it) => (it.component.id === id ? { ...it, portions } : it))
    )
  }

  function removeFromTray(id: number) {
    setTray((prev) => prev.filter((it) => it.component.id !== id))
  }

  async function handleSave() {
    if (tray.length === 0) return
    try {
      let plateId = existingPlate?.id
      if (!plateId) {
        const created = await createPlateMut.mutateAsync({
          day,
          slot_id: slotId,
          components: tray.map((it) => ({
            food_id: it.component.id,
            portions: it.portions,
          })),
        })
        plateId = created.id
      } else {
        for (const it of tray) {
          await addCompMut.mutateAsync({
            plateId,
            input: {
              food_id: it.component.id,
              portions: it.portions,
            },
          })
        }
      }
      toast.success(t("picker.tray.saved", { defaultValue: "Plate saved" }))
      onBack()
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleMarkSkip() {
    try {
      let plateId = existingPlate?.id
      if (!plateId) {
        const created = await createPlateMut.mutateAsync({
          day,
          slot_id: slotId,
        })
        plateId = created.id
      }
      await setSkippedMut.mutateAsync({
        plateId,
        input: { skipped: true, note: existingPlate?.note ?? null },
      })
      onBack()
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleToggleFavorite(component: Food) {
    try {
      await favoriteMut.mutateAsync({
        id: component.id,
        favorite: !component.favorite,
      })
    } catch (err) {
      toastError(err, t)
    }
  }

  const slotName = slot ? slotLabel(t, slot.name_key) : ""
  const roleOptions = useMemo(() => {
    const set = new Set<string>()
    for (const c of componentsQuery.data?.items ?? []) {
      if (c.kind === "composed" && c.role) set.add(c.role)
    }
    return Array.from(set).sort()
  }, [componentsQuery.data])

  return (
    <div className="mx-auto grid max-w-7xl gap-6 px-4 py-8 md:grid-cols-[1fr_360px] md:px-8 md:py-12">
      <section className="editorial-shadow min-w-0 rounded-[22px] border border-outline-variant/50 bg-surface-container-lowest p-6 md:p-8">
        <div
          className="mb-5 flex items-center gap-3.5 rounded-[14px] bg-primary-fixed px-4 py-3"
          data-testid="picker-target"
        >
          <button
            type="button"
            onClick={onBack}
            aria-label={t("common.back", { defaultValue: "Back" })}
            className="grid size-9 place-items-center rounded-full bg-white/80 text-primary hover:bg-white"
          >
            <ChevronLeft className="h-4 w-4" aria-hidden />
          </button>
          <div className="flex-1">
            <p className="font-heading text-[11px] font-bold tracking-[0.16em] text-on-primary-fixed/70 uppercase">
              {t("picker.target", { defaultValue: "Planning" })} · {slotName}
            </p>
            <p className="font-heading text-[16px] font-bold text-on-primary-fixed">
              {t("picker.target_sub", {
                defaultValue: "Pick what's on the plate",
              })}
            </p>
          </div>
        </div>

        {/* Kind tabs */}
        <div className="mb-4 flex gap-1 rounded-xl bg-surface-container-low p-1">
          {(
            [
              {
                key: "composed",
                label: t("picker.tab.recipes", { defaultValue: "Rezepte" }),
              },
              {
                key: "leaf",
                label: t("picker.tab.foods", { defaultValue: "Lebensmittel" }),
              },
            ] as const
          ).map(({ key, label }) => (
            <button
              key={key}
              type="button"
              data-testid={`picker-tab-${key}`}
              onClick={() => {
                setKindTab(key)
                setPreset("all")
                setRoleFilter("all")
                if (key === "leaf" && sort === "recent") setSort("name")
              }}
              className={`flex-1 rounded-lg py-2 font-heading text-[13px] font-semibold transition-colors ${kindTab === key ? "bg-white text-on-surface shadow-sm" : "text-on-surface-variant hover:text-on-surface"}`}
            >
              {label}
            </button>
          ))}
        </div>

        <label className="mb-4 flex items-center gap-3 rounded-xl bg-surface-container-low px-4 py-3">
          <Search className="h-4 w-4 text-on-surface-variant" aria-hidden />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("picker.search.placeholder")}
            data-testid="picker-search"
            className="flex-1 bg-transparent text-[14px] text-on-surface outline-none placeholder:text-on-surface-variant"
          />
        </label>

        {kindTab === "composed" && (
          <div className="mb-4">
            <PickerFilters
              value={preset}
              onChange={setPreset}
              counts={counts}
              onSkipShortcut={handleMarkSkip}
              canSkip={!existingPlate?.skipped}
            />
          </div>
        )}

        <div className="mb-5 grid grid-cols-2 gap-2.5 md:grid-cols-3">
          {kindTab === "composed" && (
            <FilterSelect
              testid="picker-role-filter"
              value={roleFilter}
              onChange={setRoleFilter}
              options={[
                { value: "all", label: t("picker.role.all") },
                ...roleOptions.map((r) => ({
                  value: r,
                  label: t(`planner.slot.role.${r}`, { defaultValue: r }),
                })),
              ]}
            />
          )}
          <FilterSelect
            testid="picker-sort"
            value={sort}
            onChange={(v) => setSort(v as typeof sort)}
            options={
              kindTab === "leaf"
                ? [
                    { value: "name", label: t("picker.sort.name") },
                    { value: "kcal", label: t("picker.sort.kcal") },
                  ]
                : [
                    { value: "name", label: t("picker.sort.name") },
                    { value: "recent", label: t("picker.sort.recent") },
                    { value: "kcal", label: t("picker.sort.kcal") },
                  ]
            }
          />
        </div>

        {kindTab === "composed" && !existingPlate && <ApplyTemplateSection />}

        {items.length === 0 ? (
          <div className="grid place-items-center rounded-[14px] border border-dashed border-outline-variant/50 py-20 text-center text-on-surface-variant">
            <div className="space-y-2">
              <UtensilsCrossed
                className="mx-auto h-8 w-8 opacity-40"
                aria-hidden
              />
              <p className="text-[13px]">
                {kindTab === "leaf"
                  ? t("picker.empty_leaf", {
                      defaultValue: "No foods match this search.",
                    })
                  : t("picker.empty", {
                      defaultValue: "No components match this filter.",
                    })}
              </p>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2 lg:grid-cols-3">
            {items.map((c) => (
              <PickerCard
                key={c.id}
                component={c}
                onPick={() => addToTray(c)}
                onToggleFavorite={() => void handleToggleFavorite(c)}
              />
            ))}
          </div>
        )}
      </section>

      <ComposingTray
        items={tray}
        onPortionChange={changePortion}
        onRemove={removeFromTray}
        onSave={handleSave}
        onCancel={onBack}
        saving={createPlateMut.isPending || addCompMut.isPending}
      />
    </div>
  )
}

interface FilterSelectProps<T extends string> {
  testid: string
  value: T
  onChange: (v: T) => void
  options: { value: T; label: string }[]
}

function FilterSelect<T extends string>({
  testid,
  value,
  onChange,
  options,
}: FilterSelectProps<T>) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as T)}>
      <SelectTrigger data-testid={testid} className="h-10">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function useWeekFromCache(weekId: number) {
  return useWeek(weekId).data
}
