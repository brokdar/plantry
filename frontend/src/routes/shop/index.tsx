import { createFileRoute } from "@tanstack/react-router"

import { ShopPage } from "@/components/shop/ShopPage"

const DATE = /^\d{4}-\d{2}-\d{2}$/

function dateParam(value: unknown): string | undefined {
  return typeof value === "string" && DATE.test(value) ? value : undefined
}

export const Route = createFileRoute("/shop/")({
  // `from` and `to` come together (the plan board links its window);
  // without them the list shows the server's default range.
  validateSearch: (search): { from?: string; to?: string } => ({
    from: dateParam(search.from),
    to: dateParam(search.to),
  }),
  component: ShopRoute,
})

function ShopRoute() {
  const { from, to } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ShopPage
      range={from && to ? { from, to } : undefined}
      onRangeChange={(range) => void navigate({ search: range })}
    />
  )
}
