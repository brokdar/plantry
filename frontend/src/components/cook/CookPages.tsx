import { Link, useNavigate } from "@tanstack/react-router"
import { X } from "lucide-react"
import { useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { Food } from "@/lib/api/foods"
import { batchBadgeText, coverageText } from "@/lib/plan/format"
import { useFood, useScaled } from "@/lib/queries/foods"
import { useSession, useSetSessionCooked } from "@/lib/queries/sessions"
import { useTimeSlots } from "@/lib/queries/slots"
import { slotLabel } from "@/lib/slot-label"

import { CookMode } from "./CookMode"
import { useSessionPortions } from "./useSessionPortions"

function stepsOf(food: Food | undefined): string[] {
  if (food?.kind !== "composed") return []
  return [...(food.instructions ?? [])]
    .sort((a, b) => a.step_number - b.step_number)
    .map((s) => s.text)
}

function CloseButton({ children }: { children: ReactNode }) {
  return (
    <Button asChild variant="outline" size="icon" className="size-11 shrink-0">
      {children}
    </Button>
  )
}

function CookShell({ message }: { message?: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-dvh flex-col gap-4 bg-ground px-4 py-6 md:px-8">
      {message ? (
        <>
          <p className="text-h3 text-ink">{message}</p>
          <Button asChild className="h-11 self-start px-4">
            <Link to="/today">{t("error.go_today")}</Link>
          </Button>
        </>
      ) : (
        <div aria-hidden className="flex flex-col gap-4">
          <div className="h-10 w-2/3 animate-pulse rounded-cell bg-panel-sunken" />
          <div className="h-11 w-1/2 animate-pulse rounded-cell bg-panel-sunken" />
          <div className="h-56 w-full animate-pulse rounded-cell bg-panel-sunken" />
        </div>
      )}
    </div>
  )
}

/**
 * `/cook/$sessionId`: cooks a planned batch. The stepper writes the batch's
 * portions (debounced) and says what they cover; Done cooking marks the
 * batch cooked and returns to Today.
 */
export function CookSessionPage({ sessionId }: { sessionId: number }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const session = useSession(sessionId)
  const food = useFood(session.data?.food_id ?? 0)
  const slots = useTimeSlots()
  const { portions, change, flush } = useSessionPortions(session.data)
  const scaled = useScaled(session.data?.food_id ?? 0, portions)
  const cooked = useSetSessionCooked()

  if (session.isError || food.isError) {
    return <CookShell message={t("cook.missing")} />
  }
  const s = session.data
  if (!s || !food.data) return <CookShell />

  const names = new Map(
    (slots.data?.items ?? []).map((slot) => [
      slot.id,
      slotLabel(t, slot.name_key),
    ])
  )
  const spare = Math.max(0, portions - s.consumed)
  const coverage =
    coverageText(s.links, (id) => names.get(id) ?? "", i18n) +
    (spare > 0
      ? ` · ${batchBadgeText({ variant: "surplus", count: spare }, i18n)}`
      : "")

  return (
    <CookMode
      name={s.food_name}
      steps={stepsOf(food.data)}
      ingredients={scaled.data?.ingredients}
      portions={portions}
      onPortionsChange={change}
      coverage={coverage}
      freezePortions={s.frozen_portions}
      close={
        <CloseButton>
          <Link to="/today" aria-label={t("cook.close")}>
            <X className="size-5" strokeWidth={1.5} aria-hidden />
          </Link>
        </CloseButton>
      }
      donePending={cooked.isPending}
      onDone={() => {
        flush()
        cooked.mutate(
          { id: s.id, cooked: true },
          { onSuccess: () => void navigate({ to: "/today" }) }
        )
      }}
    />
  )
}

/**
 * `/recipes/$id/cook?portions=`: "Cook now" without a plan (D30). The
 * stepper is local and nothing is written; Done returns to the recipe.
 */
export function CookRecipePage({
  id,
  portions: initial,
}: {
  id: number
  portions?: number
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const food = useFood(id)
  const [chosen, setChosen] = useState<number | null>(initial ?? null)
  const reference =
    food.data?.kind === "composed" ? (food.data.reference_portions ?? 1) : 1
  const portions = chosen ?? reference
  const scaled = useScaled(food.data?.kind === "composed" ? id : 0, portions)

  if (food.isError || (food.data && food.data.kind !== "composed")) {
    return <CookShell message={t("cook.recipe_missing")} />
  }
  if (!food.data) return <CookShell />

  const back = { to: "/recipes/$id", params: { id: String(id) } } as const
  return (
    <CookMode
      name={food.data.name}
      steps={stepsOf(food.data)}
      ingredients={scaled.data?.ingredients}
      portions={portions}
      onPortionsChange={setChosen}
      close={
        <CloseButton>
          <Link {...back} aria-label={t("cook.close")}>
            <X className="size-5" strokeWidth={1.5} aria-hidden />
          </Link>
        </CloseButton>
      }
      onDone={() => void navigate(back)}
    />
  )
}
