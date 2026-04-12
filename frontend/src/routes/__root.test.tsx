import { describe, expect, test } from "vitest"
import { render, screen } from "@testing-library/react"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"

import "@/lib/i18n"
import { Route as RootRoute } from "./__root"
import { Route as IndexRoute } from "./index"

function renderApp() {
  // Rebuild the route tree in-memory so tests don't depend on the generated
  // routeTree.gen.ts (which is produced by the Vite plugin during build/dev).
  const rootRoute = createRootRoute({
    component: RootRoute.options.component,
  })
  const indexRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: IndexRoute.options.component,
  })
  const routeTree = rootRoute.addChildren([indexRoute])

  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe("app shell", () => {
  test("renders Plantry brand in the nav", async () => {
    renderApp()
    expect(await screen.findByText("Plantry")).toBeInTheDocument()
  })

  test("renders the home welcome heading", async () => {
    renderApp()
    expect(
      await screen.findByRole("heading", { name: /welcome to plantry/i })
    ).toBeInTheDocument()
  })
})
