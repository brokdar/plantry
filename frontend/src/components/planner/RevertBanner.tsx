import { Sparkles } from "lucide-react"
import { useTranslation } from "react-i18next"

interface RevertBannerProps {
  count: number
  onRevert: () => void
  onDismiss: () => void
  reverting?: boolean
}

export function RevertBanner({
  count,
  onRevert,
  onDismiss,
  reverting,
}: RevertBannerProps) {
  const { t } = useTranslation()
  return (
    <div
      role="status"
      aria-live="polite"
      data-testid="revert-banner"
      className="mb-6 flex items-center gap-4 rounded-[14px] border border-[#c2974a]/30 p-3.5 px-5 text-[13.5px]"
      style={{
        backgroundImage:
          "linear-gradient(90deg, #f1e3c4 0%, color-mix(in oklab, #f1e3c4 50%, var(--surface-container-lowest)) 100%)",
      }}
    >
      <span className="grid size-7 place-items-center rounded-lg bg-[#c2974a] text-white">
        <Sparkles className="h-3.5 w-3.5" aria-hidden />
      </span>
      <div className="flex-1">
        <p className="font-heading font-bold">
          {t("planner.revert.title", { count })}
        </p>
        <p className="text-[12.5px] text-on-surface-variant">
          {t("planner.revert.body")}
        </p>
      </div>
      <button
        type="button"
        onClick={onRevert}
        disabled={reverting}
        data-testid="revert-banner-revert"
        className="h-8 rounded-full border border-[#8a6a2f] px-3.5 font-heading text-[12.5px] font-semibold text-[#6a4f20] transition-colors hover:bg-white disabled:opacity-50"
      >
        {t("planner.revert.revert")}
      </button>
      <button
        type="button"
        onClick={onDismiss}
        data-testid="revert-banner-dismiss"
        className="h-8 rounded-full px-3 font-heading text-[12.5px] font-semibold text-on-surface-variant hover:text-on-surface"
      >
        {t("planner.revert.dismiss")}
      </button>
    </div>
  )
}
