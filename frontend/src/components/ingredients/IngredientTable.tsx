import { useDeferredValue, useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Loader2, Plus, Search } from "lucide-react"

import { RecipesHeader } from "@/components/recipes/RecipesHeader"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import type { LeafFood } from "@/lib/api/foods"
import { SHOP_SECTIONS, type ShopSection } from "@/lib/api/shopping"
import { useFoods, useSetShopSection } from "@/lib/queries/foods"

const PAGE_SIZE = 50
const AUTO = "auto"

/** The Ingredients tab of Recipes (UX §5.8): a dense table with name,
 *  kcal/100 g, the editable store section, how many recipes use it and the
 *  data source. */
export function IngredientTable() {
  const { t } = useTranslation()
  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)

  return (
    <div className="mx-auto flex max-w-7xl flex-col gap-6 px-4 py-6 md:px-8 md:py-10">
      <RecipesHeader
        tab="ingredients"
        action={
          <Button asChild className="min-h-11 md:pointer-fine:min-h-8">
            <Link to="/recipes/ingredients/new" data-testid="ingredient-create">
              <Plus data-icon="inline-start" strokeWidth={1.5} aria-hidden />
              {t("recipes.ingredients.create")}
            </Link>
          </Button>
        }
      />
      <div className="relative w-full sm:max-w-sm">
        <Search
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-muted"
          strokeWidth={1.5}
          aria-hidden
        />
        <Input
          type="search"
          aria-label={t("recipes.ingredients.search")}
          placeholder={t("recipes.ingredients.search")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-11 rounded-cell border-line bg-panel pl-9 text-body md:pointer-fine:h-9"
          data-testid="ingredients-search"
        />
      </div>
      <IngredientRows search={deferredSearch} />
    </div>
  )
}

function IngredientRows({ search }: { search: string }) {
  const { t, i18n } = useTranslation()
  const [page, setPage] = useState({ search, limit: PAGE_SIZE })
  const limit = page.search === search ? page.limit : PAGE_SIZE
  const { data, isLoading, isFetching } = useFoods({
    kind: "leaf",
    search: search || undefined,
    sort: "name",
    limit,
    offset: 0,
  })
  const items = (data?.items ?? []) as LeafFood[]
  const total = data?.total ?? 0
  const kcal = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 0,
  })

  if (isLoading) {
    return (
      <div className="flex flex-col gap-2">
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-11 w-full rounded-cell" />
        ))}
      </div>
    )
  }

  if (items.length === 0) {
    return (
      <div
        className="flex flex-col items-start gap-2 rounded-lg border border-line bg-panel p-6"
        data-testid="ingredients-empty"
      >
        <h2 className="text-h3 font-semibold text-ink">
          {search
            ? t("recipes.ingredients.no_results_title")
            : t("recipes.ingredients.empty_title")}
        </h2>
        <p className="text-body text-ink-muted">
          {search
            ? t("recipes.ingredients.no_results_body")
            : t("recipes.ingredients.empty_body")}
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-small text-ink-muted tabular-nums">
        {t("recipes.ingredients.count", { count: total })}
      </p>
      <div className="overflow-x-auto rounded-lg border border-line bg-panel">
        <table className="w-full text-body" data-testid="ingredients-table">
          <thead className="border-b border-line bg-panel-sunken text-left text-small whitespace-nowrap text-ink-muted">
            <tr>
              <th scope="col" className="px-3 py-2 font-semibold">
                {t("recipes.ingredients.col_name")}
              </th>
              <th scope="col" className="px-3 py-2 text-right font-semibold">
                {t("recipes.ingredients.col_kcal")}
              </th>
              <th scope="col" className="px-3 py-2 font-semibold">
                {t("recipes.ingredients.col_section")}
              </th>
              <th
                scope="col"
                className="hidden px-3 py-2 text-right font-semibold sm:table-cell"
              >
                {t("recipes.ingredients.col_used_in")}
              </th>
              <th
                scope="col"
                className="hidden px-3 py-2 font-semibold sm:table-cell"
              >
                {t("recipes.ingredients.col_source")}
              </th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr
                key={item.id}
                className="border-b border-line last:border-b-0"
                data-testid={`ingredient-row-${item.id}`}
              >
                <th scope="row" className="px-3 py-1 text-left font-normal">
                  <Link
                    to="/recipes/ingredients/$id/edit"
                    params={{ id: String(item.id) }}
                    className="inline-flex min-h-11 items-center text-ink underline-offset-2 hover:underline md:pointer-fine:min-h-8"
                  >
                    {item.name}
                  </Link>
                </th>
                <td className="px-3 py-1 text-right text-ink-muted tabular-nums">
                  {item.kcal_100g == null ? "–" : kcal.format(item.kcal_100g)}
                </td>
                <td className="px-3 py-1">
                  <SectionSelect ingredient={item} />
                </td>
                <td className="hidden px-3 py-1 text-right text-ink-muted tabular-nums sm:table-cell">
                  {t("recipes.ingredients.used_in", {
                    count: item.used_in_count,
                  })}
                </td>
                <td className="hidden px-3 py-1 sm:table-cell">
                  <span className="inline-flex rounded-cell border border-line px-1.5 text-caption text-ink-muted">
                    {t(`recipes.ingredients.source_${item.source ?? "manual"}`)}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {items.length < total && (
        <Button
          variant="outline"
          className="min-h-11 self-center"
          onClick={() => setPage({ search, limit: limit + PAGE_SIZE })}
          disabled={isFetching}
          data-testid="ingredients-load-more"
        >
          {isFetching && <Loader2 className="animate-spin" aria-hidden />}
          {t("recipes.ingredients.load_more")}
        </Button>
      )}
    </div>
  )
}

/** The store section (D18). "Automatic" clears the user's choice and hands
 *  the leaf back to the classifier. */
function SectionSelect({ ingredient }: { ingredient: LeafFood }) {
  const { t } = useTranslation()
  const setSection = useSetShopSection()
  const auto = ingredient.shop_section_source === "auto"

  return (
    <Select
      value={auto ? AUTO : ingredient.shop_section}
      onValueChange={(v) =>
        setSection.mutate({
          id: ingredient.id,
          section: v === AUTO ? null : (v as ShopSection),
        })
      }
      disabled={setSection.isPending}
    >
      <SelectTrigger
        aria-label={t("recipes.ingredients.section_for", {
          name: ingredient.name,
        })}
        className="min-h-11 w-full rounded-cell border-line bg-transparent text-body data-[size=default]:h-auto max-sm:whitespace-normal sm:min-w-40 md:pointer-fine:min-h-8"
        data-testid={`ingredient-section-${ingredient.id}`}
      >
        <span className="text-left sm:truncate">
          {t(`shop.section.${ingredient.shop_section}`)}
          {/* Below sm the label may wrap and the hint is read out but not
              shown, so the select fits a phone without scrolling the table. */}
          {auto && (
            <span className="ml-1.5 text-caption text-ink-muted max-sm:sr-only">
              {t("recipes.ingredients.section_auto_hint")}
            </span>
          )}
        </span>
      </SelectTrigger>
      <SelectContent position="popper">
        <SelectItem value={AUTO}>
          {t("recipes.ingredients.section_auto")}
        </SelectItem>
        {SHOP_SECTIONS.map((s) => (
          <SelectItem key={s} value={s}>
            {t(`shop.section.${s}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
