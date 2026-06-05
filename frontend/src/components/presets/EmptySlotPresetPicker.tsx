import { Bookmark, Search } from "lucide-react"
import { useDeferredValue, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import type { Preset } from "@/lib/api/presets"
import { useApplyPreset, usePresets, useUndoApply } from "@/lib/queries/presets"
import { showPresetApplyToasts } from "@/lib/preset-apply-toast"

interface EmptySlotPresetPickerProps {
  slotId: number
  slotLabel: string
  targetDate: string
  trigger: React.ReactNode
}

export function EmptySlotPresetPicker({
  slotId,
  slotLabel,
  targetDate,
  trigger,
}: EmptySlotPresetPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const deferred = useDeferredValue(search)

  const presetsQuery = usePresets({
    slot_ids: [slotId],
    search: deferred || undefined,
    sort: "recent",
    limit: 20,
  })
  const applyMutation = useApplyPreset()
  const undoMutation = useUndoApply()

  const items = presetsQuery.data?.items ?? []

  function handleApply(preset: Preset) {
    applyMutation.mutate(
      {
        presetId: preset.id,
        input: {
          target_date: targetDate,
          on_conflict: "skip",
          slot_ids_filter: [slotId],
        },
      },
      {
        onSuccess: (result) => {
          showPresetApplyToasts(result, t, (snap) => undoMutation.mutate(snap))
          setOpen(false)
        },
      }
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        align="start"
        sideOffset={6}
        className="w-80 p-0"
        data-testid="empty-slot-preset-picker"
      >
        <div className="space-y-2 border-b border-border/40 p-3">
          <p className="font-body text-[10px] font-bold tracking-widest text-on-surface-variant uppercase">
            {t("preset.empty_slot_picker.title", { slot: slotLabel })}
          </p>
          <div className="relative">
            <Search
              aria-hidden
              className="absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              autoFocus
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("preset.search_placeholder")}
              className="h-8 pl-7 text-sm"
            />
          </div>
        </div>

        <div
          className="max-h-72 overflow-y-auto p-2"
          data-testid="empty-slot-preset-list"
        >
          {presetsQuery.isLoading ? (
            <p className="px-2 py-4 text-center text-xs text-muted-foreground">
              …
            </p>
          ) : items.length === 0 ? (
            <p className="px-2 py-4 text-center text-xs text-muted-foreground">
              {t("preset.empty_slot_picker.no_matches")}
            </p>
          ) : (
            <ul role="list" className="space-y-1">
              {items.map((preset) => (
                <li key={preset.id}>
                  <button
                    type="button"
                    onClick={() => handleApply(preset)}
                    disabled={applyMutation.isPending}
                    className="group flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-surface-container-low focus-visible:bg-surface-container-low focus-visible:outline-none"
                  >
                    <span className="flex items-center gap-1.5 truncate">
                      <Bookmark className="size-3.5 text-muted-foreground group-hover:text-primary" />
                      <span className="truncate text-on-surface">
                        {preset.name}
                      </span>
                    </span>
                    {preset.tags.length > 0 && (
                      <span className="hidden gap-1 sm:flex">
                        {preset.tags.slice(0, 2).map((tag) => (
                          <Badge
                            key={tag}
                            variant="outline"
                            className="text-[10px] font-normal"
                          >
                            {tag}
                          </Badge>
                        ))}
                      </span>
                    )}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
