import { createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { ShoppingList } from "@/components/planner/ShoppingPanel"
import { usePlannerWindow } from "@/lib/hooks/usePlannerWindow"

const DATE = /^\d{4}-\d{2}-\d{2}$/

function dateParam(value: unknown): string | undefined {
  return typeof value === "string" && DATE.test(value) ? value : undefined
}

export const Route = createFileRoute("/shop/")({
  // The plan board links here with its window (`/shop?from&to`).
  validateSearch: (search): { from?: string; to?: string } => ({
    from: dateParam(search.from),
    to: dateParam(search.to),
  }),
  component: ShopPage,
})

function ShopPage() {
  const { t } = useTranslation()
  const search = Route.useSearch()
  const planner = usePlannerWindow()
  const { from, to } =
    search.from && search.to ? { from: search.from, to: search.to } : planner

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 px-4 py-8 md:px-8 md:py-12">
      <h1 className="text-ink">{t("shopping.page_title")}</h1>
      <ShoppingList key={from + "_" + to} range={{ from, to }} />
    </div>
  )
}
