import { X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { ImportWizard } from "@/components/import/ImportWizard"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

/** The import as a full-height sheet over the library (`/recipes?add=url`,
 *  UX §2.2). Saving navigates to the new recipe; closing drops `add`. */
export function ImportSheet({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      <SheetContent
        side="right"
        showCloseButton={false}
        className="flex w-full flex-col gap-0 bg-panel p-0 text-ink shadow-sheet sm:max-w-2xl data-[side=right]:sm:max-w-2xl"
        data-testid="import-sheet"
      >
        <SheetClose asChild>
          <Button
            type="button"
            variant="ghost"
            className="absolute top-3 right-3 size-11 md:pointer-fine:size-8"
            aria-label={t("common.close")}
            data-testid="import-sheet-close"
          >
            <X strokeWidth={1.5} aria-hidden />
          </Button>
        </SheetClose>
        <SheetHeader className="border-b border-line px-4 pt-5 pr-14 pb-3 md:px-6">
          <SheetTitle className="font-heading text-h2 font-normal text-ink">
            {t("import.title")}
          </SheetTitle>
          <SheetDescription className="text-small text-ink-muted">
            {t("import.subtitle")}
          </SheetDescription>
        </SheetHeader>
        <div className="flex-1 overflow-y-auto px-4 py-5 md:px-6">
          <ImportWizard />
        </div>
      </SheetContent>
    </Sheet>
  )
}
