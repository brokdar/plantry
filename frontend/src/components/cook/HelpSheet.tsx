import { ChevronLeft } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet"
import { Textarea } from "@/components/ui/textarea"
import { ApiError } from "@/lib/api/client"
import {
  HELP_PROBLEMS,
  type HelpInput,
  type HelpProblem,
  type HelpVerdict,
} from "@/lib/api/help"
import type { ScaledIngredient } from "@/lib/api/foods"
import { useAskHelp, useHelpVerdict } from "@/lib/queries/help"

import { SubstituteDialog } from "./SubstituteDialog"

type View = "tiles" | "wrong" | "answer"

const TILE =
  "flex min-h-14 w-full items-center rounded-cell border border-line bg-panel px-4 py-3 text-left text-h3 text-ink transition-colors hover:bg-panel-sunken"

/**
 * Cook mode's Help (HELP-1, HELP-2): one 56 px button that opens a sheet with
 * three tiles. "I don't have an ingredient" is the instant rule-based
 * substitute flow; "Explain this step" and "Something went wrong" ask the
 * assistant about the step on screen and answer in at most three lines.
 */
export function HelpButton({
  sessionId,
  step,
  ingredients,
}: {
  sessionId: number
  /** 0-based step on screen. */
  step: number
  ingredients: readonly ScaledIngredient[] | undefined
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [swapOpen, setSwapOpen] = useState(false)
  const [view, setView] = useState<View>("tiles")
  const [problem, setProblem] = useState<HelpProblem | null>(null)
  const [text, setText] = useState("")
  const [last, setLast] = useState<HelpInput | null>(null)
  const [verdict, setVerdict] = useState<HelpVerdict | null>(null)
  const ask = useAskHelp()
  const sendVerdict = useHelpVerdict()
  const choices = (ingredients ?? [])
    .filter((i) => i.kind === "leaf")
    .map((i) => ({ id: i.child_id, name: i.name }))

  function reset() {
    setView("tiles")
    setProblem(null)
    setText("")
    setLast(null)
    setVerdict(null)
    ask.reset()
  }

  function send(input: HelpInput) {
    setLast(input)
    setVerdict(null)
    setView("answer")
    ask.mutate(input)
  }

  function onOpenChange(next: boolean) {
    setOpen(next)
    if (!next) reset()
  }

  const trimmed = text.trim()
  const error = ask.error
  const chefOff = error instanceof ApiError && error.status === 409
  const errorText = chefOff
    ? t("help.chef_off")
    : error instanceof ApiError && error.status === 503
      ? t("help.no_provider")
      : t("help.error")

  return (
    <>
      <Button
        variant="outline"
        className="h-14 w-full text-h3"
        onClick={() => setOpen(true)}
        data-testid="help-open"
      >
        {t("help.open")}
      </Button>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="bottom"
          className="max-h-[90dvh] gap-4 overflow-y-auto p-4"
          data-testid="help-sheet"
        >
          <div className="flex items-center gap-2">
            {view !== "tiles" && (
              <Button
                variant="outline"
                size="icon"
                className="size-11 shrink-0"
                aria-label={t("help.back")}
                onClick={reset}
              >
                <ChevronLeft className="size-5" aria-hidden />
              </Button>
            )}
            <SheetTitle className="font-heading text-h2 font-normal text-ink">
              {t("help.title")}
            </SheetTitle>
          </div>
          <SheetDescription className="sr-only">
            {t("help.description")}
          </SheetDescription>

          {view === "tiles" && (
            <div className="flex flex-col gap-3">
              {choices.length > 0 && (
                <button
                  type="button"
                  className={TILE}
                  data-testid="help-tile-substitute"
                  onClick={() => {
                    onOpenChange(false)
                    setSwapOpen(true)
                  }}
                >
                  {t("substitute.cook_entry")}
                </button>
              )}
              <button
                type="button"
                className={TILE}
                data-testid="help-tile-explain"
                onClick={() =>
                  send({
                    session_id: sessionId,
                    step_index: step,
                    kind: "explain_step",
                  })
                }
              >
                {t("help.explain")}
              </button>
              <button
                type="button"
                className={TILE}
                data-testid="help-tile-wrong"
                onClick={() => setView("wrong")}
              >
                {t("help.wrong")}
              </button>
            </div>
          )}

          {view === "wrong" && (
            <div className="flex flex-col gap-3">
              <p className="text-body text-ink-muted">{t("help.wrong_hint")}</p>
              <div
                role="group"
                aria-label={t("help.wrong")}
                className="flex flex-wrap gap-2"
              >
                {HELP_PROBLEMS.map((p) => (
                  <Button
                    key={p}
                    type="button"
                    variant={problem === p ? "default" : "outline"}
                    className="h-12 px-4 text-body"
                    aria-pressed={problem === p}
                    data-testid={`help-problem-${p}`}
                    onClick={() => setProblem(problem === p ? null : p)}
                  >
                    {t(`help.problem.${p}`)}
                  </Button>
                ))}
              </div>
              <Textarea
                value={text}
                onChange={(e) => setText(e.target.value)}
                maxLength={500}
                rows={2}
                aria-label={t("help.wrong_text")}
                placeholder={t("help.wrong_text")}
                data-testid="help-wrong-text"
              />
              <Button
                className="h-14 w-full text-h3"
                disabled={!problem && !trimmed}
                data-testid="help-wrong-send"
                onClick={() =>
                  send({
                    session_id: sessionId,
                    step_index: step,
                    kind: "went_wrong",
                    ...(problem ? { problem } : {}),
                    ...(trimmed ? { text: trimmed } : {}),
                  })
                }
              >
                {t("help.wrong_send")}
              </Button>
            </div>
          )}

          {view === "answer" && (
            <div className="flex flex-col gap-3" aria-live="polite">
              {ask.isPending && (
                <div
                  role="status"
                  aria-label={t("help.loading")}
                  className="flex flex-col gap-2"
                  data-testid="help-loading"
                >
                  <div className="h-6 w-full animate-pulse rounded-cell bg-panel-sunken" />
                  <div className="h-6 w-4/5 animate-pulse rounded-cell bg-panel-sunken" />
                </div>
              )}
              {ask.isSuccess && (
                <>
                  <ul
                    className="flex flex-col gap-2 text-h3 text-ink"
                    data-testid="help-answer"
                  >
                    {ask.data.answer.map((line) => (
                      <li key={line}>{line}</li>
                    ))}
                  </ul>
                  <div
                    role="group"
                    aria-label={t("help.verdict.label")}
                    className="flex gap-2"
                  >
                    {(["helped", "not_helped"] as const).map((v) => (
                      <Button
                        key={v}
                        type="button"
                        variant={verdict === v ? "secondary" : "ghost"}
                        className="h-11 px-4 text-body text-ink-muted"
                        aria-pressed={verdict === v}
                        data-testid={`help-verdict-${v}`}
                        onClick={() => {
                          setVerdict(v)
                          sendVerdict.mutate({
                            runId: ask.data.run_id,
                            verdict: v,
                          })
                        }}
                      >
                        {t(`help.verdict.${v}`)}
                      </Button>
                    ))}
                  </div>
                </>
              )}
              {ask.isError && (
                <div className="flex flex-col gap-3" role="alert">
                  <p className="text-body text-ink" data-testid="help-error">
                    {errorText}
                  </p>
                  {!chefOff && (
                    <Button
                      variant="outline"
                      className="h-14 w-full text-h3"
                      data-testid="help-retry"
                      onClick={() => last && send(last)}
                    >
                      {t("help.retry")}
                    </Button>
                  )}
                </div>
              )}
            </div>
          )}
        </SheetContent>
      </Sheet>
      <SubstituteDialog
        open={swapOpen}
        onOpenChange={setSwapOpen}
        sessionIds={[sessionId]}
        choices={choices}
      />
    </>
  )
}
