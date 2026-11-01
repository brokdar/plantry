import { Sparkles, X } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

interface PanelShellProps {
  /** Below 768 px: a modal bottom sheet; otherwise a plain `aside`. */
  phone: boolean
  /** From 1280 px the `aside` sits beside the board; below that it floats
   *  over it. */
  docked: boolean
  open: boolean
  onClose: () => void
  /** Header controls: history, new conversation. */
  actions: React.ReactNode
  children: React.ReactNode
}

/**
 * The shells of the assistant (D15). From 1280 px an `aside` beside the
 * board; from 768 px a non-modal `aside` floating over it (the board keeps
 * its width): no dialog, no focus trap, no scroll lock; Escape closes it
 * while focus is inside. On phones a bottom sheet with a grab handle.
 */
export function PanelShell({
  phone,
  docked,
  open,
  onClose,
  actions,
  children,
}: PanelShellProps) {
  const { t } = useTranslation()

  if (phone) {
    return (
      <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
        <SheetContent
          side="bottom"
          showCloseButton={false}
          className="gap-0 rounded-t-sheet bg-panel-raised p-0 text-ink shadow-sheet data-[side=bottom]:h-[92dvh]"
          data-testid="assistant-panel"
        >
          <div
            aria-hidden="true"
            className="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-line"
          />
          <SheetHeader className="gap-2 px-4 pt-2 pb-3">
            <div className="flex items-center justify-between gap-2">
              <SheetTitle className="flex items-center gap-2 font-heading text-h3">
                <Sparkles className="size-4" aria-hidden="true" />
                {t("assistant.title")}
              </SheetTitle>
              <CloseButton onClose={onClose} />
            </div>
            <SheetDescription className="text-small text-ink-muted">
              {t("assistant.description")}
            </SheetDescription>
            {actions}
          </SheetHeader>
          {children}
        </SheetContent>
      </Sheet>
    )
  }

  if (!open) return null
  return (
    <aside
      aria-label={t("assistant.title")}
      data-testid="assistant-panel"
      onKeyDown={(e) => {
        // A popover inside (history) takes its own Escape first.
        if (e.key === "Escape" && !e.defaultPrevented) {
          e.preventDefault()
          onClose()
        }
      }}
      className={cn(
        "flex h-svh w-[380px] flex-col border-l border-line bg-panel-raised text-ink",
        docked
          ? "sticky top-0 shrink-0 self-start"
          : "fixed inset-y-0 right-0 z-40 max-w-full shadow-sheet"
      )}
    >
      <header className="flex flex-col gap-2 px-4 pt-4 pb-3">
        <div className="flex items-center justify-between gap-2">
          <h2 className="flex items-center gap-2 font-heading text-h3">
            <Sparkles className="size-4" aria-hidden="true" />
            {t("assistant.title")}
          </h2>
          <CloseButton onClose={onClose} />
        </div>
        <p className="text-small text-ink-muted">
          {t("assistant.description")}
        </p>
        {actions}
      </header>
      {children}
    </aside>
  )
}

function CloseButton({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={onClose}
      aria-label={t("assistant.close")}
      data-testid="assistant-close"
      className="size-11 md:pointer-fine:size-9"
    >
      <X strokeWidth={1.5} aria-hidden="true" />
    </Button>
  )
}
