import { useState, useDeferredValue } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { MoreVertical, Plus } from "lucide-react"

import { EditorialCard } from "@/components/editorial/EditorialCard"
import { EmptyCreateCard } from "@/components/editorial/EmptyCreateCard"
import {
  FilterChipGroup,
  type FilterChipOption,
} from "@/components/editorial/FilterChipGroup"
import { FoodPlaceholder } from "@/components/editorial/FoodPlaceholder"
import { PageHeader } from "@/components/editorial/PageHeader"
import { MacroBar } from "@/components/editorial/MacroBar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { imageURL } from "@/lib/image-url"
import { useIngredients, useDeleteIngredient } from "@/lib/queries/ingredients"

const PAGE_SIZE = 20

const INGREDIENT_SOURCES = ["manual", "off", "fdc"] as const
type IngredientSource = (typeof INGREDIENT_SOURCES)[number]

export function IngredientList() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [sourceFilter, setSourceFilter] = useState<string | null>(null)
  const [offset, setOffset] = useState(0)
  const [deleteId, setDeleteId] = useState<number | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const { data, isLoading } = useIngredients({
    search: deferredSearch || undefined,
    limit: PAGE_SIZE,
    offset,
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
  const itemsRaw = data?.items ?? []
  const items = sourceFilter
    ? itemsRaw.filter((i) => i.source === sourceFilter)
    : itemsRaw
  const from = total > 0 ? offset + 1 : 0
  const to = Math.min(offset + PAGE_SIZE, total)

  const sourceOptions: FilterChipOption[] = INGREDIENT_SOURCES.map(
    (source) => ({
      value: source,
      label: t(`ingredient.source_${source}`),
      testId: `ingredient-filter-source-${source}`,
    })
  )

  return (
    <div className="mx-auto max-w-7xl space-y-8 px-4 py-8 md:px-8 md:py-12">
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

      <div className="space-y-4">
        <Input
          placeholder={t("ingredient.search_placeholder")}
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setOffset(0)
          }}
          className="max-w-sm rounded-full bg-surface-container-highest"
          data-testid="inventory-search"
        />
        <FilterChipGroup
          testId="ingredient-filter-source"
          ariaLabel={t("ingredient.source_label")}
          options={sourceOptions}
          value={sourceFilter}
          onValueChange={setSourceFilter}
        />
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 gap-8 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-64 w-full rounded-xl" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="grid grid-cols-1 gap-8 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          <EmptyCreateCard
            to="/ingredients/new"
            title={t("ingredient.create")}
            description={
              deferredSearch || sourceFilter
                ? t("ingredient.no_results")
                : t("ingredient.empty_state")
            }
            testId="ingredient-create-tile"
          />
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-8 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {items.map((item) => (
              <div
                key={item.id}
                className="relative"
                data-testid={`ingredient-card-${item.id}`}
              >
                <EditorialCard interactive>
                  <Link
                    to="/ingredients/$id/edit"
                    params={{ id: String(item.id) }}
                    className="absolute inset-0 z-0"
                    aria-label={item.name}
                  />
                  {item.image_path ? (
                    <EditorialCard.Image
                      src={imageURL(item.image_path, item.updated_at)}
                      alt={item.name}
                    />
                  ) : (
                    <FoodPlaceholder
                      category="ingredient"
                      className="m-2 aspect-[4/3] w-[calc(100%-1rem)]"
                      aria-label={item.name}
                    />
                  )}
                  <EditorialCard.Body>
                    <EditorialCard.Title>{item.name}</EditorialCard.Title>
                    <EditorialCard.Meta className="mt-2">
                      <Badge variant="secondary" className="text-[10px]">
                        {t(
                          `ingredient.source_${item.source as IngredientSource}`,
                          {
                            defaultValue: item.source,
                          }
                        )}
                      </Badge>
                      <span>{item.kcal_100g} kcal / 100g</span>
                    </EditorialCard.Meta>
                    <div className="mt-3 grid grid-cols-3 gap-2 text-xs text-on-surface-variant">
                      <MacroRow
                        label={t("ingredient.protein")}
                        value={item.protein_100g}
                        color="primary"
                      />
                      <MacroRow
                        label={t("ingredient.carbs")}
                        value={item.carbs_100g}
                        color="tertiary"
                      />
                      <MacroRow
                        label={t("ingredient.fat")}
                        value={item.fat_100g}
                        color="secondary"
                      />
                    </div>
                  </EditorialCard.Body>
                </EditorialCard>
                <div className="absolute top-3 right-3 z-10">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t("common.actions")}
                        className="bg-surface-container-lowest/80 backdrop-blur-sm"
                        data-testid={`ingredient-card-${item.id}-menu`}
                      >
                        <MoreVertical className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={() =>
                          navigate({
                            to: "/ingredients/$id/edit",
                            params: { id: String(item.id) },
                          })
                        }
                      >
                        {t("common.edit")}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() => setDeleteId(item.id)}
                        className="text-destructive focus:text-destructive"
                        data-testid={`ingredient-card-${item.id}-delete`}
                      >
                        {t("common.delete")}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
            ))}
            {offset + PAGE_SIZE >= total && (
              <EmptyCreateCard
                to="/ingredients/new"
                title={t("ingredient.create")}
                testId="ingredient-create-tile"
              />
            )}
          </div>

          <div className="flex items-center justify-between">
            <p className="text-sm text-muted-foreground">
              {t("common.showing", { from, to, total })}
            </p>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={offset === 0}
                onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
                data-testid="pagination-prev"
              >
                {t("common.previous")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={offset + PAGE_SIZE >= total}
                onClick={() => setOffset((o) => o + PAGE_SIZE)}
                data-testid="pagination-next"
              >
                {t("common.next")}
              </Button>
            </div>
          </div>
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

function MacroRow({
  label,
  value,
  color,
}: {
  label: string
  value: number
  color: "primary" | "tertiary" | "secondary"
}) {
  return (
    <div className="space-y-1">
      <div className="flex items-baseline justify-between gap-1">
        <span className="text-[10px] font-medium tracking-wider uppercase">
          {label}
        </span>
        <span className="font-heading text-sm font-bold text-on-surface">
          {value.toFixed(0)}
        </span>
      </div>
      <MacroBar segments={[{ value, color }]} max={50} thickness="sm" />
    </div>
  )
}
