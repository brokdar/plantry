import { Check } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { Statement } from "@/lib/api/taste"
import { useConfirmStatement } from "@/lib/queries/taste"

/** A taste or health statement the assistant noted (TASTE-8): [Always] makes
 *  it permanent, [Just this week] keeps it for the window. A health
 *  statement (allergic, celiac, intolerant) is stored only with a tap, as a
 *  never item, so it offers [Always] only. */
export function StatementChips({ statement }: { statement: Statement }) {
  const { t } = useTranslation()
  const confirm = useConfirmStatement()
  const [done, setDone] = useState<"always" | "window" | null>(null)
  const health = statement.kind === "health"
  const about =
    statement.kind === "health"
      ? statement.word
      : statement.key.replaceAll("_", " ")
  function tap(scope: "always" | "window") {
    confirm.mutate({ statement, scope }, { onSuccess: () => setDone(scope) })
  }
  return (
    <div
      data-testid="assistant-statement"
      data-state={done ?? "proposed"}
      className="flex flex-col gap-2 rounded-cell border border-line bg-panel px-3 py-2 text-small text-ink"
    >
      <p>
        {t(
          health
            ? "assistant.statement.health"
            : `assistant.statement.${statement.kind === "taste" ? statement.polarity : ""}`,
          { name: about }
        )}
      </p>
      {done ? (
        <p className="flex items-center gap-2 text-ink-muted">
          <Check className="size-4 text-success" aria-hidden="true" />
          {t(`assistant.statement.done_${done}`)}
        </p>
      ) : (
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            disabled={confirm.isPending}
            onClick={() => tap("always")}
            data-testid="assistant-statement-always"
            className="pointer-coarse:h-11"
          >
            {t("assistant.statement.always")}
          </Button>
          {!health && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={confirm.isPending}
              onClick={() => tap("window")}
              data-testid="assistant-statement-week"
              className="pointer-coarse:h-11"
            >
              {t("assistant.statement.week")}
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
