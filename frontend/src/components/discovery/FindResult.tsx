import { useNavigate } from "@tanstack/react-router"
import { Loader2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { Find, FindFlag, FindReason } from "@/lib/api/finds"
import { EQUIPMENT_TOOLS, type EquipmentTool } from "@/lib/api/kitchen"
import { useDiscardFind, useFind } from "@/lib/queries/finds"
import { useKitchen, usePatchKitchen } from "@/lib/queries/kitchen"

const REASONS: FindReason[] = [
  "taste",
  "ingredients",
  "too_long",
  "looks_wrong",
]

/**
 * One requested find on the Discover sheet (REV-1): the pipeline's steps in
 * Plantry's words while it searches, then the review card — source line,
 * origin, what changed, flags, KIT questions — with "Add to my recipes"
 * (the import review, then keep) and "Not for me" (one-tap reasons).
 */
export function FindResult({ id }: { id: number }) {
  const { t } = useTranslation()
  const find = useFind(id)
  const f = find.data
  if (find.isError)
    return (
      <p role="alert" className="mt-6 text-body text-danger">
        {t("discovery.find.gone")}
      </p>
    )
  if (!f || f.state === "searching")
    return (
      <div role="status" className="mt-6 flex flex-col gap-2">
        <p className="flex items-center gap-2 text-body text-ink-muted">
          <Loader2 className="size-4 animate-spin" aria-hidden />
          {t("discovery.find.searching")}
        </p>
        {f && <Steps find={f} />}
      </div>
    )
  if (f.state === "failed")
    return (
      <div role="status" className="mt-6 flex flex-col gap-2">
        <p className="text-body text-ink-muted">{t("discovery.find.failed")}</p>
        <Steps find={f} />
      </div>
    )
  if (f.state === "in_library" || f.state === "discarded") return null
  return <Review find={f} />
}

function Steps({ find }: { find: Find }) {
  const { t } = useTranslation()
  return (
    <ul className="flex flex-col gap-1 text-small text-ink-muted">
      {(find.steps ?? []).map((s, i) => (
        <li key={i}>{t(`discovery.find.steps.${s.key}`, s.params)}</li>
      ))}
    </ul>
  )
}

function Review({ find }: { find: Find }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const discard = useDiscardFind()
  const [rejecting, setRejecting] = useState(false)
  const base = find.sources?.find((s) => s.base)
  const site = base?.site ?? ""
  const changes = find.changes ?? []
  const flags = find.flags ?? []
  const questions = find.questions ?? []

  return (
    <article
      className="mt-6 flex flex-col gap-4 border-b border-line pb-6"
      data-testid="find-review"
    >
      <header className="flex flex-col gap-1">
        <h3 className="font-heading text-h3 text-ink">
          {find.draft?.name ?? find.dish}
        </h3>
        {base && (
          <p className="text-small text-ink-muted" data-testid="find-source">
            {base.rating !== undefined
              ? t("discovery.find.source", {
                  site,
                  rating: base.rating,
                  votes: base.votes,
                })
              : t("discovery.find.source_unrated", { site })}
          </p>
        )}
        {find.origin && (
          <p className="text-small text-ink-muted" data-testid="find-origin">
            {t(`discovery.find.origin.${find.origin}`, { site })}
          </p>
        )}
      </header>

      {find.state === "needs_check" && (
        <p className="text-small text-ink">{t("discovery.find.needs_check")}</p>
      )}

      {changes.length > 0 && (
        <section className="flex flex-col gap-1">
          <h4 className="text-small font-semibold text-ink">
            {t("discovery.find.changes")}
          </h4>
          <ul className="flex flex-col gap-1 text-small text-ink-muted">
            {changes.map((c, i) => (
              <li key={i}>
                {c.from && c.to ? `${c.from} → ${c.to}` : (c.to ?? c.kind)}
                {c.reason && ` · ${c.reason}`}
              </li>
            ))}
          </ul>
        </section>
      )}

      {flags.length > 0 && (
        <section className="flex flex-col gap-1">
          <h4 className="text-small font-semibold text-ink">
            {t("discovery.find.flags")}
          </h4>
          <ul className="flex flex-col gap-1 text-small text-ink-muted">
            {flags.map((fl, i) => (
              <li key={i}>
                <FlagText flag={fl} />
              </li>
            ))}
          </ul>
        </section>
      )}

      {questions.map((q) => (
        <Question key={q} question={q} />
      ))}

      {rejecting ? (
        <div
          role="group"
          aria-label={t("discovery.find.reject.prompt")}
          className="flex flex-col gap-2"
        >
          <p className="text-small text-ink-muted">
            {t("discovery.find.reject.prompt")}
          </p>
          <div className="flex flex-wrap gap-2">
            {REASONS.map((reason) => (
              <Button
                key={reason}
                type="button"
                variant="outline"
                disabled={discard.isPending}
                className="h-11 rounded-full px-4 md:pointer-fine:h-9"
                onClick={() => discard.mutate({ id: find.id, reason })}
              >
                {t(`discovery.find.reject.${reason}`)}
              </Button>
            ))}
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          <Button
            className="h-11 px-4 md:pointer-fine:h-9"
            onClick={() =>
              void navigate({
                to: "/recipes",
                search: { add: "find", find: find.id },
              })
            }
            data-testid="find-add"
          >
            {t("discovery.find.add")}
          </Button>
          <Button
            variant="outline"
            className="h-11 px-4 md:pointer-fine:h-9"
            onClick={() => setRejecting(true)}
          >
            {t("discovery.find.not_for_me")}
          </Button>
        </div>
      )}
    </article>
  )
}

/** A flag in words: the known codes carry their numbers, a gate flag its
 *  own reason. */
function FlagText({ flag }: { flag: FindFlag }) {
  const { t } = useTranslation()
  return (
    <>
      {t(`discovery.find.flag.${flag.code}`, {
        value: flag.value,
        typical: flag.typical,
        defaultValue: flag.reason ?? flag.code,
      })}
    </>
  )
}

/** A KIT-2/3 question the find raised: "yes" records the tool (or a
 *  confident cook) in the kitchen; either answer closes the question. */
function Question({ question }: { question: string }) {
  const { t } = useTranslation()
  const kitchen = useKitchen()
  const patch = usePatchKitchen()
  const [answered, setAnswered] = useState(false)
  const [kind, tool] = question.split(":")
  if (answered || !kitchen.data) return null
  if (kind === "equipment" && !EQUIPMENT_TOOLS.includes(tool as EquipmentTool))
    return null
  const label =
    kind === "skill"
      ? t("discovery.find.question.skill")
      : t("discovery.find.question.equipment", {
          tool: t(`settings_page.kitchen.tool.${tool}`),
        })
  function answer(yes: boolean) {
    setAnswered(true)
    if (!yes) return
    if (kind === "skill") patch.mutate({ skill: "confident" })
    else
      patch.mutate({
        equipment: [...kitchen.data!.equipment, tool as EquipmentTool],
      })
  }
  return (
    <div
      role="group"
      aria-label={t("discovery.find.questions")}
      className="flex flex-col gap-2"
      data-testid="find-question"
    >
      <p className="text-small text-ink">{label}</p>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          className="h-11 rounded-full px-4 md:pointer-fine:h-9"
          onClick={() => answer(true)}
        >
          {t("discovery.find.answer_yes")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          className="h-11 rounded-full px-4 md:pointer-fine:h-9"
          onClick={() => answer(false)}
        >
          {t("discovery.find.answer_no")}
        </Button>
      </div>
    </div>
  )
}
