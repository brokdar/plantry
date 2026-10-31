import { useDeferredValue, useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Loader2, Search } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import type { ComposedFood, FoodRole } from "@/lib/api/foods"
import { todayISO } from "@/lib/dates"
import { useFoods } from "@/lib/queries/foods"
import { useSetupStatus } from "@/lib/queries/setup"
import {
  DIET_FILTERS,
  hasLibraryFilters,
  LIBRARY_SORTS,
  libraryQuery,
  type DietFilter,
  type LibrarySearch,
  type LibrarySort,
} from "@/lib/recipes/library"
import { FOOD_ROLES } from "@/lib/schemas/food"
import { cn } from "@/lib/utils"

import { AddRecipeMenu } from "./AddRecipeMenu"
import { DietIcon } from "./DietIcon"
import { ImportSheet } from "./ImportSheet"
import { RecipeCard } from "./RecipeCard"
import { RecipeOnboarding } from "./RecipeOnboarding"
import { RecipesHeader } from "./RecipesHeader"

const PAGE_SIZE = 24
const GRID = "grid grid-cols-2 gap-x-4 gap-y-6 md:grid-cols-3 lg:grid-cols-4"
const ALL_ROLES = "all"
const ALL_DIETS = "all"

type FlagFilter = "quick" | "stale" | "fav" | "new"
const FLAG_FILTERS: { key: FlagFilter; label: string }[] = [
  { key: "quick", label: "recipes.library.quick" },
  { key: "stale", label: "recipes.library.stale" },
  { key: "fav", label: "recipes.library.favorites" },
  { key: "new", label: "recipes.library.new" },
]

type RecipeLibraryProps = {
  /** The filters, from the `/recipes` search params. */
  search: LibrarySearch
  /** Writes the filters back to the URL. `replace` for typing. */
  onSearchChange: (next: LibrarySearch, replace?: boolean) => void
}

/** The recipe library (UX §5.8, D6): filter chips, search and sort over
 *  `GET /api/foods`, the guided state below the recipe target, and the
 *  import sheet. */
