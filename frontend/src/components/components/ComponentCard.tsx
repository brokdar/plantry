import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Clock, MoreVertical, PencilLine } from "lucide-react"

import { CardImageBadge } from "@/components/editorial/CardImageBadge"
import { EditorialCard } from "@/components/editorial/EditorialCard"
import {
  FoodPlaceholder,
  type FoodPlaceholderCategory,
} from "@/components/editorial/FoodPlaceholder"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { imageURL } from "@/lib/image-url"
import { cn } from "@/lib/utils"
import type { Component } from "@/lib/api/components"

export type ComponentCardLayout = "grid" | "list"

type ComponentCardProps = {
  component: Component
  layout?: ComponentCardLayout
  insightFlags?: { forgotten?: boolean; mostCooked?: boolean }
  onDelete: (id: number) => void
}

export function ComponentCard({
  component,
  layout = "grid",
  insightFlags,
  onDelete,
}: ComponentCardProps) {
  if (layout === "list") {
    return (
      <ComponentCardList
        component={component}
        insightFlags={insightFlags}
        onDelete={onDelete}
      />
    )
  }
  return (
    <ComponentCardGrid
      component={component}
      insightFlags={insightFlags}
      onDelete={onDelete}
    />
  )
}

function ComponentCardGrid({
  component,
  insightFlags,
  onDelete,
}: Omit<ComponentCardProps, "layout">) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id, name, role, image_path, updated_at, tags } = component
  const totalTime =
    (component.prep_minutes ?? 0) + (component.cook_minutes ?? 0)

  return (
    <div className="group relative" data-testid={`component-card-${id}`}>
      <EditorialCard interactive>
        <Link
          to="/components/$id/edit"
          params={{ id: String(id) }}
          className="absolute inset-0 z-0"
          aria-label={name}
        />
        <div className="pointer-events-none relative">
          {image_path ? (
            <EditorialCard.Image
              src={imageURL(image_path, updated_at)}
              alt={name}
            />
          ) : (
            <FoodPlaceholder
              category={role as FoodPlaceholderCategory | undefined}
              className="m-2 aspect-[4/3] w-[calc(100%-1rem)]"
              aria-label={name}
            />
          )}
          {tags.length > 0 && (
            <EditorialCard.ImageOverlay
              position="bottom-left"
              className="flex-wrap"
            >
              {tags.slice(0, 2).map((tag) => (
                <CardImageBadge key={tag}>{tag}</CardImageBadge>
              ))}
              {tags.length > 2 && (
                <CardImageBadge>+{tags.length - 2}</CardImageBadge>
              )}
            </EditorialCard.ImageOverlay>
          )}
          <EditorialCard.ImageOverlay
            position="top-right"
            className="opacity-100 transition-opacity duration-200 md:opacity-0 md:group-focus-within:opacity-100 md:group-hover:opacity-100"
          >
            <span
              aria-hidden
              className="hidden size-7 items-center justify-center rounded-full bg-surface-container-lowest/85 backdrop-blur-md md:flex"
            >
              <PencilLine className="size-3.5 text-on-surface" />
            </span>
          </EditorialCard.ImageOverlay>
        </div>
        <EditorialCard.Body>
          <EditorialCard.Title className="line-clamp-2">
            {name}
          </EditorialCard.Title>
          <EditorialCard.Meta className="mt-2 flex-wrap gap-x-3 gap-y-1.5">
            <Badge variant="secondary">{t(`component.role_${role}`)}</Badge>
            <span>
              {component.reference_portions} {t("component.reference_portions")}
            </span>
            {totalTime > 0 && (
              <span className="inline-flex items-center gap-1">
                <Clock className="size-3" aria-hidden />
                {totalTime} min
              </span>
            )}
          </EditorialCard.Meta>
          {(insightFlags?.forgotten || insightFlags?.mostCooked) && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {insightFlags.forgotten && (
                <Badge
                  variant="outline"
                  className="text-xs"
                  data-testid={`badge-forgotten-${id}`}
                >
                  {t("archive.forgotten")}
                </Badge>
              )}
              {insightFlags.mostCooked && (
                <Badge
                  className="text-xs"
                  data-testid={`badge-most-cooked-${id}`}
                >
                  {t("archive.most_cooked")}
                </Badge>
              )}
            </div>
          )}
        </EditorialCard.Body>
      </EditorialCard>
      <div className="absolute top-3 right-3 z-30 opacity-100 transition-opacity duration-200 focus-within:opacity-100 md:opacity-0 md:group-focus-within:opacity-100 md:group-hover:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("common.actions")}
              className="bg-surface-container-lowest/85 backdrop-blur-md"
              data-testid={`component-card-${id}-menu`}
            >
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onClick={() =>
                navigate({
                  to: "/components/$id/edit",
                  params: { id: String(id) },
                })
              }
            >
              {t("common.edit")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onDelete(id)}
              className="text-destructive focus:text-destructive"
              data-testid={`component-card-${id}-delete`}
            >
              {t("common.delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}

function ComponentCardList({
  component,
  insightFlags,
  onDelete,
}: Omit<ComponentCardProps, "layout">) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { id, name, role, image_path, updated_at, tags } = component
  const totalTime =
    (component.prep_minutes ?? 0) + (component.cook_minutes ?? 0)

  return (
    <div className="group relative" data-testid={`component-card-${id}`}>
      <Link
        to="/components/$id/edit"
        params={{ id: String(id) }}
        className="editorial-shadow flex items-center gap-4 rounded-2xl bg-surface-container-lowest p-3 transition-all duration-200 hover:-translate-y-0.5 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        aria-label={name}
      >
        <div
          className={cn(
            "relative aspect-[4/3] h-20 shrink-0 overflow-hidden rounded-xl bg-surface-container-high"
          )}
        >
          {image_path ? (
            <img
              src={imageURL(image_path, updated_at)}
              alt=""
              className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
              loading="lazy"
            />
          ) : (
            <FoodPlaceholder
              category={role as FoodPlaceholderCategory | undefined}
              className="h-full w-full"
              aria-label={name}
            />
          )}
        </div>
        <div className="min-w-0 flex-1 space-y-1.5">
          <p className="truncate font-heading text-base leading-tight font-bold text-on-surface">
            {name}
          </p>
          <div className="flex flex-wrap items-center gap-2 text-xs text-on-surface-variant">
            <Badge variant="secondary" className="text-[10px]">
              {t(`component.role_${role}`)}
            </Badge>
            <span>
              {component.reference_portions} {t("component.reference_portions")}
            </span>
            {totalTime > 0 && (
              <span className="inline-flex items-center gap-1">
                <Clock className="size-3" aria-hidden />
                {totalTime} min
              </span>
            )}
            {tags.slice(0, 3).map((tag) => (
              <Badge key={tag} variant="outline" className="text-[10px]">
                {tag}
              </Badge>
            ))}
            {insightFlags?.forgotten && (
              <Badge
                variant="outline"
                className="text-[10px]"
                data-testid={`badge-forgotten-${id}`}
              >
                {t("archive.forgotten")}
              </Badge>
            )}
            {insightFlags?.mostCooked && (
              <Badge
                className="text-[10px]"
                data-testid={`badge-most-cooked-${id}`}
              >
                {t("archive.most_cooked")}
              </Badge>
            )}
          </div>
        </div>
        <PencilLine className="hidden size-4 shrink-0 text-on-surface-variant/40 transition-colors group-hover:text-on-surface-variant md:block" />
      </Link>
      <div className="absolute top-3 right-3 z-20 opacity-100 transition-opacity duration-200 md:opacity-0 md:group-hover:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("common.actions")}
              className="bg-surface-container-lowest/85 backdrop-blur-md"
              data-testid={`component-card-${id}-menu`}
            >
              <MoreVertical className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onClick={() =>
                navigate({
                  to: "/components/$id/edit",
                  params: { id: String(id) },
                })
              }
            >
              {t("common.edit")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onDelete(id)}
              className="text-destructive focus:text-destructive"
              data-testid={`component-card-${id}-delete`}
            >
              {t("common.delete")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}
