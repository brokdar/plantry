import { Check, Loader2, Pencil, Plus, Trash2, X } from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import type { Food } from "@/lib/api/foods"
import type { Preset, UpdatePresetInput } from "@/lib/api/presets"
import type { TimeSlot } from "@/lib/api/slots"
import {
  useKnownTags,
  usePatchPreset,
  usePreset,
  useUpdatePreset,
} from "@/lib/queries/presets"
import { toast, toastError } from "@/lib/toast"

interface PresetEditorDrawerProps {
  presetId: number | null
  onClose: () => void
  slotsById: Map<number, TimeSlot>
  foodsById: Map<number, Food>
}

type SaveState = "idle" | "saving" | "saved" | "error"

export function PresetEditorDrawer({
  presetId,
  onClose,
  slotsById,
  foodsById,
}: PresetEditorDrawerProps) {
  const open = presetId !== null
  const presetQuery = usePreset(presetId ?? 0)

  function handleSheetOpenChange(next: boolean) {
    if (!next) onClose()
  }

  const preset = presetQuery.data
  return (
    <Sheet open={open} onOpenChange={handleSheetOpenChange}>
      <SheetContent
        side="right"
        className="w-full sm:max-w-lg"
        data-testid="preset-editor-drawer"
      >
        {presetQuery.isLoading || !preset ? (
          <LoadingSkeleton />
        ) : (
          // Re-keying on preset.id resets every input + saveState when the
          // drawer is opened for a different preset, so we don't need a
          // setState-in-effect to sync local state with prop changes.
          <EditorBody
            key={preset.id}
            preset={preset}
            slotsById={slotsById}
            foodsById={foodsById}
          />
        )}
      </SheetContent>
    </Sheet>
  )
}

function LoadingSkeleton() {
  return (
    <>
      <SheetHeader>
        <SheetTitle> </SheetTitle>
      </SheetHeader>
      <div className="space-y-3 p-6">
        <div className="h-5 w-1/3 animate-pulse rounded bg-muted" />
        <div className="h-12 w-full animate-pulse rounded bg-muted" />
        <div className="h-32 w-full animate-pulse rounded bg-muted" />
      </div>
    </>
  )
}

interface EditorBodyProps {
  preset: Preset
  slotsById: Map<number, TimeSlot>
  foodsById: Map<number, Food>
}

