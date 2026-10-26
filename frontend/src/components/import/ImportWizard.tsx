import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
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

import type { Draft, Resolution } from "@/lib/api/import"
import { useExtractRecipe, useResolveImport } from "@/lib/queries/import"
import type { FoodRole } from "@/lib/api/foods"
import { useCreateFood } from "@/lib/queries/foods"
import { FOOD_ROLES } from "@/lib/schemas/food"
import { ApiError } from "@/lib/api/client"
import { toast } from "@/lib/toast"

import { IngredientResolveRow, type RowState } from "./IngredientResolveRow"
import { ImportNutritionSummary } from "./ImportNutritionSummary"

export function ImportWizard() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [step, setStep] = useState<1 | 2>(1)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [url, setUrl] = useState("")
  const [html, setHtml] = useState("")
  const [showHtml, setShowHtml] = useState(false)
  const [rows, setRows] = useState<RowState[]>([])
  const [recipeName, setRecipeName] = useState("")
  const [role, setRole] = useState<(typeof FOOD_ROLES)[number]>("main")
  // Kept as the raw input string so the field can be cleared while typing;
  // validated (≥ 1) on save.
  const [portionsInput, setPortionsInput] = useState("1")
  const [portionsError, setPortionsError] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
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
        showHtml ? { html } : { url: url.trim() }
      )
      const d = res.draft
      setDraft(d)
      setRecipeName(d.name)
      setPortionsInput(String(d.reference_portions || 1))
      setPortionsError(false)
      setRows(
        d.ingredients.map((ing) => ({
          resolution:
            ing.confidence === "unparsed" ? "skip" : ("existing" as Resolution),
          foodId: null,
          foodName: null,
          amount: ing.amount,
          unit: ing.unit || "g",
        }))
      )
      setStep(2)
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.messageKey)
      else setSubmitError("error.server")
    }
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
        prep_minutes: draft.prep_minutes,
        cook_minutes: draft.cook_minutes,
        notes: null,
        tags: draft.tags,
        instructions: draft.instructions.map((text, i) => ({
          step_number: i + 1,
          text,
        })),
        children: rows.map((row) => ({
          resolution: row.resolution,
          food_id: row.foodId ?? 0,
          amount: row.amount,
          unit: row.unit,
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
        description: draft.description || undefined,
        image_url: draft.image_url || undefined,
        children: resolved.children,
        instructions: resolved.instructions,
        tags: resolved.tags,
      })
      if (created.warnings?.includes("image_download_failed")) {
        toast.warning(t("import.review.image_download_failed"))
      }
      navigate({
        to: "/components/$id",
        params: { id: String(created.id) },
      })
    } catch {
      // The failing mutation has already toasted the error.
    }
  }

  // Skipped rows (including quantity-less lines like "Salz und Pfeffer")
  // count as resolved.
  const unresolvedCount = rows.filter(
    (r) => r.resolution === "existing" && !r.foodId
  ).length
  const saving = resolve.isPending || createComp.isPending

  return (
    <section className="mx-auto max-w-4xl space-y-8 px-4 py-8 md:px-8 md:py-12">
      <header className="space-y-2">
        <h1 className="font-heading text-4xl font-extrabold tracking-tight text-on-surface md:text-5xl">
          {t("import.title")}
        </h1>
        <p className="max-w-lg text-on-surface-variant md:text-lg">
          {t("import.subtitle")}
        </p>
      </header>

      {step === 1 && (
        <Card className="space-y-4 p-6">
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
                />
                <button
                  type="button"
                  className="text-xs text-muted-foreground underline underline-offset-2"
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
                  className="text-xs text-muted-foreground underline underline-offset-2"
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
              <p className="text-sm text-destructive">{t(submitError)}</p>
            )}

            <Button type="submit" disabled={extract.isPending}>
              {extract.isPending
                ? t("import.status.extracting")
                : t("import.step1.submit")}
            </Button>
          </form>
        </Card>
      )}

      {step === 2 && draft && (
        <Card className="flex flex-col gap-6 p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-medium">{t("import.review.title")}</h2>
            <Badge variant="secondary">
              {draft.extract_method === "llm"
                ? t("import.status.via_llm")
                : t("import.status.via_jsonld")}
            </Badge>
          </div>
          {draft.extract_method === "llm" && (
            <p className="rounded border border-amber-400 bg-amber-50 p-2 text-sm text-amber-900">
              {t("import.step2.llm_warning")}
            </p>
          )}

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

          <Separator />

          <div className="space-y-2">
            <Label>{t("import.step2.ingredient_col")}</Label>
            <div className="space-y-2">
              {draft.ingredients.map((ing, idx) => (
                <IngredientResolveRow
                  key={idx}
                  ing={ing}
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

          <div className="flex flex-wrap items-center justify-between gap-3">
            <Button variant="outline" onClick={() => setStep(1)}>
              {t("common.back")}
            </Button>
            <div className="flex flex-wrap items-center justify-end gap-3">
              {unresolvedCount > 0 && (
                <p
                  id="import-save-hint"
                  role="status"
                  className="text-sm text-ink-muted"
                >
                  {t("import.review.needs_choice", { count: unresolvedCount })}
                </p>
              )}
              <Button
                onClick={onSave}
                disabled={unresolvedCount > 0 || saving}
                aria-describedby={
                  unresolvedCount > 0 ? "import-save-hint" : undefined
                }
              >
                {saving ? t("common.loading") : t("import.review.save")}
              </Button>
            </div>
          </div>
        </Card>
      )}
    </section>
  )
}
