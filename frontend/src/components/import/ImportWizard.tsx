import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import type {
  Draft,
  ImportCheck,
  ImportChange,
  IngredientEditValue,
} from "@/lib/api/import"
import { useExtractRecipe, useResolveImport } from "@/lib/queries/import"
import type { FoodRole } from "@/lib/api/foods"
import { useCreateFood } from "@/lib/queries/foods"
import { FOOD_ROLES } from "@/lib/schemas/food"
import { ApiError } from "@/lib/api/client"
import { toast } from "@/lib/toast"

import { draftOrder, draftRows, rowProblem } from "./draft-rows"
import { IngredientResolveRow, type RowState } from "./IngredientResolveRow"
import { ImportCheckSummary } from "./ImportCheckSummary"
import { ImportNutritionSummary } from "./ImportNutritionSummary"
import { StepsEditor } from "./StepsEditor"

export interface ImportWizardProps {
  /** Opens straight at the review with this draft (D18: a discovery
   *  suggestion); there is no link step to go back to. */
  initialDraft?: Draft
  /** The review's rows for `initialDraft`, e.g. preselected matches.
   *  Defaults to `draftRows(initialDraft)`. */
  initialRows?: RowState[]
  /** The import check of `initialDraft` (a Discover add, IMP-1); its
   *  changes are already in the draft. */
  initialCheck?: ImportCheck
  /** Tags added to the draft's own on save. */
  extraTags?: string[]
  /** Called with the new recipe's id after it is saved, before the recipe
   *  opens. */
  onSaved?: (foodId: number) => void
}

