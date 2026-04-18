import { useState, useDeferredValue } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import {
  Bookmark,
  Download,
  MoreHorizontal,
  MoreVertical,
  Plus,
} from "lucide-react"

import { EditorialCard } from "@/components/editorial/EditorialCard"
import { EmptyCreateCard } from "@/components/editorial/EmptyCreateCard"
import {
  FilterChipGroup,
  type FilterChipOption,
} from "@/components/editorial/FilterChipGroup"
import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { PageHeader } from "@/components/editorial/PageHeader"
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
import {
  useComponents,
  useDeleteComponent,
  useInsights,
} from "@/lib/queries/components"
import { imageURL } from "@/lib/image-url"
import { COMPONENT_ROLES } from "@/lib/schemas/component"

const PAGE_SIZE = 20

export function ComponentList() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [roleFilter, setRoleFilter] = useState<string | null>(null)
  const [offset, setOffset] = useState(0)
  const [deleteId, setDeleteId] = useState<number | null>(null)

  const { data, isLoading } = useComponents({
    search: deferredSearch || undefined,
    role: roleFilter ?? undefined,
    limit: PAGE_SIZE,
    offset,
  })

  const deleteMutation = useDeleteComponent()
  const { data: insights } = useInsights()
  const forgottenIds = new Set(insights?.forgotten.map((c) => c.id) ?? [])
  const mostCookedIds = new Set(insights?.most_cooked.map((c) => c.id) ?? [])

  function handleDelete() {
    if (deleteId === null) return
    deleteMutation.mutate(deleteId, {
      onSuccess: () => setDeleteId(null),
    })
  }

  const total = data?.total ?? 0
  const items = data?.items ?? []
  const from = total > 0 ? offset + 1 : 0
  const to = Math.min(offset + PAGE_SIZE, total)

  const roleOptions: FilterChipOption[] = COMPONENT_ROLES.map((role) => ({
    value: role,
    label: t(`component.role_${role}`),
    testId: `component-filter-role-${role}`,
  }))

  return (
    <div className="mx-auto max-w-7xl space-y-8 px-4 py-8 md:px-8 md:py-12">
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

      <div className="space-y-4">
        <Input
          placeholder={t("component.search_placeholder")}
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setOffset(0)
          }}
          className="max-w-sm rounded-full bg-surface-container-highest"
          data-testid="catalog-search"
        />
        <FilterChipGroup
          testId="component-filter-role"
          ariaLabel={t("component.role")}
          options={roleOptions}
          value={roleFilter}
          onValueChange={(v) => {
            setRoleFilter(v)
            setOffset(0)
          }}
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
            to="/components/new"
            title={t("component.create")}
            description={t("component.empty_state")}
            testId="component-create-tile"
          />
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-8 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {items.map((item) => (
              <div
                key={item.id}
                className="relative"
                data-testid={`component-card-${item.id}`}
              >
                <EditorialCard interactive>
                  <Link
                    to="/components/$id"
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
                      category={
                        item.role as FoodPlaceholderCategory | undefined
                      }
                      className="m-2 aspect-[4/3] w-[calc(100%-1rem)]"
                      aria-label={item.name}
                    />
                  )}
                  <EditorialCard.Body>
                    <div className="flex items-start gap-2">
                      <EditorialCard.Title className="flex-1">
                        {item.name}
                      </EditorialCard.Title>
                    </div>
                    <EditorialCard.Meta className="mt-2 flex-wrap">
                      <Badge variant="secondary">
                        {t(`component.role_${item.role}`)}
                      </Badge>
                      <span>
                        {item.reference_portions}{" "}
                        {t("component.reference_portions")}
                      </span>
                      {forgottenIds.has(item.id) && (
                        <Badge
                          variant="outline"
                          className="text-xs"
                          data-testid={`badge-forgotten-${item.id}`}
                        >
                          {t("archive.forgotten")}
                        </Badge>
                      )}
                      {mostCookedIds.has(item.id) && (
                        <Badge
                          className="text-xs"
                          data-testid={`badge-most-cooked-${item.id}`}
                        >
                          {t("archive.most_cooked")}
                        </Badge>
                      )}
                    </EditorialCard.Meta>
                    {item.tags.length > 0 && (
                      <div className="mt-3 flex flex-wrap gap-1">
                        {item.tags.map((tag) => (
                          <Badge
                            key={tag}
                            variant="outline"
                            className="text-xs"
                          >
                            {tag}
                          </Badge>
                        ))}
                      </div>
                    )}
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
                        data-testid={`component-card-${item.id}-menu`}
                      >
                        <MoreVertical className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={() =>
                          navigate({
                            to: "/components/$id/edit",
                            params: { id: String(item.id) },
                          })
                        }
                      >
                        {t("common.edit")}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() => setDeleteId(item.id)}
                        className="text-destructive focus:text-destructive"
                        data-testid={`component-card-${item.id}-delete`}
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
                to="/components/new"
                title={t("component.create")}
                testId="component-create-tile"
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
