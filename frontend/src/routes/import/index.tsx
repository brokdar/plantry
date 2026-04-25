import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState } from "react"
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

import type { Draft, DraftIngredient, Resolution } from "@/lib/api/import"
import {
  useExtractRecipe,
  useImportLineLookup,
  useResolveImport,
} from "@/lib/queries/import"
import { createFood, type FoodRole } from "@/lib/api/foods"
import { useCreateFood } from "@/lib/queries/foods"
import { foodKeys } from "@/lib/queries/keys"
import { FOOD_ROLES } from "@/lib/schemas/food"
import { ApiError } from "@/lib/api/client"

export const Route = createFileRoute("/import/")({
  component: ImportPage,
})

type RowState = {
  resolution: Resolution
  foodId: number | null
  foodName: string | null
  amount: number
  unit: string
}

function ImportPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [url, setUrl] = useState("")
  const [html, setHtml] = useState("")
  const [showHtml, setShowHtml] = useState(false)
  const [rows, setRows] = useState<RowState[]>([])
  const [recipeName, setRecipeName] = useState("")
  const [role, setRole] = useState<(typeof FOOD_ROLES)[number]>("main")
  const [referencePortions, setReferencePortions] = useState(1)
  const [submitError, setSubmitError] = useState<string | null>(null)

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
      setReferencePortions(d.reference_portions || 1)
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

  async function onSubmitStep3() {
    if (!draft) return
    setSubmitError(null)
    try {
      const resolveRes = await resolve.mutateAsync({
        name: recipeName,
        role,
        reference_portions: referencePortions,
        prep_minutes: draft.prep_minutes,
        cook_minutes: draft.cook_minutes,
        notes: draft.description || null,
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
        children: resolved.children,
        instructions: resolved.instructions,
        tags: resolved.tags,
      })
      navigate({
        to: "/components/$id/edit",
        params: { id: String(created.id) },
      })
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.messageKey)
      else setSubmitError("error.server")
    }
  }

  const unresolvedCount = useMemo(
    () => rows.filter((r) => r.resolution === "existing" && !r.foodId).length,
    [rows]
  )

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
        <Card className="space-y-4 p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-medium">{t("import.step2.title")}</h2>
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

          {submitError && (
            <p className="text-sm text-destructive">{t(submitError)}</p>
          )}

          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep(1)}>
              {t("common.back")}
            </Button>
            <Button
              onClick={() => setStep(3)}
              disabled={unresolvedCount > 0}
              title={
                unresolvedCount > 0
                  ? t("import.step2.resolve_first", { count: unresolvedCount })
                  : undefined
              }
            >
              {t("common.next")}
            </Button>
          </div>
        </Card>
      )}

      {step === 3 && draft && (
        <Card className="space-y-4 p-6">
          <h2 className="text-lg font-medium">{t("import.step3.title")}</h2>
          <div className="space-y-2">
            <Label htmlFor="import-name">{t("import.step3.name_label")}</Label>
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
                onValueChange={(v) => setRole(v as (typeof FOOD_ROLES)[number])}
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
                value={referencePortions}
                onChange={(e) =>
                  setReferencePortions(Math.max(1, Number(e.target.value)))
                }
              />
            </div>
          </div>

          <Separator />

          {submitError && (
            <p className="text-sm text-destructive">{t(submitError)}</p>
          )}

          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep(2)}>
              {t("common.back")}
            </Button>
            <Button
              onClick={onSubmitStep3}
              disabled={resolve.isPending || createComp.isPending}
            >
              {resolve.isPending || createComp.isPending
                ? t("common.loading")
                : t("import.step3.save")}
            </Button>
          </div>
        </Card>
      )}
    </section>
  )
}

interface RowProps {
  ing: DraftIngredient
  state: RowState
  lang: "de" | "en"
  onChange: (patch: Partial<RowState>) => void
}

