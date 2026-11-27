import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ThumbsDown } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { NotForMeReason } from "@/lib/api/foods"
import { useNotForMe } from "@/lib/queries/foods"

const REASONS: NotForMeReason[] = [
  "taste",
  "ingredients",
  "too_long",
  "looks_wrong",
]

/** "Not for me" in the recipe detail (Chef on): one tap on a reason hides the
 *  recipe from planning and teaches the taste model; Undo comes as a toast. */
export function NotForMe({ id }: { id: number }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [asking, setAsking] = useState(false)
  const notForMe = useNotForMe()

  if (!asking) {
    return (
      <Button
        type="button"
        variant="outline"
        className="max-md:size-11"
        onClick={() => setAsking(true)}
        data-testid="recipe-not-for-me"
      >
        <ThumbsDown data-icon="inline-start" strokeWidth={1.5} aria-hidden />
        <span className="max-md:sr-only">
          {t("recipe_detail.not_for_me.button")}
        </span>
      </Button>
    )
  }
  return (
    <div
      role="group"
      aria-label={t("recipe_detail.not_for_me.prompt")}
      className="flex w-full flex-col gap-2"
      data-testid="recipe-not-for-me-reasons"
    >
      <p className="text-small text-ink-muted">
        {t("recipe_detail.not_for_me.prompt")}
      </p>
      <div className="flex flex-wrap gap-2">
        {REASONS.map((reason) => (
          <Button
            key={reason}
            type="button"
            variant="outline"
            disabled={notForMe.isPending}
            className="h-11 rounded-full px-4 md:pointer-fine:h-9"
            onClick={() =>
              notForMe.mutate(
                { id, reason },
                { onSuccess: () => void navigate({ to: "/recipes" }) }
              )
            }
          >
            {t(`recipe_detail.not_for_me.${reason}`)}
          </Button>
        ))}
      </div>
    </div>
  )
}
