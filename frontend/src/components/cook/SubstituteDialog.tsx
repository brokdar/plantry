import { ChevronLeft } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { ScaledIngredient } from "@/lib/api/foods"
import { formatQuantity } from "@/lib/plan/format"
import {
  useApplySubstitution,
  useKeepSubstitution,
  useRefreshSwap,
  useSubstitutes,
} from "@/lib/queries/substitutions"

/** An ingredient the dialog can swap. */
export interface SubstituteSource {
  id: number
  name: string
}

interface SubstituteDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The batches the swap applies to (every batch that needs the item). */
  sessionIds: readonly number[]
  /** Cook mode: the recipe's ingredients to pick from. */
  choices?: readonly SubstituteSource[]
  /** Shop: the item is already known. */
  from?: SubstituteSource
}

/** A swap just applied: the dialog now offers to keep it. */
interface Applied {
  from: SubstituteSource
  to: SubstituteSource
  sessionIds: readonly number[]
  /** The same swap was made before in this recipe (offered once). */
  offer: boolean
  /** Confirmed days whose shopping an in-place change shifts. */
  dates: readonly string[]
}

/**
 * "I don't have an ingredient" (cook mode) and "Not available" (shop): up to
 * three rule-based substitutes with amounts; one tap applies the swap to the
 * batch (every batch that needs the item) and offers Undo.
 */
