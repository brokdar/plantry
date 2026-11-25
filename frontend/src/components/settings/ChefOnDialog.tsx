import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { LabellingEstimate } from "@/lib/api/chef"
import { useLabellingEstimate } from "@/lib/queries/chef"

import { formatUSD } from "./insights/format"

interface ChefOnDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  pending: boolean
  onConfirm: () => void
}

/**
 * Turning the Chef on labels the library once (ON-2): the dialog shows
 * what that labels, roughly costs and takes before the switch turns on.
 */
export function ChefOnDialog({
  open,
  onOpenChange,
  pending,
  onConfirm,
}: ChefOnDialogProps) {
  const { t } = useTranslation()
  const estimate = useLabellingEstimate(open)
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false} data-testid="chef-on-dialog">
        <DialogHeader>
          <DialogTitle>
            {t("settings_page.assistant.chef_on_title")}
          </DialogTitle>
          <DialogDescription>
            {t("settings_page.assistant.chef_on_body")}
          </DialogDescription>
        </DialogHeader>
        <p
          className="text-small text-ink"
          aria-live="polite"
          data-testid="chef-on-estimate"
        >
          {estimate.data ? (
            <EstimateText estimate={estimate.data} />
          ) : estimate.isError ? (
            t("settings_page.assistant.chef_on_error")
          ) : (
            t("settings_page.assistant.chef_on_loading")
          )}
        </p>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="h-11 md:pointer-fine:h-9"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            className="h-11 md:pointer-fine:h-9"
            disabled={pending || !estimate.data}
            onClick={onConfirm}
          >
            {t("settings_page.assistant.chef_on_confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function EstimateText({ estimate }: { estimate: LabellingEstimate }) {
  const { t, i18n } = useTranslation()
  if (estimate.recipes === 0) {
    return t("settings_page.assistant.chef_on_nothing")
  }
  if (estimate.cost_usd === null) {
    return t("settings_page.assistant.chef_on_unpriced", {
      count: estimate.recipes,
    })
  }
  return (
    <>
      {t("settings_page.assistant.chef_on_estimate", {
        count: estimate.recipes,
        cost: formatUSD(estimate.cost_usd, i18n),
        minutes: estimate.minutes,
      })}{" "}
      {t("settings_page.assistant.chef_on_background")}
    </>
  )
}
