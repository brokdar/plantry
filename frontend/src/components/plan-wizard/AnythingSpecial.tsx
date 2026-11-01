import { useId } from "react"
import { useTranslation } from "react-i18next"

import { Switch } from "@/components/settings/controls"
import { Textarea } from "@/components/ui/textarea"

interface AnythingSpecialProps {
  instruction: string
  onInstructionChange: (next: string) => void
  /** "Include a new recipe": one never cooked (D15). */
  includeNew: boolean
  onIncludeNewChange: (next: boolean) => void
}

/** Plan wizard step 2, "Anything special this week?" (UX §5.4): one
 *  optional note and the new-recipe toggle. */
export function AnythingSpecial({
  instruction,
  onInstructionChange,
  includeNew,
  onIncludeNewChange,
}: AnythingSpecialProps) {
  const { t } = useTranslation()
  const noteId = useId()
  const newId = useId()
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <label htmlFor={noteId} className="text-body font-semibold text-ink">
          {t("plan.wizard.special.instruction")}
        </label>
        <Textarea
          id={noteId}
          value={instruction}
          maxLength={500}
          rows={3}
          placeholder={t("plan.wizard.special.placeholder")}
          onChange={(e) => onInstructionChange(e.target.value)}
          className="min-h-24 bg-panel text-body"
          data-testid="wizard-instruction"
        />
      </div>
      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col gap-0.5">
          <label htmlFor={newId} className="text-body text-ink">
            {t("plan.wizard.special.new_recipe")}
          </label>
          <p className="text-small text-ink-muted">
            {t("plan.wizard.special.new_recipe_hint")}
          </p>
        </div>
        <Switch
          id={newId}
          checked={includeNew}
          onChange={onIncludeNewChange}
          testId="wizard-new-recipe"
        />
      </div>
    </div>
  )
}
