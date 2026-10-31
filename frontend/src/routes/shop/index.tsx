import { createFileRoute } from "@tanstack/react-router"

import { ShopPage } from "@/components/shop/ShopPage"

const DATE = /^\d{4}-\d{2}-\d{2}$/

function dateParam(value: unknown): string | undefined {
  return typeof value === "string" && DATE.test(value) ? value : undefined
}

export const Route = createFileRoute("/shop/")({
  // `from` and `to` come together (the plan board links its window);
  // without them the list shows the server's default range. `drafts=1`
  // adds draft plates.
  validateSearch: (search): { from?: string; to?: string; drafts?: 1 } => ({
    from: dateParam(search.from),
    to: dateParam(search.to),
    drafts: search.drafts === 1 || search.drafts === "1" ? 1 : undefined,
  }),
  component: ShopRoute,
})

function ShopRoute() {
  const { from, to, drafts } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ShopPage
      range={from && to ? { from, to } : undefined}
      includeDrafts={drafts === 1}
      onRangeChange={(range) =>
        void navigate({ search: (prev) => ({ ...prev, ...range }) })
      }
      onIncludeDraftsChange={(on) =>
        void navigate({
          search: (prev) => ({ ...prev, drafts: on ? 1 : undefined }),
        })
      }
    />
  )
}