export function ImportWizard({
  initialDraft,
  initialRows,
  initialCheck,
  extraTags = [],
  onSaved,
}: ImportWizardProps = {}) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const lang = i18n.language.startsWith("en") ? "en" : "de"

  const [step, setStep] = useState<1 | 2>(initialDraft ? 2 : 1)
  const [draft, setDraft] = useState<Draft | null>(initialDraft ?? null)
  const [url, setUrl] = useState("")
  const [html, setHtml] = useState("")
  const [showHtml, setShowHtml] = useState(false)
  const [rows, setRows] = useState<RowState[]>(
    () => initialRows ?? (initialDraft ? draftRows(initialDraft) : [])
  )
  const [order, setOrder] = useState<number[]>(() =>
    initialDraft ? draftOrder(initialDraft) : []
  )
  const [steps, setSteps] = useState<string[]>(
    () => initialDraft?.instructions ?? []
  )
  const [prepMinutes, setPrepMinutes] = useState<number | null>(
    initialDraft?.prep_minutes ?? null
  )
  const [cookMinutes, setCookMinutes] = useState<number | null>(
    initialDraft?.cook_minutes ?? null
  )
  const [recipeName, setRecipeName] = useState(initialDraft?.name ?? "")
  const [role, setRole] = useState<(typeof FOOD_ROLES)[number]>(
    asRole(initialDraft?.role)
  )
  const [description, setDescription] = useState(
    initialDraft?.description ?? ""
  )
  const [keepDays, setKeepDays] = useState<number | null>(
    initialDraft?.keep_days ?? null
  )
  const [freezable, setFreezable] = useState<boolean | null>(
    initialDraft?.freezable ?? null
  )
  const [check, setCheck] = useState<ImportCheck | null>(initialCheck ?? null)
  const [undone, setUndone] = useState<ReadonlySet<string>>(new Set())
  const [touchAdded, setTouchAdded] = useState(false)
  // Kept as the raw input string so the field can be cleared while typing;
  // validated (≥ 1) on save.
  const [portionsInput, setPortionsInput] = useState(
    String(initialDraft?.reference_portions || 1)
  )
  const [portionsError, setPortionsError] = useState(false)
  const [submitError, setSubmitError] = useState<{
    key: string
    params?: Record<string, string>
  } | null>(null)
  const referencePortions = Number(portionsInput)
  const portionsValid =
    Number.isFinite(referencePortions) && referencePortions >= 1

  const extract = useExtractRecipe()
  const resolve = useResolveImport()
  const createComp = useCreateFood()

  async function onSubmitStep1(e: React.FormEvent) {
    e.preventDefault()
    setSubmitError(null)
    try {
      const res = await extract.mutateAsync(
        showHtml ? { html, lang } : { url: url.trim(), lang }
      )
      const d = res.draft
      setDraft(d)
      setRecipeName(d.name)
      setPortionsInput(String(d.reference_portions || 1))
      setPortionsError(false)
      setRows(draftRows(d))
      setOrder(draftOrder(d))
      setSteps(d.instructions)
      setPrepMinutes(d.prep_minutes)
      setCookMinutes(d.cook_minutes)
      setRole(asRole(d.role))
      setDescription(d.description)
      setKeepDays(d.keep_days ?? null)
      setFreezable(d.freezable ?? null)
      setCheck(res.check ?? null)
      setUndone(new Set())
      setTouchAdded(false)
      setStep(2)
    } catch (err) {
      if (err instanceof ApiError)
        setSubmitError({ key: err.messageKey, params: err.params })
      else setSubmitError({ key: "error.server" })
    }
  }

  /** Puts one change's edits back (IMP-5: Undo before Save). */
  function undoChange(change: ImportChange) {
    let stale = false
    for (const e of change.edits) {
      switch (e.field) {
        case "prep_minutes":
          setPrepMinutes(e.before as number | null)
          break
        case "cook_minutes":
          setCookMinutes(e.before as number | null)
          break
        case "keep_days":
          setKeepDays(e.before as number | null)
          break
        case "freezable":
          setFreezable(e.before as boolean | null)
          break
        case "role":
          setRole(asRole(e.before as string | null))
          break
        case "name":
          setRecipeName(e.before as string)
          break
        case "description":
          setDescription(e.before as string)
          break
        case "step": {
          // Steps can be edited, moved or deleted after the check: put
          // `before` back only over a step that still reads `after`.
          const after = (e.after as string).trim()
          const at =
            steps[e.index]?.trim() === after
              ? e.index
              : steps.findIndex((s) => s.trim() === after)
          if (at < 0) {
            stale = true
            break
          }
          setSteps((prev) =>
            prev.map((s, i) => (i === at ? (e.before as string) : s))
          )
          break
        }
        case "ingredient": {
          const b = e.before as IngredientEditValue
          setRows((prev) =>
            prev.map((r, i) =>
              i === e.index
                ? {
                    ...r,
                    amount: b.amount,
                    unit: b.unit || "g",
                    proposed: b.proposed,
                  }
                : r
            )
          )
          break
        }
      }
    }
    if (stale) {
      toast.warning(t("import.check.step_changed"))
      return
    }
    setUndone((prev) => new Set(prev).add(change.id))
  }

  async function onSave() {
    if (!draft) return
    if (!portionsValid) {
      setPortionsError(true)
      return
    }
    try {
      const resolveRes = await resolve.mutateAsync({
        name: recipeName,
        role,
        reference_portions: referencePortions,
        prep_minutes: prepMinutes,
        cook_minutes: cookMinutes,
        notes: null,
        tags: [...new Set([...draft.tags, ...extraTags])],
        instructions: steps
          .map((text) => text.trim())
          .filter((text) => text !== "")
          .map((text, i) => ({ step_number: i + 1, text })),
        children: rows
          .filter((row) => !row.removed)
          .map((row) => ({
            resolution: row.resolution,
            food_id: row.foodId ?? 0,
            amount: row.amount,
            unit: row.unit,
            to_taste: row.toTaste,
            portion_grams:
              row.portionEdited && row.unit === row.countUnit
                ? row.portionGrams
                : null,
          })),
      })
      const resolved = resolveRes.food
      const created = await createComp.mutateAsync({
        kind: "composed",
        name: resolved.name,
        role: resolved.role as FoodRole | undefined,
        reference_portions: resolved.reference_portions,
        prep_minutes: resolved.prep_minutes ?? undefined,
        cook_minutes: resolved.cook_minutes ?? undefined,
        notes: resolved.notes,
        source_url: draft.source_url || undefined,
        description: description || undefined,
        keep_days: keepDays,
        freezable,
        image_url: draft.image_url || undefined,
        children: resolved.children,
        instructions: resolved.instructions,
        tags: resolved.tags,
      })
      if (created.warnings?.includes("image_download_failed")) {
        toast.warning(t("import.review.image_download_failed"))
      }
      onSaved?.(created.id)
      navigate({
        to: "/recipes/$id",
        params: { id: String(created.id) },
      })
    } catch {
      // The failing mutation has already toasted the error.
    }
  }

  // P3: every kept line needs a food, and an amount unless it is to taste.
  const problems = rows.map(rowProblem)
  const count = (p: string) => problems.filter((x) => x === p).length
  const unresolvedCount = count("match")
  const missingAmountCount = count("amount")
  const missingWeightCount = count("weight")
  const noLines = rows.every((r) => r.removed)
  const blocked =
    unresolvedCount + missingAmountCount + missingWeightCount > 0 || noLines
  const saving = resolve.isPending || createComp.isPending

  return (
    <div className="flex flex-col gap-6">
      {step === 1 && (
        <div className="flex flex-col gap-4">
          <h2 className="text-lg font-medium">{t("import.step1.title")}</h2>
          <form className="space-y-4" onSubmit={onSubmitStep1}>
            {!showHtml ? (
              <div className="space-y-2">
                <Label htmlFor="import-url">
                  {t("import.step1.url_label")}
                </Label>
                <Input
                  id="import-url"
                  type="url"
                  placeholder="https://www.chefkoch.de/…"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  required
                  className="h-11 md:pointer-fine:h-9"
                />
                <button
                  type="button"
                  className="flex min-h-11 items-center text-xs text-muted-foreground underline underline-offset-2 md:pointer-fine:min-h-0"
                  onClick={() => {
                    setShowHtml(true)
                    setUrl("")
                  }}
                >
                  {t("import.step1.paste_html_toggle")}
                </button>
              </div>
            ) : (
              <div className="space-y-2">
                <Label htmlFor="import-html">
                  {t("import.step1.html_label")}
                </Label>
                <Textarea
                  id="import-html"
                  rows={8}
                  value={html}
                  onChange={(e) => setHtml(e.target.value)}
                  placeholder="<html>…</html>"
                  required
                />
                <button
                  type="button"
                  className="flex min-h-11 items-center text-xs text-muted-foreground underline underline-offset-2 md:pointer-fine:min-h-0"
                  onClick={() => {
                    setShowHtml(false)
                    setHtml("")
                  }}
                >
                  {t("import.step1.url_toggle")}
                </button>
              </div>
            )}

            {submitError && (
              <p className="text-sm text-destructive">
                {t(submitError.key, submitError.params)}
              </p>
            )}

            <Button
              type="submit"
              disabled={extract.isPending}
              className="h-11 md:pointer-fine:h-9"
            >
              {extract.isPending
                ? t("import.status.extracting")
                : t("import.step1.submit")}
            </Button>
          </form>
        </div>
      )}

      {step === 2 && draft && (
        <div className="flex flex-col gap-6">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-medium">{t("import.review.title")}</h2>
            <Badge variant="secondary">
              {draft.extract_method === "llm"
                ? t("import.status.via_llm")
                : t("import.status.via_jsonld")}
            </Badge>
          </div>
          {draft.warnings.map((w) => (
            <p
              key={w}
              className="rounded border border-amber-400 bg-amber-50 p-2 text-sm text-amber-900"
            >
              {t(`import.warning.${w}`)}
            </p>
          ))}
          {draft.extract_method === "llm" && (
            <p className="rounded border border-amber-400 bg-amber-50 p-2 text-sm text-amber-900">
              {t("import.step2.llm_warning")}
            </p>
          )}

          <ImportCheckSummary
            check={check}
            undone={undone}
            onUndo={(id) => {
              const change = check?.changes.find((c) => c.id === id)
              if (change) undoChange(change)
            }}
            touchAdded={touchAdded}
            onAddTouch={() => {
              if (check?.finishing_touch)
                setSteps((prev) => [...prev, check.finishing_touch!])
              setTouchAdded(true)
            }}
          />

          <div className="flex flex-col gap-4">
            <div className="space-y-2">
              <Label htmlFor="import-name">
                {t("import.review.name_label")}
              </Label>
              <Input
                id="import-name"
                value={recipeName}
                onChange={(e) => setRecipeName(e.target.value)}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="import-role">
                  {t("import.step3.role_label")}
                </Label>
                <Select
                  value={role}
                  onValueChange={(v) =>
                    setRole(v as (typeof FOOD_ROLES)[number])
                  }
                >
                  <SelectTrigger id="import-role">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {FOOD_ROLES.map((r) => (
                      <SelectItem key={r} value={r}>
                        {t(`component.role_${r}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="import-portions">
                  {t("import.step3.portions_label")}
                </Label>
                <Input
                  id="import-portions"
                  type="number"
                  min={1}
                  value={portionsInput}
                  onChange={(e) => {
                    setPortionsInput(e.target.value)
                    setPortionsError(false)
                  }}
                  aria-invalid={portionsError || undefined}
                  aria-describedby={
                    portionsError ? "import-portions-error" : undefined
                  }
                />
                {portionsError && (
                  <p
                    id="import-portions-error"
                    className="text-small text-danger"
                  >
                    {t("import.review.portions_invalid")}
                  </p>
                )}
              </div>
            </div>
            <ImportNutritionSummary
              rows={rows}
              complete={unresolvedCount === 0 && portionsValid}
              referencePortions={referencePortions}
              sourceKcalPerServing={draft.source_kcal_per_serving}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <MinutesInput
              id="import-prep"
              label={t("import.review.prep_label")}
              value={prepMinutes}
              onChange={setPrepMinutes}
            />
            <MinutesInput
              id="import-cook"
              label={t("import.review.cook_label")}
              value={cookMinutes}
              onChange={setCookMinutes}
            />
          </div>

          <Separator />

          <div className="space-y-2">
            <Label>{t("import.step2.ingredient_col")}</Label>
            <div className="space-y-2">
              {order.map((idx) => (
                <IngredientResolveRow
                  key={idx}
                  ing={draft.ingredients[idx]}
                  state={rows[idx]}
                  lang={draft.language === "en" ? "en" : "de"}
                  onChange={(patch) =>
                    setRows((prev) =>
                      prev.map((r, i) => (i === idx ? { ...r, ...patch } : r))
                    )
                  }
                />
              ))}
            </div>
          </div>

          <Separator />

          <div className="space-y-2">
            <Label>{t("import.steps.title")}</Label>
            <StepsEditor steps={steps} onChange={setSteps} />
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3">
            {!initialDraft && (
              <Button variant="outline" onClick={() => setStep(1)}>
                {t("common.back")}
              </Button>
            )}
            <div className="ml-auto flex flex-wrap items-center justify-end gap-3">
              {blocked && (
                <p
                  id="import-save-hint"
                  role="status"
                  className="text-sm text-ink-muted"
                >
                  {unresolvedCount > 0
                    ? t("import.review.needs_choice", {
                        count: unresolvedCount,
                      })
                    : missingAmountCount > 0
                      ? t("import.review.needs_amount", {
                          count: missingAmountCount,
                        })
                      : missingWeightCount > 0
                        ? t("import.review.needs_weight", {
                            count: missingWeightCount,
                          })
                        : t("import.review.no_lines")}
                </p>
              )}
              <Button
                onClick={onSave}
                disabled={blocked || saving}
                aria-describedby={blocked ? "import-save-hint" : undefined}
              >
                {saving ? t("common.loading") : t("import.review.save")}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function asRole(v: string | null | undefined): (typeof FOOD_ROLES)[number] {
  return FOOD_ROLES.find((r) => r === v) ?? "main"
}

function MinutesInput({
  id,
  label,
  value,
  onChange,
}: {
  id: string
  label: string
  value: number | null
  onChange: (v: number | null) => void
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type="number"
        inputMode="numeric"
        min={0}
        value={value ?? ""}
        onChange={(e) =>
          onChange(
            e.target.value === "" ? null : Math.max(0, Number(e.target.value))
          )
        }
      />
    </div>
  )
}