function EditorBody({ preset, slotsById, foodsById }: EditorBodyProps) {
  const { t } = useTranslation()
  const knownTagsQuery = useKnownTags()
  const patchMutation = usePatchPreset()
  const updateMutation = useUpdatePreset()

  const [saveState, setSaveState] = useState<SaveState>("idle")
  const [name, setName] = useState(preset.name)
  const initialNameRef = useRef(preset.name)
  const [tagInput, setTagInput] = useState("")

  function commitNameIfChanged() {
    if (name.trim() === "" || name === initialNameRef.current) {
      if (name.trim() === "" && initialNameRef.current) {
        setName(initialNameRef.current)
      }
      return
    }
    setSaveState("saving")
    patchMutation.mutate(
      { id: preset.id, input: { name: name.trim() } },
      {
        onSuccess: () => {
          initialNameRef.current = name.trim()
          setSaveState("saved")
        },
        onError: (err) => {
          setSaveState("error")
          toastError(err, t)
        },
      }
    )
  }

  function addTag(raw: string) {
    const clean = raw.trim().toLowerCase()
    if (!clean) return
    if (preset.tags.includes(clean)) return
    setSaveState("saving")
    patchMutation.mutate(
      { id: preset.id, input: { add_tags: [clean] } },
      {
        onSuccess: () => {
          setSaveState("saved")
          setTagInput("")
        },
        onError: (err) => {
          setSaveState("error")
          toastError(err, t)
        },
      }
    )
  }

  function removeTag(tag: string) {
    setSaveState("saving")
    patchMutation.mutate(
      { id: preset.id, input: { remove_tags: [tag] } },
      {
        onSuccess: () => setSaveState("saved"),
        onError: (err) => {
          setSaveState("error")
          toastError(err, t)
        },
      }
    )
  }

  function deletePlate(plateIndex: number) {
    const nextPlates = preset.plates
      .filter((_, i) => i !== plateIndex)
      .map((p) => ({
        slot_id: p.slot_id,
        components: p.components.map((c) => ({
          food_id: c.food_id,
          portions: c.portions ?? null,
          amount: c.amount ?? null,
          unit: c.unit ?? null,
          note: c.note ?? null,
        })),
      }))
    if (nextPlates.length === 0) {
      toast.message(
        t("preset.editor.remove_plate") + " — " + t("common.cancel")
      )
      return
    }
    const input: UpdatePresetInput = { plates: nextPlates }
    setSaveState("saving")
    updateMutation.mutate(
      { id: preset.id, input },
      {
        onSuccess: () => setSaveState("saved"),
        onError: (err) => {
          setSaveState("error")
          toastError(err, t)
        },
      }
    )
  }

  const tagSuggestions = useMemo(
    () =>
      (knownTagsQuery.data?.items ?? [])
        .filter((kt) => !preset.tags.includes(kt.tag))
        .slice(0, 6),
    [knownTagsQuery.data, preset.tags]
  )

  return (
    <>
      <SheetHeader>
        <SheetTitle className="flex items-center gap-2">
          <Pencil className="size-4" aria-hidden />
          {preset.name}
        </SheetTitle>
        <SheetDescription className="flex items-center gap-2 text-xs">
          <SaveIndicator state={saveState} />
        </SheetDescription>
      </SheetHeader>

      <div className="space-y-6 overflow-y-auto px-6 pb-6">
        <section className="space-y-2">
          <label
            htmlFor="preset-editor-name"
            className="font-body text-[11px] font-bold tracking-widest text-muted-foreground uppercase"
          >
            {t("preset.name")}
          </label>
          <Input
            id="preset-editor-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={commitNameIfChanged}
            data-testid="preset-editor-name-input"
          />
        </section>

        <section className="space-y-2">
          <label
            htmlFor="preset-editor-tag"
            className="font-body text-[11px] font-bold tracking-widest text-muted-foreground uppercase"
          >
            {t("preset.tags_label")}
          </label>
          {preset.tags.length > 0 && (
            <ul role="list" className="flex flex-wrap gap-1.5">
              {preset.tags.map((tag) => (
                <li key={tag}>
                  <Badge variant="secondary" className="gap-1 pr-1 text-xs">
                    {tag}
                    <button
                      type="button"
                      onClick={() => removeTag(tag)}
                      className="rounded-sm p-0.5 text-muted-foreground hover:text-on-surface"
                      aria-label={`Remove tag ${tag}`}
                    >
                      <X className="size-3" />
                    </button>
                  </Badge>
                </li>
              ))}
            </ul>
          )}
          <Input
            id="preset-editor-tag"
            value={tagInput}
            onChange={(e) => setTagInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === ",") {
                e.preventDefault()
                addTag(tagInput)
              }
            }}
            placeholder={t("preset.tags_placeholder")}
          />
          {tagSuggestions.length > 0 && (
            <ul
              role="list"
              className="flex flex-wrap gap-1.5"
              aria-label="Tag suggestions"
            >
              {tagSuggestions.map((kt) => (
                <li key={kt.tag}>
                  <button
                    type="button"
                    onClick={() => addTag(kt.tag)}
                    className="rounded-full border border-dashed border-border/60 px-2 py-0.5 text-[11px] text-muted-foreground hover:border-primary/40 hover:text-on-surface"
                  >
                    + {kt.tag}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>

        <Separator />

        <section className="space-y-3">
          <h3 className="font-body text-[11px] font-bold tracking-widest text-muted-foreground uppercase">
            {t("preset.plates_count", { count: preset.plates.length })}
          </h3>
          <ul role="list" className="space-y-3">
            {preset.plates.map((plate, i) => {
              const slot = slotsById.get(plate.slot_id)
              return (
                <li
                  key={plate.id}
                  className="rounded-lg border border-border bg-card p-3"
                  data-testid={`preset-editor-plate-${plate.id}`}
                >
                  <div className="mb-2 flex items-center justify-between">
                    <span className="font-body text-[10px] font-bold tracking-widest text-on-surface-variant uppercase">
                      {slot
                        ? t(slot.name_key, { defaultValue: slot.name_key })
                        : `slot #${plate.slot_id}`}
                    </span>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t("preset.editor.remove_plate")}
                      onClick={() => deletePlate(i)}
                      disabled={preset.plates.length === 1}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                  <ul role="list" className="space-y-1">
                    {plate.components.map((c) => {
                      const food = foodsById.get(c.food_id)
                      const qty =
                        c.portions != null
                          ? `× ${c.portions}`
                          : c.amount != null && c.unit
                            ? `${c.amount} ${c.unit}`
                            : ""
                      return (
                        <li
                          key={c.id}
                          className="flex items-center justify-between text-sm"
                        >
                          <span className="text-on-surface">
                            {food?.name ?? `#${c.food_id}`}
                          </span>
                          <span className="font-mono text-[11px] text-muted-foreground">
                            {qty}
                          </span>
                        </li>
                      )
                    })}
                  </ul>
                </li>
              )
            })}
          </ul>
          <Button
            variant="outline"
            size="sm"
            disabled
            className="w-full opacity-60"
          >
            <Plus className="size-3.5" />
            {t("preset.editor.add_plate")}
          </Button>
          <p className="text-[11px] text-muted-foreground">
            {t("preset.editor.pull_from_planner_body")}
          </p>
        </section>
      </div>
    </>
  )
}

function SaveIndicator({ state }: { state: SaveState }) {
  const { t } = useTranslation()
  if (state === "saving") {
    return (
      <>
        <Loader2 className="size-3 animate-spin" />
        <span>{t("preset.editor.saving")}</span>
      </>
    )
  }
  if (state === "saved") {
    return (
      <>
        <Check className="size-3 text-primary" />
        <span>{t("preset.editor.saved")}</span>
      </>
    )
  }
  if (state === "error") {
    return (
      <span className="text-destructive">{t("preset.editor.save_error")}</span>
    )
  }
  return null
}
