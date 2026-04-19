import { useMemo, useState, useDeferredValue } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Loader2, Package, Plus } from "lucide-react"

import {
  FilterChipGroup,
  type FilterChipOption,
} from "@/components/editorial/FilterChipGroup"
import { PageHeader } from "@/components/editorial/PageHeader"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { useIngredients, useDeleteIngredient } from "@/lib/queries/ingredients"

import { IngredientCard } from "./IngredientCard"

const PAGE_SIZE = 16

export function IngredientList() {
  const { t } = useTranslation()

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [sort, setSort] = useState<string>("name")
  const [limit, setLimit] = useState(PAGE_SIZE)
  const [deleteId, setDeleteId] = useState<number | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const { data, isLoading, isFetching } = useIngredients({
    search: deferredSearch || undefined,
    sort,
    limit,
    offset: 0,
  })

  const deleteMutation = useDeleteIngredient()

  function handleDelete() {
    if (deleteId === null) return
    deleteMutation.mutate(deleteId, {
      onSuccess: () => setDeleteId(null),
      onError: (err: unknown) => {
        const key = err instanceof Error ? err.message : "error.server"
        setDeleteError(t(key))
      },
    })
  }

  const total = data?.total ?? 0
  const items = data?.items ?? []
  const hasMore = items.length < total

  const sortOptions: FilterChipOption[] = useMemo(
    () => [
      { value: "name", label: t("ingredient.sort_name") },
      { value: "kcal", label: t("ingredient.sort_kcal") },
      { value: "protein", label: t("ingredient.sort_protein") },
    ],
    [t]
  )

  return (
    <div className="mx-auto max-w-7xl space-y-6 px-4 py-8 md:px-8 md:py-12">
      <PageHeader
        title={t("ingredient.title")}
        description={t("ingredient.subtitle")}
        actions={
          <Button
            asChild
            className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
          >
            <Link to="/ingredients/new">
              <Plus className="mr-1.5 size-4" aria-hidden />
              {t("ingredient.create")}
            </Link>
          </Button>
        }
      />

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Input
          placeholder={t("ingredient.search_placeholder")}
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setLimit(PAGE_SIZE)
          }}
          className="flex-1 rounded-full bg-surface-container-highest"
          data-testid="inventory-search"
        />
        <FilterChipGroup
          testId="ingredient-sort"
          ariaLabel={t("ingredient.sort_label")}
          options={sortOptions}
          value={sort}
          onValueChange={(v) => {
            if (v) setSort(v)
          }}
          allowDeselect={false}
        />
      </div>

      {!isLoading && total > 0 && (
        <p className="text-xs tracking-wider text-on-surface-variant uppercase">
          {t("ingredient.total_count", { count: total })}
        </p>
      )}

      {isLoading ? (
        <div className="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="aspect-[4/3] w-full rounded-2xl" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyIngredients hasFilters={deferredSearch.length > 0} t={t} />
      ) : (
        <>
          <div className="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {items.map((item) => (
              <IngredientCard
                key={item.id}
                ingredient={item}
                onDelete={setDeleteId}
              />
            ))}
          </div>

          {hasMore && (
            <div className="flex justify-center">
              <Button
                variant="outline"
                onClick={() => setLimit((l) => l + PAGE_SIZE)}
                disabled={isFetching}
                data-testid="ingredients-load-more"
              >
                {isFetching && <Loader2 className="mr-2 size-4 animate-spin" />}
                {t("ingredient.load_more")}
              </Button>
            </div>
          )}
        </>
      )}

      <Dialog
        open={deleteId !== null}
        onOpenChange={() => {
          setDeleteId(null)
          setDeleteError(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("ingredient.delete_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("ingredient.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          {deleteError && (
            <p className="px-1 text-sm text-destructive">{deleteError}</p>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setDeleteId(null)
                setDeleteError(null)
              }}
            >
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteMutation.isPending}
              data-testid="confirm-delete"
            >
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function EmptyIngredients({
  hasFilters,
  t,
}: {
  hasFilters: boolean
  t: (k: string) => string
}) {
  return (
    <div
      className="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed border-outline-variant/40 bg-surface-container-lowest px-6 py-20 text-center"
      data-testid="ingredient-create-tile"
    >
      <div className="flex size-14 items-center justify-center rounded-full bg-surface-container">
        <Package className="size-6 text-on-surface-variant" aria-hidden />
      </div>
      <div className="space-y-1">
        <p className="font-heading text-lg font-bold text-on-surface">
          {hasFilters
            ? t("ingredient.no_results")
            : t("ingredient.empty_title")}
        </p>
        <p className="text-sm text-on-surface-variant">
          {hasFilters
            ? t("ingredient.no_results_body")
            : t("ingredient.empty_state")}
        </p>
      </div>
      <Button
        asChild
        className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
      >
        <Link to="/ingredients/new">
          <Plus className="mr-1.5 size-4" aria-hidden />
          {t("ingredient.create")}
        </Link>
      </Button>
    </div>
  )
}
