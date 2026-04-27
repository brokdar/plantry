import { useTranslation } from "react-i18next"

import { ComponentPicker } from "@/components/component/ComponentPicker"
import { ApplyTemplateSection } from "@/components/templates/ApplyTemplateSection"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import type { Food } from "@/lib/api/foods"

interface AddComponentSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultRole?: string
  onPick: (c: Food) => void
  showTemplates?: boolean
  defaultSlotId?: string
  defaultDate?: string
}

export function AddComponentSheet({
  open,
  onOpenChange,
  defaultRole,
  onPick,
  showTemplates,
  defaultSlotId,
  defaultDate,
}: AddComponentSheetProps) {
  const { t } = useTranslation()
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-4 sm:max-w-md"
      >
        <SheetHeader>
          <SheetTitle>{t("plate.pick_component")}</SheetTitle>
          <SheetDescription>{t("plate.filter_by_role")}</SheetDescription>
        </SheetHeader>
        <div className="space-y-4 overflow-y-auto px-4 pb-4">
          {showTemplates && (
            <ApplyTemplateSection
              defaultSlotId={defaultSlotId}
              defaultDate={defaultDate}
            />
          )}
          <ComponentPicker defaultRole={defaultRole} onPick={onPick} />
        </div>
      </SheetContent>
    </Sheet>
  )
}
