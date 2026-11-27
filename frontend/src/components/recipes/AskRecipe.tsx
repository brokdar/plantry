import { Link } from "@tanstack/react-router"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import type { TFunction } from "i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type {
  AskResult,
  EditPreview,
  GateFinding,
  RecipeCheck,
  RecipeProposal,
} from "@/lib/api/recipechange"
import {
  useAskRecipe,
  useCheckRecipe,
  useRecipeChanges,
  useSaveRecipeEdit,
} from "@/lib/queries/recipechange"

/** A finding in the user's language when its code has a line, else the
 *  backend's English reason. */
function findingText(t: TFunction, f: GateFinding): string {
  return t(`recipe_chat.gate.${f.code}`, {
    defaultValue: f.reason,
    ...f.params,
  })
}

function Findings({ check }: { check: RecipeCheck }) {
  const { t } = useTranslation()
  const unmatched = check.lines.filter((l) => !l.matched)
  return (
    <div className="flex flex-col gap-2" data-testid="recipe-check">
      {check.blocks.length > 0 && (
        <ul className="list-disc pl-5 text-small text-danger" role="alert">
          {check.blocks.map((f, i) => (
            <li key={i} data-testid="recipe-check-block">
              {findingText(t, f)}
            </li>
          ))}
        </ul>
      )}
      {check.flags.length > 0 && (
        <ul className="list-disc pl-5 text-small text-ink-muted">
          {check.flags.map((f, i) => (
            <li key={i} data-testid="recipe-check-flag">
              {findingText(t, f)}
            </li>
          ))}
        </ul>
      )}
      {unmatched.length > 0 && (
        <ul className="list-disc pl-5 text-small text-ink-muted">
          {unmatched.map((l, i) => (
            <li key={i}>{t("recipe_chat.unmatched", { line: l.text })}</li>
          ))}
        </ul>
      )}
      {check.blocks.length === 0 &&
        check.flags.length === 0 &&
        unmatched.length === 0 && (
          <p className="text-small text-ink-muted">
            {t("recipe_chat.check_ok")}
          </p>
        )}
    </div>
  )
}

function DiffList({
  title,
  before,
  after,
}: {
  title: string
  before: string[]
  after: string[]
}) {
  const removed = before.filter((l) => !after.includes(l))
  const added = after.filter((l) => !before.includes(l))
  if (removed.length === 0 && added.length === 0) return null
  return (
    <div className="flex flex-col gap-1">
      <h4 className="text-small font-medium">{title}</h4>
      <ul className="font-mono text-small">
        {removed.map((l, i) => (
          <li key={`r${i}`} className="text-danger" data-testid="diff-removed">
            − {l}
          </li>
        ))}
        {added.map((l, i) => (
          <li key={`a${i}`} className="text-accent" data-testid="diff-added">
            + {l}
          </li>
        ))}
      </ul>
    </div>
  )
}

function EditPreviewCard({
  preview,
  proposal,
  recipeId,
  onDone,
}: {
  preview: EditPreview
  proposal: RecipeProposal
  recipeId: number
  onDone: () => void
}) {
  const { t } = useTranslation()
  const save = useSaveRecipeEdit(recipeId)
  const blocked = preview.check.blocks.length > 0
  return (
    <div
      className="flex flex-col gap-3 rounded-lg border border-line p-4"
      data-testid="recipe-edit-preview"
    >
      <h3 className="font-heading text-h3 font-semibold">
        {t("recipe_chat.edit_title")}
      </h3>
      {preview.before.name !== preview.after.name && (
        <p className="text-small">
          {preview.before.name} → {preview.after.name}
        </p>
      )}
      <DiffList
        title={t("recipe_chat.ingredients")}
        before={preview.before.ingredients}
        after={preview.after.ingredients}
      />
      <DiffList
        title={t("recipe_chat.steps")}
        before={preview.before.steps}
        after={preview.after.steps}
      />
      {preview.confirmed_dates.length > 0 && (
        <p
          className="text-small text-warning"
          data-testid="recipe-edit-warning"
        >
          {t("recipe_chat.confirmed_warning", {
            dates: preview.confirmed_dates.join(", "),
          })}
        </p>
      )}
      <Findings check={preview.check} />
      <div className="flex gap-2">
        <Button
          type="button"
          disabled={blocked || save.isPending}
          onClick={() =>
            save.mutate(proposal, {
              onSuccess: (res) => res.food_id > 0 && onDone(),
            })
          }
          data-testid="recipe-edit-save"
        >
          {t("recipe_chat.edit_save")}
        </Button>
        <Button type="button" variant="outline" onClick={onDone}>
          {t("common.cancel")}
        </Button>
      </div>
    </div>
  )
}

