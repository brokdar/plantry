import { Check, Undo2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { RecipeChange } from "@/lib/assistant/transcript"
import { useRevertRecipeChange } from "@/lib/queries/recipechange"

/** A recipe change the chat saved (a variant, an in-place edit, attributes),
 *  with its Undo through the food change log (HELP-5). */
export function RecipeChangeLine({ change }: { change: RecipeChange }) {
  const { t } = useTranslation()
  const [undone, setUndone] = useState(false)
  const revert = useRevertRecipeChange()
  return (
    <div
      data-testid="assistant-recipe-change"
      data-state={undone ? "undone" : "applied"}
      className="flex items-center justify-between gap-3 rounded-cell border border-line bg-panel px-3 py-2 text-small text-ink"
    >
      <span className="flex min-w-0 items-center gap-2">
        <Check className="size-4 shrink-0 text-success" aria-hidden="true" />
        <span className={undone ? "text-ink-muted line-through" : ""}>
          {t(`assistant.recipe_change.${change.tool}`)}
        </span>
      </span>
      {undone ? (
        <span className="shrink-0 text-caption text-ink-muted">
          {t("assistant.undone")}
        </span>
      ) : (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={revert.isPending}
          onClick={() =>
            revert.mutate(change.undo, { onSuccess: () => setUndone(true) })
          }
          data-testid="assistant-recipe-change-undo"
        >
          <Undo2 className="mr-1.5 size-3.5" aria-hidden="true" />
          {t("assistant.undo")}
        </Button>
      )}
    </div>
  )
}