export function RecipeLibrary({ search, onSearchChange }: RecipeLibraryProps) {
  const { t } = useTranslation()
  const [today] = useState(todayISO)
  const deferredQ = useDeferredValue(search.q)
  const query = libraryQuery({ ...search, q: deferredQ })
  const filtered = hasLibraryFilters(search)
  const setup = useSetupStatus()
  const guided =
    setup.data !== undefined &&
    setup.data.recipe_count < setup.data.recipe_target

  const set = (patch: Partial<LibrarySearch>, replace?: boolean) =>
    onSearchChange({ ...search, ...patch }, replace)

  return (
    <div className="mx-auto flex max-w-7xl flex-col gap-6 px-4 py-6 md:px-8 md:py-10">
      <RecipesHeader tab="recipes" action={<AddRecipeMenu />} />

      {setup.data && (
        <RecipeOnboarding
          count={setup.data.recipe_count}
          target={setup.data.recipe_target}
        />
      )}

      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <SearchField
            value={search.q ?? ""}
            onChange={(q) => set({ q: q || undefined }, true)}
          />
          <Select
            value={search.sort ?? "name"}
            onValueChange={(v) =>
              set({ sort: v === "name" ? undefined : (v as LibrarySort) })
            }
          >
            <SelectTrigger
              aria-label={t("recipes.library.sort")}
              className="h-11 rounded-cell border-line bg-panel text-body data-[size=default]:h-11 md:pointer-fine:data-[size=default]:h-9"
              data-testid="recipes-sort"
            >
              <span className="text-ink-muted">
                {t("recipes.library.sort")}:
              </span>
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper" align="end">
              {LIBRARY_SORTS.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`recipes.library.sort_${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div
          role="group"
          aria-label={t("recipes.library.filters")}
          className="flex flex-wrap items-center gap-2"
        >
          <Select
            value={search.role ?? ALL_ROLES}
            onValueChange={(v) =>
              set({ role: v === ALL_ROLES ? undefined : (v as FoodRole) })
            }
          >
            <SelectTrigger
              aria-label={t("recipes.library.role")}
              className={cn(
                chipClass(!!search.role),
                "data-[size=default]:h-auto"
              )}
              data-testid="recipes-filter-role"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper">
              <SelectItem value={ALL_ROLES}>
                {t("recipes.library.role_all")}
              </SelectItem>
              {FOOD_ROLES.map((role) => (
                <SelectItem key={role} value={role}>
                  {t(`component.role_${role}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={search.diet ?? ALL_DIETS}
            onValueChange={(v) =>
              set({ diet: v === ALL_DIETS ? undefined : (v as DietFilter) })
            }
          >
            <SelectTrigger
              aria-label={t("diet.label")}
              className={cn(
                chipClass(!!search.diet),
                "data-[size=default]:h-auto"
              )}
              data-testid="recipes-filter-diet"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent position="popper">
              <SelectItem value={ALL_DIETS}>{t("diet.filter_all")}</SelectItem>
              {DIET_FILTERS.map((d) => (
                <SelectItem key={d} value={d}>
                  <DietIcon dietClass={d} withLabel />
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {FLAG_FILTERS.map(({ key, label }) => (
            <button
              key={key}
              type="button"
              aria-pressed={!!search[key]}
              onClick={() => set({ [key]: search[key] ? undefined : true })}
              className={chipClass(!!search[key])}
              data-testid={`recipes-filter-${key}`}
            >
              {t(label)}
            </button>
          ))}
          {filtered && (
            <Button
              variant="ghost"
              className="min-h-11 text-ink-muted md:pointer-fine:min-h-8"
              onClick={() => onSearchChange({ sort: search.sort })}
              data-testid="recipes-clear-filters"
            >
              {t("recipes.library.clear")}
            </Button>
          )}
        </div>
      </div>

      <LibraryResults
        query={query}
        today={today}
        filtered={filtered}
        // An empty, unfiltered library is covered by the onboarding cards.
        showEmpty={filtered || (!guided && !setup.isPending)}
      />

      <ImportSheet
        open={search.add === "url"}
        onClose={() => set({ add: undefined })}
      />
    </div>
  )
}

/**
 * The search box keeps its own text, so the caret stays put while the URL
 * (which the router updates asynchronously) catches up. It takes the URL's
 * value only when that changes from outside: Clear filters, back/forward.
 * Values it wrote itself are recognised on arrival and ignored.
 */
function SearchField({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(value)
  const [sync, setSync] = useState({ value, pending: [] as string[] })
  if (value !== sync.value) {
    const own = sync.pending.indexOf(value)
    if (own >= 0) {
      setSync({ value, pending: sync.pending.slice(own + 1) })
    } else {
      setSync({ value, pending: [] })
      setDraft(value)
    }
  }

  return (
    <div className="relative min-w-48 flex-1 sm:max-w-sm">
      <Search
        className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-muted"
        strokeWidth={1.5}
        aria-hidden
      />
      <Input
        type="search"
        aria-label={t("recipes.library.search")}
        placeholder={t("recipes.library.search")}
        value={draft}
        onChange={(e) => {
          const next = e.target.value
          setDraft(next)
          setSync((s) => ({ ...s, pending: [...s.pending, next] }))
          onChange(next)
        }}
        className="h-11 rounded-cell border-line bg-panel pl-9 text-body md:pointer-fine:h-9"
        data-testid="recipes-search"
      />
    </div>
  )
}

function chipClass(pressed: boolean) {
  return cn(
    "inline-flex min-h-11 items-center gap-1.5 rounded-lg border px-3 text-small transition-colors duration-150 ease-out md:pointer-fine:min-h-8",
    pressed
      ? "border-accent bg-accent font-semibold text-on-accent"
      : "border-line bg-panel text-ink hover:border-ink-muted"
  )
}

/** The result grid. "Show more" grows the page; a filter change starts
 *  over at one page. */
function LibraryResults({
  query,
  today,
  filtered,
  showEmpty,
}: {
  query: ReturnType<typeof libraryQuery>
  today: string
  filtered: boolean
  showEmpty: boolean
}) {
  const { t } = useTranslation()
  const queryKey = JSON.stringify(query)
  const [page, setPage] = useState({ queryKey, limit: PAGE_SIZE })
  const limit = page.queryKey === queryKey ? page.limit : PAGE_SIZE
  const { data, isLoading, isFetching } = useFoods({
    ...query,
    limit,
    offset: 0,
  })
  const items = (data?.items ?? []) as ComposedFood[]
  const total = data?.total ?? 0

  if (isLoading) {
    return (
      <div className={GRID}>
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={i} className="flex flex-col gap-2">
            <Skeleton className="aspect-[4/3] w-full rounded-lg" />
            <Skeleton className="h-5 w-3/4" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        ))}
      </div>
    )
  }

  if (items.length === 0) {
    if (!showEmpty) return null
    return (
      <div
        className="flex flex-col items-start gap-2 rounded-lg border border-line bg-panel p-6"
        data-testid="recipes-empty"
      >
        <h2 className="text-h3 font-semibold text-ink">
          {filtered
            ? t("recipes.library.no_results_title")
            : t("recipes.library.empty_title")}
        </h2>
        <p className="text-body text-ink-muted">
          {filtered
            ? t("recipes.library.no_results_body")
            : t("recipes.library.empty_body")}
        </p>
        {!filtered && (
          <Button asChild variant="outline" className="mt-2 min-h-11">
            <Link to="/recipes/new">{t("recipes.add.create")}</Link>
          </Button>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <p className="text-small text-ink-muted tabular-nums">
        {t("recipes.library.count", { count: total })}
      </p>
      <ul className={GRID} data-testid="recipes-grid">
        {items.map((item) => (
          <li key={item.id}>
            <RecipeCard recipe={item} today={today} />
          </li>
        ))}
      </ul>
      {items.length < total && (
        <Button
          variant="outline"
          className="min-h-11 self-center"
          onClick={() => setPage({ queryKey, limit: limit + PAGE_SIZE })}
          disabled={isFetching}
          data-testid="recipes-load-more"
        >
          {isFetching && <Loader2 className="animate-spin" aria-hidden />}
          {t("recipes.library.load_more")}
        </Button>
      )}
    </div>
  )
}