function Outcome({
  result,
  recipeId,
  onDone,
}: {
  result: AskResult
  recipeId: number
  onDone: () => void
}) {
  const { t } = useTranslation()
  if (result.kind === "answer") {
    return (
      <p className="text-small" data-testid="recipe-chat-answer">
        {result.answer}
      </p>
    )
  }
  if (result.kind === "edit" && result.preview && result.proposal) {
    return (
      <EditPreviewCard
        preview={result.preview}
        proposal={result.proposal}
        recipeId={recipeId}
        onDone={onDone}
      />
    )
  }
  const v = result.variant
  if (!v) return null
  if (v.food_id === 0) {
    return (
      <div className="flex flex-col gap-2" data-testid="recipe-chat-blocked">
        <p className="text-small font-medium">{t("recipe_chat.blocked")}</p>
        <Findings check={v.check} />
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-2" data-testid="recipe-chat-variant">
      <p className="text-small">
        {t("recipe_chat.variant_saved")}{" "}
        <Link
          to="/recipes/$id"
          params={{ id: String(v.food_id) }}
          className="underline"
        >
          {result.proposal?.name ?? t("recipe_chat.open_variant")}
        </Link>
      </p>
      <ul className="list-disc pl-5 text-small text-ink-muted">
        {v.changes.map((c, i) => (
          <li key={i}>{c}</li>
        ))}
      </ul>
    </div>
  )
}

/** "Ask about this recipe" (HELP-1): chips first, free text second. Changes
 *  only this recipe. Rendered only with Chef on and a provider. */
export function AskRecipe({ recipeId }: { recipeId: number }) {
  const { t } = useTranslation()
  const [text, setText] = useState("")
  const [result, setResult] = useState<AskResult | null>(null)
  const [check, setCheck] = useState<RecipeCheck | null>(null)
  const ask = useAskRecipe(recipeId)
  const checkMut = useCheckRecipe(recipeId)
  const changes = useRecipeChanges(recipeId)

  const send = (request: string) => {
    const r = request.trim()
    if (!r) return
    setCheck(null)
    setResult(null)
    ask.mutate(r, { onSuccess: setResult })
  }
  const runCheck = () => {
    setResult(null)
    checkMut.mutate(undefined, { onSuccess: setCheck })
  }

  return (
    <section
      className="flex flex-col gap-3 rounded-lg border border-line p-4"
      aria-labelledby="ask-recipe-title"
      data-testid="ask-recipe"
    >
      <h2 id="ask-recipe-title" className="font-heading text-h3 font-semibold">
        {t("recipe_chat.title")}
      </h2>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={ask.isPending}
          onClick={() => send(t("recipe_chat.variant_request"))}
          data-testid="ask-chip-variant"
        >
          {t("recipe_chat.chip_variant")}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={checkMut.isPending}
          onClick={runCheck}
          data-testid="ask-chip-check"
        >
          {t("recipe_chat.chip_check")}
        </Button>
      </div>
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          send(text)
          setText("")
        }}
      >
        <Input
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={t("recipe_chat.placeholder")}
          aria-label={t("recipe_chat.placeholder")}
          data-testid="ask-input"
        />
        <Button type="submit" disabled={ask.isPending || !text.trim()}>
          {t("recipe_chat.send")}
        </Button>
      </form>
      {ask.isPending && (
        <p className="text-small text-ink-muted" role="status">
          {t("recipe_chat.working")}
        </p>
      )}
      {result && (
        <Outcome
          result={result}
          recipeId={recipeId}
          onDone={() => setResult(null)}
        />
      )}
      {check && <Findings check={check} />}
      {changes.data && changes.data.items.length > 0 && (
        <div className="flex flex-col gap-1" data-testid="recipe-changes">
          <h3 className="text-small font-medium">
            {t("recipe_chat.changes_title")}
          </h3>
          <ul className="list-disc pl-5 text-small text-ink-muted">
            {changes.data.items.map((c, i) => (
              <li key={i}>{c}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
