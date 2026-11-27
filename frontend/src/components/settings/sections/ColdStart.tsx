import { useState } from "react"
import { useTranslation } from "react-i18next"

import { StatementChips } from "@/components/assistant/StatementChips"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { Statement } from "@/lib/api/taste"
import { useProposeStatements } from "@/lib/queries/taste"

/** The cold-start question (COLD-1): one sentence about what the household
 *  loves and would rather skip becomes statements to confirm with the same
 *  chips the assistant uses. Nothing is stored before a tap. */
export function ColdStart() {
  const { t } = useTranslation()
  const [text, setText] = useState("")
  const [proposed, setProposed] = useState<Statement[] | null>(null)
  const propose = useProposeStatements()
  return (
    <section
      className="flex flex-col gap-3"
      aria-labelledby="cold-start"
      data-testid="cold-start"
    >
      <h2
        id="cold-start"
        className="font-sans text-body font-semibold text-ink"
      >
        {t("settings_page.taste.cold.title")}
      </h2>
      <p className="text-small text-ink-muted">
        {t("settings_page.taste.cold.hint")}
      </p>
      <Textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={t("settings_page.taste.cold.placeholder")}
        aria-labelledby="cold-start"
        data-testid="cold-start-text"
      />
      <div>
        <Button
          type="button"
          className="h-11 md:pointer-fine:h-9"
          disabled={propose.isPending || text.trim() === ""}
          onClick={() =>
            propose.mutate(text, {
              onSuccess: (res) => setProposed(res.statements),
            })
          }
          data-testid="cold-start-submit"
        >
          {t("settings_page.taste.cold.submit")}
        </Button>
      </div>
      {proposed?.length === 0 && (
        <p className="text-small text-ink-muted">
          {t("settings_page.taste.cold.none")}
        </p>
      )}
      <div className="flex flex-col gap-2">
        {proposed?.map((s) => (
          <StatementChips
            key={s.kind === "health" ? `h-${s.word}` : `${s.level}-${s.key}`}
            statement={s}
            alwaysOnly
          />
        ))}
      </div>
    </section>
  )
}
