import { useTranslation } from "react-i18next"

import { ComponentPicker } from "@/components/component/ComponentPicker"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import type { Component } from "@/lib/api/components"

interface AddComponentSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultRole?: string
  onPick: (c: Component) => void
}

export function AddComponentSheet({
  open,
  onOpenChange,
  defaultRole,
  onPick,
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
        <div className="px-4 pb-4">
          <ComponentPicker defaultRole={defaultRole} onPick={onPick} />
        </div>
      </SheetContent>
    </Sheet>
  )
}
