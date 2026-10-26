import { createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { ShoppingList } from "@/components/planner/ShoppingPanel"
import { usePlannerWindow } from "@/lib/hooks/usePlannerWindow"

export const Route = createFileRoute("/shop/")({
  component: ShopPage,
})

function ShopPage() {
  const { t } = useTranslation()
  const { from, to } = usePlannerWindow()

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 px-4 py-8 md:px-8 md:py-12">
      <h1 className="text-ink">{t("shopping.page_title")}</h1>
      <ShoppingList key={from + "_" + to} range={{ from, to }} />
    </div>
  )
}
