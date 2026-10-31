import { Link } from "@tanstack/react-router"
import { Check } from "lucide-react"
import { useId, type ComponentProps, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { cn } from "@/lib/utils"

/** The slice of a preset the picker needs. */
export interface RoutinePreset {
  id: number
  name: string
}

export interface RoutinePickerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  presets: RoutinePreset[]
  /** The preset the cell already follows, marked as current. */
  selectedId?: number
  onPick: (presetId: number) => void
  /** Lets the owner return focus to its own control on close. */
  onCloseAutoFocus?: ComponentProps<typeof PopoverContent>["onCloseAutoFocus"]
  /** The element the popover anchors to (it does not toggle it). */
  children: ReactNode
}

/**
 * A popover that lists saved routines (presets). Picking one calls `onPick`;
 * Escape, an outside click or Cancel close it without a pick. With no
 * routines it links to `/presets` to create one.
 */
export function RoutinePicker({
  open,
  onOpenChange,
  presets,
  selectedId,
  onPick,
  onCloseAutoFocus,
  children,
}: RoutinePickerProps) {
  const { t } = useTranslation()
  const titleId = useId()

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      <PopoverContent
        align="start"
        aria-labelledby={titleId}
        onCloseAutoFocus={onCloseAutoFocus}
        className="w-64 gap-1 bg-panel p-1.5 text-ink shadow-sheet ring-line"
      >
        <p id={titleId} className="px-2 pt-1 text-small text-ink-muted">
          {t("availability.picker.title")}
        </p>
        {presets.length > 0 ? (
          <ul className="flex max-h-64 flex-col overflow-y-auto">
            {presets.map((preset) => {
              const selected = preset.id === selectedId
              return (
                <li key={preset.id}>
                  <button
                    type="button"
                    aria-pressed={selected}
                    onClick={() => onPick(preset.id)}
                    className={cn(
                      "flex min-h-11 w-full items-center gap-2 rounded-cell px-2 text-left text-body text-ink pointer-fine:min-h-9",
                      "hover:bg-panel-sunken focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                      selected && "font-semibold"
                    )}
                  >
                    <span className="min-w-0 flex-1 truncate">
                      {preset.name}
                    </span>
                    {selected ? (
                      <Check
                        aria-hidden
                        strokeWidth={1.5}
                        className="size-4 shrink-0 text-accent"
                      />
                    ) : null}
                  </button>
                </li>
              )
            })}
          </ul>
        ) : (
          <div className="flex flex-col gap-1 px-2 py-1">
            <p className="text-body text-ink">
              {t("availability.picker.empty")}
            </p>
            <p className="text-small text-ink-muted">
              {t("availability.picker.empty_hint")}
            </p>
            <Link
              to="/settings/$section"
              params={{ section: "meals" }}
              className="flex min-h-11 items-center self-start rounded-cell text-small font-semibold text-accent underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent pointer-fine:min-h-8"
            >
              {t("availability.picker.create")}
            </Link>
          </div>
        )}
        <div className="flex justify-end border-t border-line pt-1">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
