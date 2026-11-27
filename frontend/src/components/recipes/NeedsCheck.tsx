import { useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { NewLeafForm } from "@/components/import/IngredientResolveRow"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet"
import {
  leafInputFromCandidate,
  type NeedsCheck,
  type NeedsCheckLine,
} from "@/lib/api/foods"
import type { LookupCandidate } from "@/lib/api/lookup"
import { useFixLeafNutrition, useNeedsCheck } from "@/lib/queries/foods"
import { useImportLineLookup } from "@/lib/queries/import"

/** The quiet count line of the Recipes screen (never a banner): absent when
 *  nothing needs a check, otherwise opens the list. */
export function NeedsCheckEntry() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const { data } = useNeedsCheck()
  if (!data || data.count === 0) return null
  return (
    <>
      <button
        type="button"
        data-testid="needs-check-entry"
        className="w-fit text-small text-ink-muted underline underline-offset-2 hover:text-ink"
        onClick={() => setOpen(true)}
      >
        {t("recipes.needs_check.entry", { count: data.count })}
      </button>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className="w-full gap-4 overflow-y-auto bg-panel p-4 text-ink sm:max-w-xl">
          <SheetTitle>{t("recipes.needs_check.title")}</SheetTitle>
          <SheetDescription>{t("recipes.needs_check.intro")}</SheetDescription>
          <NeedsCheckList data={data} />
        </SheetContent>
      </Sheet>
    </>
  )
}

function NeedsCheckList({ data }: { data: NeedsCheck }) {
  const usedIn = new Map(data.leaves.map((l) => [l.id, l.recipes.length]))
  return (
    <ul className="flex flex-col gap-4" data-testid="needs-check-list">
      {data.recipes.map((r) => (
        <li key={r.id} className="flex flex-col gap-2">
          <RecipeHeading id={r.id} name={r.name} />
          <ul className="flex flex-col gap-2">
            {(r.lines ?? []).map((line) => (
              <LineFix
                key={`${r.id}-${line.leaf_id}`}
                line={line}
                recipeCount={usedIn.get(line.leaf_id) ?? 1}
              />
            ))}
          </ul>
        </li>
      ))}
    </ul>
  )
}

function RecipeHeading({ id, name }: { id: number; name: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-baseline justify-between gap-3">
      <Link
        to="/recipes/$id"
        params={{ id: String(id) }}
        className="font-semibold text-ink underline-offset-2 hover:underline"
      >
        {name}
      </Link>
      <Link
        to="/recipes/$id/edit"
        params={{ id: String(id) }}
        className="shrink-0 text-xs text-ink-muted underline underline-offset-2"
      >
        {t("recipes.needs_check.remove_in_recipe")}
      </Link>
    </div>
  )
}

type Mode = "search" | "enter" | null

function LineFix({
  line,
  recipeCount,
}: {
  line: NeedsCheckLine
  recipeCount: number
}) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<Mode>(null)
  return (
    <li className="rounded border border-line p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span>
          <span className="font-medium">{line.leaf_name}</span>{" "}
          <span className="text-ink-muted">
            {line.amount} {line.unit} · {t("recipes.needs_check.no_nutrition")}
          </span>
        </span>
        <span className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            aria-expanded={mode === "search"}
            onClick={() => setMode(mode === "search" ? null : "search")}
          >
            {t("recipes.needs_check.search_again")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            aria-expanded={mode === "enter"}
            onClick={() => setMode(mode === "enter" ? null : "enter")}
          >
            {t("recipes.needs_check.enter")}
          </Button>
        </span>
      </div>
      {recipeCount > 1 && (
        <p className="mt-1 text-xs text-ink-muted">
          {t("recipes.needs_check.shared", { count: recipeCount })}
        </p>
      )}
      {mode === "search" && <SearchFix line={line} />}
      {mode === "enter" && (
        <EnterFix line={line} onCancel={() => setMode(null)} />
      )}
    </li>
  )
}

function SearchFix({ line }: { line: NeedsCheckLine }) {
  const { t, i18n } = useTranslation()
  const fix = useFixLeafNutrition()
  const [text, setText] = useState(line.leaf_name)
  const [query, setQuery] = useState(line.leaf_name)
  const lang = i18n.language.startsWith("de") ? "de" : "en"
  const lookup = useImportLineLookup(query, lang)
  // Library leaves are other ingredients; only a catalogue source supplies
  // nutrition for this one.
  const results = (lookup.data?.results ?? []).filter(
    (r) => r.source !== "local"
  )

  function pick(c: LookupCandidate) {
    fix.mutate({
      id: line.leaf_id,
      data: { ...leafInputFromCandidate(c), name: line.leaf_name },
    })
  }

  return (
    <div className="mt-2 flex flex-col gap-2">
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          setQuery(text.trim())
        }}
      >
        <Input
          aria-label={t("recipes.needs_check.search_for", {
            name: line.leaf_name,
          })}
          value={text}
          onChange={(e) => setText(e.target.value)}
          className="h-9"
        />
        <Button type="submit" variant="outline" size="sm">
          {t("import.step2.search")}
        </Button>
      </form>
      {lookup.isLoading ? (
        <p className="text-xs text-ink-muted">{t("import.step2.searching")}</p>
      ) : results.length > 0 ? (
        <div className="flex flex-col gap-1">
          <p className="text-xs text-ink-muted">
            {t("recipes.needs_check.pick")}
          </p>
          {results.map((r, i) => (
            <button
              key={`${r.source}-${r.fdc_id ?? r.barcode ?? r.name}-${i}`}
              type="button"
              disabled={fix.isPending}
              className="flex min-h-9 w-full items-baseline justify-between gap-3 rounded border border-line px-2 py-1 text-left hover:bg-panel-sunken disabled:opacity-60"
              onClick={() => pick(r)}
            >
              <span>{r.source_name || r.name}</span>
              <span className="flex shrink-0 gap-3 text-xs text-ink-muted tabular-nums">
                {r.kcal_100g != null && (
                  <span>
                    {t("import.step2.kcal_100g", {
                      kcal: Math.round(r.kcal_100g),
                    })}
                  </span>
                )}
                <span>{t(`import.step2.source_${r.source}`)}</span>
              </span>
            </button>
          ))}
        </div>
      ) : (
        <p className="text-xs text-ink-muted">{t("import.step2.no_match")}</p>
      )}
    </div>
  )
}

function EnterFix({
  line,
  onCancel,
}: {
  line: NeedsCheckLine
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const fix = useFixLeafNutrition()
  return (
    <div className="mt-2">
      <NewLeafForm
        name={line.leaf_name}
        pending={fix.isPending}
        hint={t("recipes.needs_check.enter_hint", { name: line.leaf_name })}
        submitLabel={t("recipes.needs_check.save_nutrition")}
        onCancel={onCancel}
        onCreate={(macros) =>
          fix.mutate({
            id: line.leaf_id,
            data: {
              kind: "leaf",
              name: line.leaf_name,
              source: "manual",
              ...macros,
            },
          })
        }
      />
    </div>
  )
}
