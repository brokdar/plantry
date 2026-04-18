import { BookmarkPlus, MoreVertical, Plus, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Component } from "@/lib/api/components"
import type { Plate } from "@/lib/api/plates"

import { PlateComponentChip } from "./PlateComponentChip"
import { PlateFeedbackBar } from "./PlateFeedbackBar"

interface PlateCellProps {
  plate: Plate | undefined
  weekId: number
  componentsById: Map<number, Component>
  onAdd: () => void
  onSwap: (pcId: number, currentRole?: string) => void
  onRemoveComponent: (pcId: number) => void
  onDeletePlate: () => void
  onSaveAsTemplate?: () => void
}

export function PlateCell({
  plate,
  weekId,
  componentsById,
  onAdd,
  onSwap,
  onRemoveComponent,
  onDeletePlate,
  onSaveAsTemplate,
}: PlateCellProps) {
  const { t } = useTranslation()

  if (!plate) {
    return (
      <button
        type="button"
        onClick={onAdd}
        aria-label={t("plate.empty_cell")}
        className="group flex min-h-24 w-full flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed border-outline-variant/30 text-on-surface-variant/40 transition-all hover:border-primary/40 hover:bg-surface-container-low"
      >
        <Plus className="size-5 group-hover:text-primary" aria-hidden />
        <span className="text-[9px] font-bold tracking-wider uppercase">
          {t("plate.empty_cell")}
        </span>
      </button>
    )
  }

  return (
    <div className="editorial-shadow flex min-h-24 min-w-0 flex-col gap-2 overflow-hidden rounded-xl bg-surface-container-lowest p-2.5">
      <div className="flex items-center justify-between">
        <Button
          variant="ghost"
          size="sm"
          onClick={onAdd}
          className="h-6 px-2 text-xs"
        >
          <Plus className="h-3 w-3" />
          {t("plate.add_component")}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              aria-label={t("common.actions")}
            >
              <MoreVertical className="h-3 w-3" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {onSaveAsTemplate && (
              <DropdownMenuItem onClick={onSaveAsTemplate}>
                <BookmarkPlus className="h-3 w-3" />
                {t("template.save_as")}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem
              onClick={onDeletePlate}
              className="text-destructive"
            >
              <Trash2 className="h-3 w-3" />
              {t("plate.delete_plate")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="flex flex-col gap-1">
        {plate.components.map((pc) => {
          const c = componentsById.get(pc.component_id)
          return (
            <PlateComponentChip
              key={pc.id}
              pc={pc}
              component={c}
              onSwap={() => onSwap(pc.id, c?.role)}
              onRemove={() => onRemoveComponent(pc.id)}
            />
          )
        })}
      </div>
      <PlateFeedbackBar plate={plate} weekId={weekId} />
    </div>
  )
}
