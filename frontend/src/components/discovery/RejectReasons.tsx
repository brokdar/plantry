import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { RejectReason } from "@/lib/api/discovery"

const REASONS: RejectReason[] = ["too_long", "ingredients", "taste"]

interface RejectReasonsProps {
  /** One tap decides; `undefined` is "No reason". */
  onPick: (reason: RejectReason | undefined) => void
  disabled?: boolean
}

/** The chips behind "Not for me": one tap rejects with that reason. */
export function RejectReasons({ onPick, disabled }: RejectReasonsProps) {
  const { t } = useTranslation()
  return (
    <div
      role="group"
      aria-label={t("discovery.reject.prompt")}
      className="flex flex-col gap-2"
      data-testid="discovery-reject-reasons"
    >
      <p className="text-small text-ink-muted">
        {t("discovery.reject.prompt")}
      </p>
      <div className="flex flex-wrap gap-2">
        {REASONS.map((reason) => (
          <Button
            key={reason}
            type="button"
            variant="outline"
            disabled={disabled}
            className="h-11 rounded-full px-4 md:pointer-fine:h-9"
            onClick={() => onPick(reason)}
          >
            {t(`discovery.reject.${reason}`)}
          </Button>
        ))}
        <Button
          type="button"
          variant="ghost"
          disabled={disabled}
          className="h-11 rounded-full px-4 md:pointer-fine:h-9"
          onClick={() => onPick(undefined)}
        >
          {t("discovery.reject.none")}
        </Button>
      </div>
    </div>
  )
}
