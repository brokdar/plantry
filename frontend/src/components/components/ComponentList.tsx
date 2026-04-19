import { useMemo, useState, useDeferredValue } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import {
  Bookmark,
  Download,
  LayoutGrid,
  List,
  Loader2,
  MoreHorizontal,
  Plus,
  Utensils,
} from "lucide-react"

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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  useComponents,
  useDeleteComponent,
  useInsights,
} from "@/lib/queries/components"
import { useLocalStorageState } from "@/lib/hooks/useLocalStorageState"
import { COMPONENT_ROLES } from "@/lib/schemas/component"
import { cn } from "@/lib/utils"

import { ComponentCard, type ComponentCardLayout } from "./ComponentCard"

const PAGE_SIZE = 16

const LAYOUT_VALUES = ["grid", "list"] as const
function isLayout(value: string): value is ComponentCardLayout {
  return (LAYOUT_VALUES as readonly string[]).includes(value)
}

export function ComponentList() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [sort, setSort] = useState<string>("name")
  const [roleFilter, setRoleFilter] = useState<string | null>(null)
  const [tagFilter, setTagFilter] = useState<string | null>(null)
  const [limit, setLimit] = useState(PAGE_SIZE)
  const [deleteId, setDeleteId] = useState<number | null>(null)
  const [layout, setLayout] = useLocalStorageState<ComponentCardLayout>(
    "plantry.componentLayout",
    "grid",
    isLayout
  )

  const { data, isLoading, isFetching } = useComponents({
    search: deferredSearch || undefined,
    role: roleFilter ?? undefined,
    tag: tagFilter ?? undefined,
    sort,
    limit,
    offset: 0,
  })

  const deleteMutation = useDeleteComponent()
  const { data: insights } = useInsights()
  const forgottenIds = useMemo(
    () => new Set(insights?.forgotten.map((c) => c.id) ?? []),
    [insights]
  )
  const mostCookedIds = useMemo(
    () => new Set(insights?.most_cooked.map((c) => c.id) ?? []),
    [insights]
  )

  function handleDelete() {
    if (deleteId === null) return
    deleteMutation.mutate(deleteId, {
      onSuccess: () => setDeleteId(null),
    })
  }

  const total = data?.total ?? 0
  const items = useMemo(() => data?.items ?? [], [data])
  const hasMore = items.length < total

  const sortOptions: FilterChipOption[] = useMemo(
    () => [
      { value: "name", label: t("component.sort_name") },
      { value: "created", label: t("component.sort_created") },
    ],
    [t]
  )

  const roleOptions: FilterChipOption[] = COMPONENT_ROLES.map((role) => ({
    value: role,
    label: t(`component.role_${role}`),
    testId: `component-filter-role-${role}`,
  }))

  const availableTags = useMemo(() => {
    const set = new Set<string>()
    for (const item of items) for (const tag of item.tags) set.add(tag)
    return [...set].sort()
  }, [items])

  const tagOptions: FilterChipOption[] = availableTags.map((tag) => ({
    value: tag,
    label: tag,
    testId: `component-filter-tag-${tag}`,
  }))

  return (
    <div className="mx-auto max-w-7xl space-y-6 px-4 py-8 md:px-8 md:py-12">
      <PageHeader
        title={t("component.title")}
        description={t("component.subtitle")}
        actions={
          <>
            <Button
              asChild
              className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
            >
              <Link to="/components/new">
                <Plus className="mr-1.5 size-4" aria-hidden />
                {t("component.create")}
              </Link>
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t("common.actions")}
                  data-testid="catalog-secondary-actions"
                >
                  <MoreHorizontal className="size-5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  onClick={() => navigate({ to: "/import" })}
                  data-testid="catalog-menu-import"
                >
                  <Download className="size-4" />
                  {t("component.import_from_url")}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => navigate({ to: "/templates" })}
                  data-testid="catalog-menu-templates"
                >
                  <Bookmark className="size-4" />
                  {t("component.browse_templates")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </>
        }
      />

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Input
          placeholder={t("component.search_placeholder")}
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setLimit(PAGE_SIZE)
          }}
          className="flex-1 rounded-full bg-surface-container-highest"
          data-testid="catalog-search"
        />
        <FilterChipGroup
          testId="component-sort"
          ariaLabel={t("component.sort_label")}
          options={sortOptions}
          value={sort}
          onValueChange={(v) => {
            if (v) setSort(v)
          }}
          allowDeselect={false}
        />
        <LayoutToggle value={layout} onChange={setLayout} />
      </div>

      <div className="space-y-3">
        <FilterChipGroup
          testId="component-filter-role"
          ariaLabel={t("component.role")}
          options={roleOptions}
          value={roleFilter}
          onValueChange={(v) => {
            setRoleFilter(v)
            setLimit(PAGE_SIZE)
          }}
        />
        {tagOptions.length > 0 && (
          <FilterChipGroup
            testId="component-filter-tag"
            ariaLabel={t("component.tags")}
            options={tagOptions}
            value={tagFilter}
            onValueChange={(v) => {
              setTagFilter(v)
              setLimit(PAGE_SIZE)
            }}
          />
        )}
      </div>

      {!isLoading && total > 0 && (
        <p className="text-xs tracking-wider text-on-surface-variant uppercase">
          {t("component.total_count", { count: total })}
        </p>
      )}

      {isLoading ? (
        <ListSkeleton layout={layout} />
      ) : items.length === 0 ? (
        <EmptyState
          hasFilters={!!deferredSearch || !!roleFilter || !!tagFilter}
          t={t}
        />
      ) : (
        <>
          <div
            className={cn(
              layout === "grid"
                ? "grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
                : "flex flex-col gap-3"
            )}
          >
            {items.map((item) => (
              <ComponentCard
                key={item.id}
                component={item}
                layout={layout}
                insightFlags={{
                  forgotten: forgottenIds.has(item.id),
                  mostCooked: mostCookedIds.has(item.id),
                }}
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
                data-testid="components-load-more"
              >
                {isFetching && <Loader2 className="mr-2 size-4 animate-spin" />}
                {t("component.load_more")}
              </Button>
            </div>
          )}
        </>
      )}

      <Dialog open={deleteId !== null} onOpenChange={() => setDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("component.delete_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("component.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteId(null)}>
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

function LayoutToggle({
  value,
  onChange,
}: {
  value: ComponentCardLayout
  onChange: (next: ComponentCardLayout) => void
}) {
  const { t } = useTranslation()
  return (
    <div
      role="group"
      aria-label={t("component.layout_label")}
      className="inline-flex items-center rounded-full bg-surface-container-highest p-0.5"
    >
      <button
        type="button"
        onClick={() => onChange("grid")}
        aria-pressed={value === "grid"}
        aria-label={t("component.layout_grid")}
        data-testid="component-layout-grid"
        className={cn(
          "inline-flex size-8 items-center justify-center rounded-full transition-colors",
          value === "grid"
            ? "bg-surface-container-lowest text-on-surface shadow-sm"
            : "text-on-surface-variant hover:text-on-surface"
        )}
      >
        <LayoutGrid className="size-4" aria-hidden />
      </button>
      <button
        type="button"
        onClick={() => onChange("list")}
        aria-pressed={value === "list"}
        aria-label={t("component.layout_list")}
        data-testid="component-layout-list"
        className={cn(
          "inline-flex size-8 items-center justify-center rounded-full transition-colors",
          value === "list"
            ? "bg-surface-container-lowest text-on-surface shadow-sm"
            : "text-on-surface-variant hover:text-on-surface"
        )}
      >
        <List className="size-4" aria-hidden />
      </button>
    </div>
  )
}

function ListSkeleton({ layout }: { layout: ComponentCardLayout }) {
  if (layout === "list") {
    return (
      <div className="flex flex-col gap-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-24 w-full rounded-2xl" />
        ))}
      </div>
    )
  }
  return (
    <div className="grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {Array.from({ length: 8 }).map((_, i) => (
        <Skeleton key={i} className="aspect-[4/3] w-full rounded-2xl" />
      ))}
    </div>
  )
}

function EmptyState({
  hasFilters,
  t,
}: {
  hasFilters: boolean
  t: (k: string) => string
}) {
  return (
    <div
      className="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed border-outline-variant/40 bg-surface-container-lowest px-6 py-20 text-center"
      data-testid="component-create-tile"
    >
      <div className="flex size-14 items-center justify-center rounded-full bg-surface-container">
        <Utensils className="size-6 text-on-surface-variant" aria-hidden />
      </div>
      <div className="space-y-1">
        <p className="font-heading text-lg font-bold text-on-surface">
          {hasFilters ? t("component.no_results") : t("component.empty_title")}
        </p>
        <p className="text-sm text-on-surface-variant">
          {hasFilters
            ? t("component.no_results_body")
            : t("component.empty_state")}
        </p>
      </div>
      <Button
        asChild
        className="gradient-primary editorial-shadow border-0 text-on-primary hover:opacity-90"
      >
        <Link to="/components/new">
          <Plus className="mr-1.5 size-4" aria-hidden />
          {t("component.create")}
        </Link>
      </Button>
    </div>
  )
}