function IngredientResolveRow({ ing, state, lang, onChange }: RowProps) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)

  const lookup = useImportLineLookup(ing.name, lang)
  const results = lookup.data?.results ?? []

  // Auto-select the first returned candidate that has an existing_id.
  useEffect(() => {
    if (state.resolution !== "existing") return
    if (state.foodId) return
    if (!lookup.data) return
    const hit = results.find((r) => r.existing_id)
    if (hit && hit.existing_id) {
      onChange({ foodId: hit.existing_id, foodName: hit.name })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lookup.data])

  async function onCreateManual() {
    setCreating(true)
    try {
      const ingr = await createFood({
        kind: "leaf",
        name: ing.name,
        source: "manual",
      })
      void qc.invalidateQueries({ queryKey: foodKeys.lists() })
      onChange({
        resolution: "existing",
        foodId: ingr.id,
        foodName: ingr.name,
      })
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="rounded border border-border p-3">
      <div className="flex items-start justify-between gap-2">
        <div className="flex-1">
          <div className="text-sm font-medium">
            {ing.amount > 0
              ? `${ing.amount} ${ing.unit || ing.original_unit} `
              : ""}
            {ing.name || ing.raw_text}
            {ing.note && (
              <span className="text-muted-foreground"> — {ing.note}</span>
            )}
          </div>
          <div className="text-xs text-muted-foreground">{ing.raw_text}</div>
          {ing.confidence === "unparsed" && (
            <Badge variant="destructive" className="mt-1">
              {t("import.step2.unparsed_warning")}
            </Badge>
          )}
        </div>

        <div className="flex items-center gap-2">
          <Select
            value={state.resolution}
            onValueChange={(v) =>
              onChange({ resolution: v as Resolution, foodId: null })
            }
          >
            <SelectTrigger className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="existing">
                {t("import.step2.res_existing")}
              </SelectItem>
              <SelectItem value="skip">{t("import.step2.res_skip")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {state.resolution === "existing" && (
        <div className="mt-2 space-y-2">
          {state.foodId ? (
            <div className="flex items-center gap-2 text-sm">
              <Badge>{t("import.step2.resolved_label")}</Badge>
              <span>{state.foodName}</span>
              <button
                className="text-xs text-muted-foreground underline underline-offset-2"
                onClick={() => onChange({ foodId: null, foodName: null })}
              >
                {t("import.step2.change")}
              </button>
            </div>
          ) : lookup.isLoading ? (
            <p className="text-xs text-muted-foreground">
              {t("import.step2.searching")}
            </p>
          ) : results.length > 0 ? (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">
                {t("import.step2.pick_match")}
              </p>
              {results.map((r, i) => (
                <button
                  key={i}
                  type="button"
                  className="block w-full rounded border border-border px-2 py-1 text-left text-sm hover:bg-muted"
                  onClick={() =>
                    r.existing_id
                      ? onChange({
                          foodId: r.existing_id,
                          foodName: r.name,
                        })
                      : undefined
                  }
                  disabled={!r.existing_id}
                >
                  {r.name}
                  {!r.existing_id && (
                    <span className="ml-2 text-xs text-muted-foreground">
                      ({t("import.step2.not_in_library")})
                    </span>
                  )}
                </button>
              ))}
              <Button
                variant="outline"
                size="sm"
                onClick={onCreateManual}
                disabled={creating}
              >
                {t("import.step2.create_new")}
              </Button>
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-xs text-muted-foreground">
                {t("import.step2.no_match")}
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={onCreateManual}
                disabled={creating}
              >
                {creating ? t("common.loading") : t("import.step2.create_new")}
              </Button>
            </div>
          )}

          <div className="grid grid-cols-3 gap-2">
            <div>
              <Label className="text-xs">{t("import.step2.amount")}</Label>
              <Input
                type="number"
                step="any"
                value={state.amount}
                onChange={(e) => onChange({ amount: Number(e.target.value) })}
              />
            </div>
            <div>
              <Label className="text-xs">{t("import.step2.unit")}</Label>
              <Select
                value={state.unit}
                onValueChange={(v) => onChange({ unit: v })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="g">g</SelectItem>
                  <SelectItem value="ml">ml</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