export function SubstituteDialog({
  open,
  onOpenChange,
  sessionIds,
  choices,
  from: fixed,
}: SubstituteDialogProps) {
  const { t, i18n } = useTranslation()
  const [picked, setPicked] = useState<SubstituteSource | null>(null)
  const from = fixed ?? picked
  const options = useSubstitutes(from?.id ?? 0, sessionIds, open && !!from)
  const apply = useApplySubstitution()
  const keep = useKeepSubstitution()
  const refresh = useRefreshSwap()
  const [applied, setApplied] = useState<Applied | null>(null)
  const [confirmAlways, setConfirmAlways] = useState(false)
  // Only the batches whose recipe has the item (and are not past) change.
  const applicableIds = options.data?.session_ids ?? sessionIds
  const german = i18n.language.startsWith("de")

  function close(next: boolean) {
    if (!next) {
      if (applied) void refresh()
      setPicked(null)
      setApplied(null)
      setConfirmAlways(false)
    }
    onOpenChange(next)
  }

  function keepSwap(mode: "recipe" | "variant") {
    if (!applied) return
    keep.mutate(
      {
        mode,
        sessionIds: applied.sessionIds,
        fromId: applied.from.id,
        toId: applied.to.id,
        message: t(
          mode === "recipe"
            ? "substitute.kept_recipe"
            : "substitute.kept_variant",
          { to: applied.to.name, from: applied.from.name }
        ),
      },
      { onSuccess: () => close(false) }
    )
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent data-testid="substitute-dialog">
        <DialogHeader>
          <DialogTitle>
            {applied
              ? t("substitute.applied", {
                  to: applied.to.name,
                  from: applied.from.name,
                })
              : from
                ? t("substitute.title", { name: from.name })
                : t("substitute.pick_title")}
          </DialogTitle>
          <DialogDescription>
            {applied
              ? applied.offer
                ? t("substitute.offer", {
                    to: applied.to.name,
                    from: applied.from.name,
                  })
                : t("substitute.keep_hint")
              : !from
                ? t("substitute.pick_hint")
                : applicableIds.length < sessionIds.length
                  ? t("substitute.some_batches", {
                      count: applicableIds.length,
                      total: sessionIds.length,
                    })
                  : applicableIds.length > 1
                    ? t("substitute.all_batches", {
                        count: applicableIds.length,
                      })
                    : t("substitute.this_batch")}
          </DialogDescription>
        </DialogHeader>

        {applied && (
          <div className="flex flex-col gap-2" data-testid="substitute-keep">
            {confirmAlways ? (
              <>
                <p className="text-body" role="status">
                  {t("substitute.always_warning", {
                    count: applied.dates.length,
                  })}
                </p>
                <Button
                  className="h-14 w-full text-h3"
                  disabled={keep.isPending}
                  onClick={() => keepSwap("recipe")}
                  data-testid="substitute-always-confirm"
                >
                  {t("substitute.always_confirm")}
                </Button>
              </>
            ) : (
              <>
                <Button
                  variant={applied.offer ? "default" : "outline"}
                  className="h-14 w-full text-h3"
                  disabled={keep.isPending}
                  onClick={() =>
                    applied.dates.length > 0
                      ? setConfirmAlways(true)
                      : keepSwap("recipe")
                  }
                  data-testid="substitute-always"
                >
                  {t("substitute.always")}
                </Button>
                <Button
                  variant="outline"
                  className="h-14 w-full text-h3"
                  disabled={keep.isPending}
                  onClick={() => keepSwap("variant")}
                  data-testid="substitute-variant"
                >
                  {t("substitute.save_variant")}
                </Button>
              </>
            )}
            <Button
              variant="ghost"
              className="h-11 self-start px-3"
              onClick={() => close(false)}
              data-testid="substitute-done"
            >
              {t("substitute.done")}
            </Button>
          </div>
        )}

        {!applied && !from && (
          <ul className="flex flex-col gap-2">
            {(choices ?? []).map((c) => (
              <li key={c.id}>
                <Button
                  variant="outline"
                  className="h-14 w-full justify-start text-h3"
                  onClick={() => setPicked(c)}
                  data-testid="substitute-pick"
                >
                  {c.name}
                </Button>
              </li>
            ))}
          </ul>
        )}

        {!applied && from && options.isPending && (
          <p className="text-body text-ink-muted" role="status">
            {t("substitute.loading")}
          </p>
        )}
        {!applied &&
          from &&
          options.data &&
          options.data.options.length === 0 && (
            <p
              className="text-body text-ink-muted"
              data-testid="substitute-none"
            >
              {t("substitute.none", { name: from.name })}
            </p>
          )}
        {!applied &&
          from &&
          options.data &&
          options.data.options.length > 0 && (
            <ul className="flex flex-col gap-2">
              {options.data.options.map((o) => {
                const note = german ? o.note_de : o.note_en
                const label = t("substitute.option", {
                  amount: o.amount
                    ? formatQuantity(o.amount.amount, o.amount.unit, i18n)
                    : "",
                  name: o.name,
                }).trim()
                return (
                  <li key={o.food_id}>
                    <Button
                      variant="outline"
                      className="h-auto min-h-14 w-full flex-col items-start gap-0.5 py-2 text-left text-h3"
                      disabled={apply.isPending}
                      data-testid="substitute-option"
                      onClick={() =>
                        apply.mutate(
                          {
                            sessionIds: applicableIds,
                            fromId: from.id,
                            toId: o.food_id,
                            message:
                              applicableIds.length > 1
                                ? t("substitute.applied_many", {
                                    to: o.name,
                                    from: from.name,
                                    count: applicableIds.length,
                                  })
                                : t("substitute.applied", {
                                    to: o.name,
                                    from: from.name,
                                  }),
                          },
                          {
                            onSuccess: ({ offer, dates }) =>
                              setApplied({
                                from,
                                to: { id: o.food_id, name: o.name },
                                sessionIds: applicableIds,
                                offer,
                                dates,
                              }),
                          }
                        )
                      }
                    >
                      <span>{label}</span>
                      {note && (
                        <span className="text-small font-normal text-ink-muted">
                          {note}
                        </span>
                      )}
                    </Button>
                  </li>
                )
              })}
            </ul>
          )}

        {!fixed && from && !applied && (
          <Button
            variant="ghost"
            className="h-11 self-start px-3"
            onClick={() => setPicked(null)}
          >
            <ChevronLeft className="size-4" aria-hidden />
            {t("substitute.back")}
          </Button>
        )}
      </DialogContent>
    </Dialog>
  )
}

/** Cook mode's entry: the 56 px button under the ingredients and its dialog. */
export function SubstituteButton({
  sessionId,
  ingredients,
}: {
  sessionId: number
  ingredients: readonly ScaledIngredient[] | undefined
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const choices = (ingredients ?? [])
    .filter((i) => i.kind === "leaf")
    .map((i) => ({ id: i.child_id, name: i.name }))
  if (choices.length === 0) return null
  return (
    <>
      <Button
        variant="outline"
        className="h-14 w-full text-h3"
        onClick={() => setOpen(true)}
        data-testid="substitute-open"
      >
        {t("substitute.cook_entry")}
      </Button>
      <SubstituteDialog
        open={open}
        onOpenChange={setOpen}
        sessionIds={[sessionId]}
        choices={choices}
      />
    </>
  )
}
