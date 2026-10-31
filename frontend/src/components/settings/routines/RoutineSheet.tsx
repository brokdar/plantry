import { Plus, Trash2, X } from "lucide-react"
import {
  useDeferredValue,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react"
import { useTranslation } from "react-i18next"

import { QuantityStepper } from "@/components/plan/QuantityStepper"
import { useIsPhone } from "@/components/plan/use-is-phone"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import type { Food } from "@/lib/api/foods"
import type { Preset, UpdatePresetPlateInput } from "@/lib/api/presets"
import type { TimeSlot } from "@/lib/api/slots"
import { useFoods } from "@/lib/queries/foods"
import {
  useCreatePreset,
  useKnownTags,
  usePatchPreset,
  usePreset,
  useUpdatePreset,
} from "@/lib/queries/presets"
import { slotLabel } from "@/lib/slot-label"
import { toast } from "@/lib/toast"
import { cn } from "@/lib/utils"

import { SavedNote } from "../controls"
import { useSavedNote } from "../setting-field"

export type RoutineSheetTarget = { kind: "new" } | { kind: "edit"; id: number }

interface DraftComponent {
  food_id: number
  /** Name of a food added in this sheet; others come from `foodsById`. */
  name?: string
  portions: number | null
  amount: number | null
  unit: string | null
}

interface DraftPlate {
  key: number
  /** 0 until a meal is picked. */
  slot_id: number
  components: DraftComponent[]
}

let nextKey = 1

function draftFrom(preset: Preset): DraftPlate[] {
  return preset.plates.map((p) => ({
    key: nextKey++,
    slot_id: p.slot_id,
    components: p.components.map((c) => ({
      food_id: c.food_id,
      portions: c.portions ?? null,
      amount: c.amount ?? null,
      unit: c.unit ?? null,
    })),
  }))
}

function isComplete(plates: DraftPlate[]) {
  return (
    plates.length > 0 &&
    plates.every((p) => p.slot_id > 0 && p.components.length > 0)
  )
}

function toInput(plates: DraftPlate[]): UpdatePresetPlateInput[] {
  return plates.map((p) => ({
    slot_id: p.slot_id,
    components: p.components.map((c) => ({
      food_id: c.food_id,
      portions: c.portions,
      amount: c.amount,
      unit: c.unit,
    })),
  }))
}

interface RoutineSheetProps {
  target: RoutineSheetTarget | null
  onClose: () => void
  slots: TimeSlot[]
  foodsById: Map<number, Food>
}

/** Creates a routine or edits one: name, tags and its meals (slot plus
 *  foods). Edits save as they happen; a new routine saves on Create. */
export function RoutineSheet({
  target,
  onClose,
  slots,
  foodsById,
}: RoutineSheetProps) {
  const { t } = useTranslation()
  const phone = useIsPhone()
  const editId = target?.kind === "edit" ? target.id : 0
  const preset = usePreset(editId)

  return (
    <Sheet open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <SheetContent
        side={phone ? "bottom" : "right"}
        className={cn(
          "flex flex-col gap-0 bg-panel p-0 text-ink shadow-sheet",
          phone ? "max-h-[90dvh] rounded-t-sheet" : "w-full sm:max-w-md"
        )}
        data-testid="routine-sheet"
        showCloseButton={false}
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
        {target?.kind === "new" ? (
          <NewRoutine slots={slots} foodsById={foodsById} onDone={onClose} />
        ) : preset.data && preset.data.id === editId ? (
          <EditRoutine
            key={preset.data.id}
            preset={preset.data}
            slots={slots}
            foodsById={foodsById}
          />
        ) : (
          <>
            <SheetHeader className="px-4 pt-5 pr-14 pb-3">
              <SheetTitle className="sr-only">
                {t("routines.editor.title_new")}
              </SheetTitle>
              <SheetDescription className="sr-only">
                {t("common.loading")}
              </SheetDescription>
            </SheetHeader>
            <div className="flex flex-col gap-3 p-4">
              <Skeleton className="h-9 w-2/3" />
              <Skeleton className="h-32 w-full" />
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

function NewRoutine({
  slots,
  foodsById,
  onDone,
}: {
  slots: TimeSlot[]
  foodsById: Map<number, Food>
  onDone: () => void
}) {
  const { t } = useTranslation()
  const create = useCreatePreset()
  const [name, setName] = useState("")
  const [tags, setTags] = useState<string[]>([])
  const [plates, setPlates] = useState<DraftPlate[]>(() => [
    { key: nextKey++, slot_id: slots[0]?.id ?? 0, components: [] },
  ])
  const ready = name.trim() !== "" && isComplete(plates)

  function submit() {
    if (!ready) return
    create.mutate(
      { name: name.trim(), tags, plates: toInput(plates) },
      {
        onSuccess: () => {
          toast.success(t("routines.created"))
          onDone()
        },
      }
    )
  }

  return (
    <form
      className="flex min-h-0 flex-1 flex-col"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <SheetHeader className="px-4 pt-5 pr-14 pb-3">
        <SheetTitle className="font-heading text-h2 font-normal text-ink">
          {t("routines.editor.title_new")}
        </SheetTitle>
        <SheetDescription className="text-small text-ink-muted">
          {t("routines.editor.description_new")}
        </SheetDescription>
      </SheetHeader>
      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-4 pb-4">
        <NameField value={name} onChange={setName} autoFocus />
        <TagsField
          tags={tags}
          onAdd={(tag) => setTags((prev) => [...prev, tag])}
          onRemove={(tag) => setTags((prev) => prev.filter((x) => x !== tag))}
        />
        <PlatesEditor
          plates={plates}
          onChange={setPlates}
          slots={slots}
          foodsById={foodsById}
        />
      </div>
      <div className="border-t border-line px-4 py-3">
        <Button
          type="submit"
          className="w-full pointer-coarse:h-11"
          disabled={!ready || create.isPending}
        >
          {t("routines.editor.create")}
        </Button>
      </div>
    </form>
  )
}

function EditRoutine({
  preset,
  slots,
  foodsById,
}: {
  preset: Preset
  slots: TimeSlot[]
  foodsById: Map<number, Food>
}) {
  const { t } = useTranslation()
  const patch = usePatchPreset()
  // Scoped per routine: its full-replacement PUTs run one at a time, in
  // order, so the last change always lands last.
  const update = useUpdatePreset(preset.id)
  const { saved, markSaved } = useSavedNote()
  const [name, setName] = useState(preset.name)
  const [savedName, setSavedName] = useState(preset.name)
  const [plates, setPlates] = useState(() => draftFrom(preset))
  // The typed name and the last name sent, for the flush on close.
  const draftName = useRef(preset.name)
  const sentName = useRef(preset.name)

  function sendName(onSaved?: (name: string) => void) {
    const next = draftName.current.trim()
    if (next === "" || next === sentName.current) return
    sentName.current = next
    patch.mutate(
      { id: preset.id, input: { name: next } },
      { onSuccess: () => onSaved?.(next) }
    )
  }

  function commitName() {
    if (name.trim() === "") {
      draftName.current = sentName.current
      setName(sentName.current)
      return
    }
    sendName((next) => {
      setSavedName(next)
      markSaved()
    })
  }

  // Closing the sheet (Escape, outside click) unmounts it, possibly without
  // a blur: send a pending rename then.
  const flushName = useRef(sendName)
  useEffect(() => {
    flushName.current = sendName
  })
  useEffect(() => () => flushName.current(), [])

  function changePlates(next: DraftPlate[]) {
    setPlates(next)
    if (!isComplete(next)) return
    update.mutate(
      { id: preset.id, input: { plates: toInput(next) } },
      { onSuccess: markSaved }
    )
  }

  return (
    <>
      <SheetHeader className="px-4 pt-5 pr-14 pb-3">
        <SheetTitle className="font-heading text-h2 font-normal text-ink">
          {savedName}
        </SheetTitle>
        <SheetDescription className="flex flex-wrap items-center gap-2 text-small text-ink-muted">
          {t("routines.editor.description")}
          <SavedNote saved={saved} />
        </SheetDescription>
      </SheetHeader>
      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-4 pb-6">
        <NameField
          value={name}
          onChange={(v) => {
            draftName.current = v
            setName(v)
          }}
          onCommit={commitName}
        />
        <TagsField
          tags={preset.tags}
          onAdd={(tag) =>
            patch.mutate(
              { id: preset.id, input: { add_tags: [tag] } },
              { onSuccess: markSaved }
            )
          }
          onRemove={(tag) =>
            patch.mutate(
              { id: preset.id, input: { remove_tags: [tag] } },
              { onSuccess: markSaved }
            )
          }
        />
        <PlatesEditor
          plates={plates}
          onChange={changePlates}
          slots={slots}
          foodsById={foodsById}
        />
      </div>
    </>
  )
}

function NameField({
  value,
  onChange,
  onCommit,
  autoFocus,
}: {
  value: string
  onChange: (v: string) => void
  /** Saves the name, on blur and on Enter. */
  onCommit?: () => void
  autoFocus?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-small text-ink-muted">
        {t("routines.name")}
      </label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onBlur={onCommit}
        onKeyDown={(e) => {
          if (e.key === "Enter" && onCommit) {
            e.preventDefault()
            onCommit()
          }
        }}
        autoFocus={autoFocus}
        className="pointer-coarse:h-11"
      />
    </div>
  )
}

function TagsField({
  tags,
  onAdd,
  onRemove,
}: {
  tags: string[]
  onAdd: (tag: string) => void
  onRemove: (tag: string) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const known = useKnownTags()
  const [input, setInput] = useState("")
  const suggestions = (known.data?.items ?? [])
    .filter((kt) => !tags.includes(kt.tag))
    .slice(0, 6)

  function add(raw: string) {
    const tag = raw.trim().toLowerCase()
    setInput("")
    if (tag && !tags.includes(tag)) onAdd(tag)
  }

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-small text-ink-muted">
        {t("routines.tags_label")}
      </label>
      {tags.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {tags.map((tag) => (
            <li
              key={tag}
              className="inline-flex items-center gap-0.5 rounded-cell border border-line bg-panel-sunken pl-2 text-small text-ink"
            >
              {tag}
              <button
                type="button"
                onClick={() => onRemove(tag)}
                aria-label={t("routines.remove_tag", { tag })}
                className="grid size-11 place-items-center rounded-cell text-ink-muted hover:text-ink focus-visible:outline-2 focus-visible:outline-accent md:pointer-fine:size-7"
              >
                <X className="size-3.5" strokeWidth={1.5} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}
      <Input
        id={id}
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === ",") {
            e.preventDefault()
            add(input)
          }
        }}
        placeholder={t("routines.tags_placeholder")}
        className="pointer-coarse:h-11"
      />
      {suggestions.length > 0 && (
        <ul
          aria-label={t("routines.tag_suggestions")}
          className="flex flex-wrap gap-1.5"
        >
          {suggestions.map((kt) => (
            <li key={kt.tag}>
              <button
                type="button"
                onClick={() => add(kt.tag)}
                className="min-h-11 rounded-cell border border-dashed border-line px-2 text-small text-ink-muted hover:text-ink focus-visible:outline-2 focus-visible:outline-accent md:pointer-fine:min-h-7"
              >
                + {kt.tag}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function PlatesEditor({
  plates,
  onChange,
  slots,
  foodsById,
}: {
  plates: DraftPlate[]
  onChange: (next: DraftPlate[]) => void
  slots: TimeSlot[]
  foodsById: Map<number, Food>
}) {
  const { t } = useTranslation()

  function setPlate(key: number, patch: (p: DraftPlate) => DraftPlate) {
    onChange(plates.map((p) => (p.key === key ? patch(p) : p)))
  }

  function addMeal() {
    const used = new Set(plates.map((p) => p.slot_id))
    const slot = slots.find((s) => !used.has(s.id)) ?? slots[0]
    onChange([
      ...plates,
      { key: nextKey++, slot_id: slot?.id ?? 0, components: [] },
    ])
  }

  return (
    <section className="flex flex-col gap-3">
      <ul className="flex flex-col gap-3">
        {plates.map((plate) => (
          <li key={plate.key}>
            <PlateEditor
              plate={plate}
              slots={slots}
              foodsById={foodsById}
              canRemove={plates.length > 1}
              onChange={(patch) => setPlate(plate.key, patch)}
              onRemove={() =>
                onChange(plates.filter((p) => p.key !== plate.key))
              }
            />
          </li>
        ))}
      </ul>
      <Button
        type="button"
        variant="outline"
        onClick={addMeal}
        className="self-start pointer-coarse:h-11"
      >
        <Plus strokeWidth={1.5} aria-hidden />
        {t("routines.editor.add_meal")}
      </Button>
    </section>
  )
}

function PlateEditor({
  plate,
  slots,
  foodsById,
  canRemove,
  onChange,
  onRemove,
}: {
  plate: DraftPlate
  slots: TimeSlot[]
  foodsById: Map<number, Food>
  canRemove: boolean
  onChange: (patch: (p: DraftPlate) => DraftPlate) => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const selectId = useId()
  const [adding, setAdding] = useState(false)
  const slot = slots.find((s) => s.id === plate.slot_id)
  const meal = slot
    ? slotLabel(t, slot.name_key)
    : t("routines.editor.meal_label")

  function setComponent(index: number, next: DraftComponent | null) {
    onChange((p) => ({
      ...p,
      components: next
        ? p.components.map((c, i) => (i === index ? next : c))
        : p.components.filter((_, i) => i !== index),
    }))
  }

  return (
    <div
      className="flex flex-col gap-3 rounded-cell border border-line bg-panel p-3"
      data-testid="routine-meal"
    >
      <div className="flex items-end gap-2">
        <div className="flex flex-1 flex-col gap-1.5">
          <label htmlFor={selectId} className="text-small text-ink-muted">
            {t("routines.editor.meal_label")}
          </label>
          <select
            id={selectId}
            value={plate.slot_id || ""}
            onChange={(e) =>
              onChange((p) => ({ ...p, slot_id: Number(e.target.value) }))
            }
            className="h-9 rounded-md border border-line bg-panel px-2 text-body text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent pointer-coarse:h-11"
          >
            {!slot && <option value="" disabled />}
            {slots.map((s) => (
              <option key={s.id} value={s.id}>
                {slotLabel(t, s.name_key)}
              </option>
            ))}
          </select>
        </div>
        <Button
          type="button"
          variant="ghost"
          className="size-11 text-ink-muted md:pointer-fine:size-9"
          aria-label={t("routines.editor.remove_meal", { meal })}
          disabled={!canRemove}
          onClick={onRemove}
        >
          <Trash2 strokeWidth={1.5} aria-hidden />
        </Button>
      </div>

      {plate.components.length > 0 ? (
        <ul className="flex flex-col divide-y divide-line">
          {plate.components.map((c, i) => (
            <li key={`${c.food_id}-${i}`}>
              <ComponentRow
                component={c}
                name={
                  c.name ?? foodsById.get(c.food_id)?.name ?? `#${c.food_id}`
                }
                onChange={(next) => setComponent(i, next)}
              />
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-small text-ink-muted">
          {t("routines.editor.empty_meal")}
        </p>
      )}

      {adding ? (
        <FoodSearch
          onPick={(food) => {
            setAdding(false)
            onChange((p) => ({
              ...p,
              components: [
                ...p.components,
                food.kind === "composed"
                  ? {
                      food_id: food.id,
                      name: food.name,
                      portions: 1,
                      amount: null,
                      unit: null,
                    }
                  : {
                      food_id: food.id,
                      name: food.name,
                      portions: null,
                      amount: 100,
                      unit: "g",
                    },
              ],
            }))
          }}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <Button
          type="button"
          variant="ghost"
          onClick={() => setAdding(true)}
          className="self-start text-accent pointer-coarse:h-11"
        >
          <Plus strokeWidth={1.5} aria-hidden />
          {t("routines.editor.add_food")}
        </Button>
      )}
    </div>
  )
}

function ComponentRow({
  component,
  name,
  onChange,
}: {
  component: DraftComponent
  name: string
  onChange: (next: DraftComponent | null) => void
}) {
  const { t } = useTranslation()
  const unit = component.unit ?? "g"
  return (
    <div className="flex flex-wrap items-center gap-2 py-2">
      <span className="min-w-0 flex-1 truncate text-body text-ink">{name}</span>
      {component.portions !== null ? (
        <QuantityStepper
          label={t("routines.editor.portions_of", { name })}
          value={component.portions}
          min={0.25}
          max={20}
          step={0.25}
          onChange={(portions) => onChange({ ...component, portions })}
        />
      ) : (
        <span className="flex items-center gap-1.5">
          <Input
            key={component.amount}
            type="number"
            inputMode="decimal"
            min={0}
            step="any"
            defaultValue={component.amount ?? ""}
            aria-label={t("routines.editor.amount_of", { name, unit })}
            className="w-20 text-right tabular-nums pointer-coarse:h-11"
            onBlur={(e) => {
              const amount = Number(e.target.value)
              if (amount > 0 && amount !== component.amount) {
                onChange({ ...component, amount })
              } else {
                e.target.value = String(component.amount ?? "")
              }
            }}
          />
          <span className="text-small text-ink-muted">{unit}</span>
        </span>
      )}
      <Button
        type="button"
        variant="ghost"
        className="size-11 text-ink-muted md:pointer-fine:size-8"
        aria-label={t("routines.editor.remove_food", { name })}
        onClick={() => onChange(null)}
      >
        <X strokeWidth={1.5} aria-hidden />
      </Button>
    </div>
  )
}

function FoodSearch({
  onPick,
  onCancel,
}: {
  onPick: (food: Food) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [query, setQuery] = useState("")
  const search = useDeferredValue(query.trim())
  const params = useMemo(
    () => ({ search: search || undefined, sort: "name", limit: 8 }),
    [search]
  )
  const foods = useFoods(params)
  const items = foods.data?.items ?? []

  return (
    <div className="flex flex-col gap-1.5">
      <Input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.stopPropagation()
            onCancel()
          }
        }}
        aria-label={t("routines.editor.food_search")}
        placeholder={t("routines.editor.food_search_placeholder")}
        autoFocus
        className="pointer-coarse:h-11"
      />
      {foods.isSuccess && items.length === 0 ? (
        <p className="px-2 py-2 text-small text-ink-muted">
          {t("routines.editor.no_foods")}
        </p>
      ) : (
        <ul className="flex flex-col">
          {items.map((food) => (
            <li key={food.id}>
              <button
                type="button"
                onClick={() => onPick(food)}
                className="flex min-h-11 w-full items-center rounded-cell px-2 text-left text-body text-ink hover:bg-panel-sunken focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent md:pointer-fine:min-h-9"
              >
                {food.name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
