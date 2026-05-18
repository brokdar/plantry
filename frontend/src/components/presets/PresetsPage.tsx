import { Bookmark, Search, Sprout, X } from "lucide-react"
import { useDeferredValue, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
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
import { toast, toastError } from "@/lib/toast"

import { PresetCard } from "./PresetCard"
import { PresetEditorDrawer } from "./PresetEditorDrawer"
import { PresetRenameDialog } from "./PresetRenameDialog"

export function PresetsPage() {
  const { t } = useTranslation()

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [selectedSlotIDs, setSelectedSlotIDs] = useState<number[]>([])
  const [selectedTags, setSelectedTags] = useState<string[]>([])

  const presetsQuery = usePresets({
    search: deferredSearch || undefined,
    slot_ids: selectedSlotIDs.length > 0 ? selectedSlotIDs : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
  })
  const knownTagsQuery = useKnownTags()
  const slotsQuery = useTimeSlots(true)
  const foodsQuery = useFoods({ limit: 500, offset: 0 })

  const foodsById = useMemo(
    () =>
      new Map((foodsQuery.data?.items ?? []).map((f) => [f.id, f] as const)),
    [foodsQuery.data?.items]
  )
  const slotsById = useMemo(
    () =>
      new Map((slotsQuery.data?.items ?? []).map((s) => [s.id, s] as const)),
    [slotsQuery.data]
  )

  const [editorPresetId, setEditorPresetId] = useState<number | null>(null)
  const [renameTarget, setRenameTarget] = useState<Preset | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Preset | null>(null)

  const renameMutation = useUpdatePreset()
  const duplicateMutation = useDuplicatePreset()
  const deleteMutation = useDeletePreset()

  function toggleSlot(slotId: number) {
    setSelectedSlotIDs((prev) =>
      prev.includes(slotId)
        ? prev.filter((id) => id !== slotId)
        : [...prev, slotId]
    )
  }
  function toggleTag(tag: string) {
    setSelectedTags((prev) =>
      prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag]
    )
  }
  function clearFilters() {
    setSearch("")
    setSelectedSlotIDs([])
    setSelectedTags([])
  }

  const list = presetsQuery.data?.items ?? []
  const total = presetsQuery.data?.total ?? 0
  const filtersActive =
    deferredSearch.length > 0 ||
    selectedSlotIDs.length > 0 ||
    selectedTags.length > 0
  const isEmpty = !presetsQuery.isLoading && list.length === 0 && !filtersActive
  const isNoResult =
    !presetsQuery.isLoading && list.length === 0 && filtersActive

  return (
    <div
      className="mx-auto max-w-6xl space-y-8 px-4 py-8 md:px-8 md:py-12"
      data-testid="presets-page"
    >
      <header className="flex flex-col items-start justify-between gap-6 md:flex-row md:items-end">
        <div className="max-w-2xl space-y-2">
          <span className="font-body text-xs font-bold tracking-widest text-primary uppercase">
            {t("preset.eyebrow")}
          </span>
          <h1 className="font-heading text-4xl leading-tight font-extrabold tracking-tight text-on-surface md:text-5xl">
            {t("preset.title")}
          </h1>
          <p className="max-w-lg text-base leading-relaxed text-on-surface-variant md:text-lg">
            {t("preset.subtitle")}
          </p>
        </div>
        {presetsQuery.data && (
          <Badge variant="outline" className="text-xs">
            {t("preset.plates_count", {
              count: total,
              defaultValue: `${total}`,
            })}
          </Badge>
        )}
      </header>

      <section
        className="sticky top-0 z-10 -mx-4 space-y-3 border-b border-border/40 bg-surface/95 px-4 py-4 backdrop-blur md:-mx-8 md:px-8"
        data-testid="preset-filters"
      >
        <div className="relative">
          <Search
            aria-hidden
            className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("preset.search_placeholder")}
            className="pl-9"
            data-testid="preset-search"
          />
        </div>

        {(slotsQuery.data?.items ?? []).length > 0 && (
          <div
            role="group"
            aria-label={t("preset.filter_slot_label")}
            className="flex flex-wrap gap-1.5"
          >
            {(slotsQuery.data?.items ?? []).map((slot) => {
              const active = selectedSlotIDs.includes(slot.id)
              return (
                <button
                  key={slot.id}
                  type="button"
                  onClick={() => toggleSlot(slot.id)}
                  aria-pressed={active}
                  className={
                    "font-body rounded-full border px-3 py-1 text-[11px] font-bold tracking-widest uppercase transition-colors " +
                    (active
                      ? "border-primary bg-primary text-on-primary"
                      : "border-border bg-card text-on-surface-variant hover:border-primary/40")
                  }
                >
                  {t(slot.name_key, { defaultValue: slot.name_key })}
                </button>
              )
            })}
          </div>
        )}

        {(knownTagsQuery.data?.items ?? []).length > 0 && (
          <div
            role="group"
            aria-label={t("preset.filter_tags_label")}
            className="flex flex-wrap gap-1.5"
          >
            {(knownTagsQuery.data?.items ?? []).map((kt) => {
              const active = selectedTags.includes(kt.tag)
              return (
                <button
                  key={kt.tag}
                  type="button"
                  onClick={() => toggleTag(kt.tag)}
                  aria-pressed={active}
                  className={
                    "rounded-full border px-2.5 py-0.5 text-xs transition-colors " +
                    (active
                      ? "border-primary bg-primary/15 text-primary"
                      : "border-border bg-card text-muted-foreground hover:border-primary/40 hover:text-on-surface")
                  }
                >
                  {kt.tag}
                  <span className="ml-1 text-[10px] tabular-nums opacity-60">
                    {kt.count}
                  </span>
                </button>
              )
            })}
          </div>
        )}

        {filtersActive && (
          <Button
            variant="ghost"
            size="sm"
            onClick={clearFilters}
            className="h-7 px-2 text-xs text-muted-foreground"
          >
            <X className="size-3" />
            {t("preset.no_results_clear")}
          </Button>
        )}
      </section>

      {presetsQuery.isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-44 w-full rounded-lg" />
          ))}
        </div>
      ) : isEmpty ? (
        <EmptyState />
      ) : isNoResult ? (
        <NoResultsState onClear={clearFilters} />
      ) : (
        <ul
          role="list"
          className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
          data-testid="preset-grid"
        >
          {list.map((preset) => (
            <li key={preset.id}>
              <PresetCard
                preset={preset}
                foodsById={foodsById}
                slotsById={slotsById}
                onOpen={(p) => setEditorPresetId(p.id)}
                onRename={(p) => setRenameTarget(p)}
                onDuplicate={(p) => {
                  duplicateMutation.mutate(p.id, {
                    onSuccess: (created) =>
                      toast.success(
                        t("preset.duplicated", { name: created.name })
                      ),
                    onError: (err) => toastError(err, t),
                  })
                }}
                onDelete={(p) => setDeleteTarget(p)}
              />
            </li>
          ))}
        </ul>
      )}

      <PresetEditorDrawer
        presetId={editorPresetId}
        onClose={() => setEditorPresetId(null)}
        slotsById={slotsById}
        foodsById={foodsById}
      />

      {renameTarget && (
        <PresetRenameDialog
          open={renameTarget !== null}
          onOpenChange={(o) => !o && setRenameTarget(null)}
          defaultName={renameTarget.name}
          onSubmit={(name) =>
            renameMutation.mutate(
              { id: renameTarget.id, input: { name } },
              {
                onSuccess: () => {
                  toast.success(t("preset.renamed"))
                  setRenameTarget(null)
                },
                onError: (err) => toastError(err, t),
              }
            )
          }
          pending={renameMutation.isPending}
        />
      )}

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(o) => !o && setDeleteTarget(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("preset.delete_confirm_title", {
                name: deleteTarget?.name ?? "",
              })}
            </DialogTitle>
            <DialogDescription>
              {t("preset.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (!deleteTarget) return
                deleteMutation.mutate(deleteTarget.id, {
                  onSuccess: () => {
                    toast.success(t("preset.deleted"))
                    setDeleteTarget(null)
                  },
                  onError: (err) => toastError(err, t),
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

function EmptyState() {
  const { t } = useTranslation()
  return (
    <div
      className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border bg-muted/40 px-8 py-16 text-center"
      data-testid="preset-empty"
    >
      <div className="rounded-full border border-border/60 bg-background p-3">
        <Sprout className="size-6 text-primary" aria-hidden="true" />
      </div>
      <div className="space-y-1">
        <p className="text-base font-medium">{t("preset.empty_title")}</p>
        <p className="max-w-sm text-sm text-muted-foreground">
          {t("preset.empty_body")}
        </p>
      </div>
      <Button asChild variant="outline" size="sm">
        <a href="/">
          <Bookmark className="size-3.5" />
          {t("preset.empty_cta")}
        </a>
      </Button>
    </div>
  )
}

function NoResultsState({ onClear }: { onClear: () => void }) {
  const { t } = useTranslation()
  return (
    <div
      className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border bg-muted/40 px-8 py-12 text-center"
      data-testid="preset-no-results"
    >
      <p className="text-base font-medium">{t("preset.no_results_title")}</p>
      <Button variant="outline" size="sm" onClick={onClear}>
        {t("preset.no_results_clear")}
      </Button>
    </div>
  )
}
