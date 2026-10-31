import { Plus, Search } from "lucide-react"
import { useDeferredValue, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import type { Preset } from "@/lib/api/presets"
import { useFoods } from "@/lib/queries/foods"
import {
  useDeletePreset,
  useDuplicatePreset,
  useKnownTags,
  usePresets,
  useUpdatePreset,
} from "@/lib/queries/presets"
import { useTimeSlots } from "@/lib/queries/slots"
import { slotLabel } from "@/lib/slot-label"
import { toast } from "@/lib/toast"
import { cn } from "@/lib/utils"

import { RoutineCard } from "./RoutineCard"
import { RoutineRenameDialog } from "./RoutineRenameDialog"
import { RoutineSheet, type RoutineSheetTarget } from "./RoutineSheet"

const FOODS_PARAMS = { limit: 500, offset: 0 } as const

const chipClass =
  "min-h-11 rounded-cell border px-3 text-small transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent md:pointer-fine:min-h-8"

function chipState(active: boolean) {
  return active
    ? "border-accent bg-accent text-on-accent"
    : "border-line bg-panel text-ink hover:bg-panel-sunken"
}

/** Settings → Meals & week: create, edit, rename, duplicate and delete
 *  routines (plan D5; the backend calls them presets). */
export function RoutinesEditor() {
  const { t } = useTranslation()
  const [now] = useState(() => Date.now())

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search.trim())
  const [slotIds, setSlotIds] = useState<number[]>([])
  const [tags, setTags] = useState<string[]>([])

  const presets = usePresets({
    search: deferredSearch || undefined,
    slot_ids: slotIds.length > 0 ? slotIds : undefined,
    tags: tags.length > 0 ? tags : undefined,
    sort: "name",
  })
  const knownTags = useKnownTags()
  const slotsQuery = useTimeSlots()
  const foods = useFoods(FOODS_PARAMS)

  const allSlots = useMemo(
    () =>
      [...(slotsQuery.data?.items ?? [])].sort(
        (a, b) => a.sort_order - b.sort_order
      ),
    [slotsQuery.data]
  )
  const slots = allSlots.filter((s) => s.active)
  const slotsById = useMemo(
    () =>
      new Map((slotsQuery.data?.items ?? []).map((s) => [s.id, s] as const)),
    [slotsQuery.data]
  )
  const foodsById = useMemo(
    () => new Map((foods.data?.items ?? []).map((f) => [f.id, f] as const)),
    [foods.data]
  )

  const [sheet, setSheet] = useState<RoutineSheetTarget | null>(null)
  const [renameTarget, setRenameTarget] = useState<Preset | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Preset | null>(null)
  const rename = useUpdatePreset()
  const duplicate = useDuplicatePreset()
  const remove = useDeletePreset()

  function toggle<T>(list: T[], value: T) {
    return list.includes(value)
      ? list.filter((v) => v !== value)
      : [...list, value]
  }
  function clearFilters() {
    setSearch("")
    setSlotIds([])
    setTags([])
  }

  const list = presets.data?.items ?? []
  const filtered =
    deferredSearch !== "" || slotIds.length > 0 || tags.length > 0
  const tagItems = knownTags.data?.items ?? []

  return (
    <div className="flex flex-col gap-4" data-testid="routines-editor">
      <div className="flex flex-col gap-2 sm:flex-row">
        <div className="relative flex-1">
          <Search
            aria-hidden
            strokeWidth={1.5}
            className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-muted"
          />
          <Input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label={t("routines.search_label")}
            placeholder={t("routines.search_placeholder")}
            className="pl-9 pointer-coarse:h-11"
          />
        </div>
        <Button
          type="button"
          onClick={() => setSheet({ kind: "new" })}
          className="pointer-coarse:h-11"
        >
          <Plus strokeWidth={1.5} aria-hidden />
          {t("routines.new")}
        </Button>
      </div>

      {(slots.length > 1 || tagItems.length > 0) && (
        <div className="flex flex-col gap-2">
          {slots.length > 1 && (
            <div
              role="group"
              aria-label={t("routines.filter_meal_label")}
              className="flex flex-wrap gap-1.5"
            >
              {slots.map((slot) => {
                const active = slotIds.includes(slot.id)
                return (
                  <button
                    key={slot.id}
                    type="button"
                    aria-pressed={active}
                    onClick={() => setSlotIds((prev) => toggle(prev, slot.id))}
                    className={cn(chipClass, chipState(active))}
                  >
                    {slotLabel(t, slot.name_key)}
                  </button>
                )
              })}
            </div>
          )}
          {tagItems.length > 0 && (
            <div
              role="group"
              aria-label={t("routines.filter_tags_label")}
              className="flex flex-wrap gap-1.5"
            >
              {tagItems.map(({ tag, count }) => {
                const active = tags.includes(tag)
                return (
                  <button
                    key={tag}
                    type="button"
                    aria-pressed={active}
                    onClick={() => setTags((prev) => toggle(prev, tag))}
                    className={cn(chipClass, chipState(active))}
                  >
                    {tag}
                    <span className="ml-1 tabular-nums opacity-70">
                      {count}
                    </span>
                  </button>
                )
              })}
            </div>
          )}
        </div>
      )}

      {presets.isLoading ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-28 w-full" />
        </div>
      ) : list.length === 0 ? (
        <div className="flex flex-col items-start gap-3 rounded-cell border border-dashed border-line p-4">
          {filtered ? (
            <>
              <p className="text-body text-ink-muted">
                {t("routines.no_results")}
              </p>
              <Button type="button" variant="outline" onClick={clearFilters}>
                {t("routines.clear_filters")}
              </Button>
            </>
          ) : (
            <>
              <p className="text-body font-semibold text-ink">
                {t("routines.empty_title")}
              </p>
              <p className="text-small text-ink-muted">
                {t("routines.empty_body")}
              </p>
            </>
          )}
        </div>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2" aria-live="polite">
          {list.map((routine) => (
            <li key={routine.id}>
              <RoutineCard
                routine={routine}
                foodsById={foodsById}
                slotsById={slotsById}
                now={now}
                onOpen={() => setSheet({ kind: "edit", id: routine.id })}
                onRename={() => setRenameTarget(routine)}
                onDuplicate={() =>
                  duplicate.mutate(routine.id, {
                    onSuccess: (created) =>
                      toast.success(
                        t("routines.duplicated", { name: created.name })
                      ),
                  })
                }
                onDelete={() => setDeleteTarget(routine)}
              />
            </li>
          ))}
        </ul>
      )}

      <RoutineSheet
        target={sheet}
        onClose={() => setSheet(null)}
        slots={allSlots}
        foodsById={foodsById}
      />

      {renameTarget && (
        <RoutineRenameDialog
          open
          onOpenChange={(open) => !open && setRenameTarget(null)}
          defaultName={renameTarget.name}
          pending={rename.isPending}
          onSubmit={(name) =>
            rename.mutate(
              { id: renameTarget.id, input: { name } },
              {
                onSuccess: () => {
                  toast.success(t("routines.renamed"))
                  setRenameTarget(null)
                },
              }
            )
          }
        />
      )}

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <DialogContent className="bg-panel text-ink">
          <DialogHeader>
            <DialogTitle className="text-h3">
              {t("routines.delete_confirm_title", {
                name: deleteTarget?.name ?? "",
              })}
            </DialogTitle>
            <DialogDescription className="text-small text-ink-muted">
              {t("routines.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => {
                if (!deleteTarget) return
                remove.mutate(deleteTarget.id, {
                  onSuccess: () => {
                    toast.success(t("routines.deleted"))
                    setDeleteTarget(null)
                  },
                })
              }}
            >
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
