import { act, render, renderHook } from "@testing-library/react"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import type { ReactNode } from "react"
import { vi } from "vitest"
import "@/lib/i18n"

const defaultOptions = {
  queries: { retry: false },
  mutations: { retry: false },
}

/** Wrapper for renderHook tests — no RouterProvider, accepts an external QC. */
export function createHookWrapper(qc?: QueryClient) {
  const client = qc ?? new QueryClient({ defaultOptions })
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
}

/** Runs a mutation hook to success and returns the query keys it
 *  invalidated, in call order. */
export async function invalidatedBy<V>(
  hook: () => { mutateAsync: (v: V) => Promise<unknown> },
  variables: V
): Promise<unknown[]> {
  const qc = new QueryClient({ defaultOptions })
  const spy = vi.spyOn(qc, "invalidateQueries")
  const { result } = renderHook(hook, { wrapper: createHookWrapper(qc) })
  await act(async () => {
    await result.current.mutateAsync(variables)
  })
  return spy.mock.calls.map((c) => c[0]?.queryKey)
}

export function renderWithRouter(ui: React.ReactElement, path = "/") {
  const rootRoute = createRootRoute({ component: () => ui })
  const routeTree = rootRoute.addChildren([])
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}
