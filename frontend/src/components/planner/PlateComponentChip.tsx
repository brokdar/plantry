import { ArrowLeftRight, X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import type { Food } from "@/lib/api/foods"
import type { PlateComponent } from "@/lib/api/plates"

interface PlateComponentChipProps {
  pc: PlateComponent
  component: Food | undefined
  onSwap: () => void
  onRemove: () => void
}

export function PlateComponentChip({
  pc,
  component,
  onSwap,
  onRemove,
}: PlateComponentChipProps) {
  const { t } = useTranslation()

  return (
    <div
      className="group flex items-center gap-2 rounded-md border border-border bg-card px-2 py-1 text-sm"
      data-testid={`plate-component-${pc.id}`}
    >
      <span className="flex-1 truncate">
        {component?.name ?? `#${pc.food_id}`}
      </span>
      {pc.portions !== 1 && (
        <Badge variant="secondary" className="text-xs">
          ×{pc.portions}
        </Badge>
      )}
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("plate.swap_component")}
        onClick={onSwap}
        className="h-6 w-6"
      >
        <ArrowLeftRight className="h-3 w-3" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("plate.remove_component")}
        onClick={onRemove}
        className="h-6 w-6"
      >
        <X className="h-3 w-3" />
      </Button>
    </div>
  )
}
