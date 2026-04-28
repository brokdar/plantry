import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import {
  useApplyTemplate,
  useCreateTemplate,
  useDeleteTemplate,
  useTemplates,
} from "@/lib/queries/templates"

import { plateKeys, templateKeys } from "./keys"

vi.mock("@/lib/api/templates", () => ({
  getTemplates: vi.fn(),
  getTemplate: vi.fn(),
  createTemplate: vi.fn(),
  updateTemplate: vi.fn(),
  deleteTemplate: vi.fn(),
  applyTemplate: vi.fn(),
}))

import {
  applyTemplate,
  createTemplate,
  deleteTemplate,
  getTemplates,
} from "@/lib/api/templates"

function makeClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function wrap(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

describe("useTemplates", () => {
  beforeEach(() => vi.clearAllMocks())

  it("fetches templates list", async () => {
    vi.mocked(getTemplates).mockResolvedValue([
      {
        id: 1,
        name: "Curry Night",
        components: [],
        created_at: "2026-01-01T00:00:00Z",
      },
    ])
    const client = makeClient()

    const { result } = renderHook(() => useTemplates(), {
      wrapper: wrap(client),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(1)
    expect(result.current.data?.[0].name).toBe("Curry Night")
  })
})

describe("useCreateTemplate", () => {
  beforeEach(() => vi.clearAllMocks())

  it("invalidates template lists after create", async () => {
    vi.mocked(createTemplate).mockResolvedValue({
      id: 5,
      name: "X",
      components: [],
      created_at: "2026-01-01T00:00:00Z",
    })
    const client = makeClient()
    const spy = vi.spyOn(client, "invalidateQueries")

    const { result } = renderHook(() => useCreateTemplate(), {
      wrapper: wrap(client),
    })
    result.current.mutate({ name: "X" })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(createTemplate).toHaveBeenCalledWith({ name: "X" })
    expect(spy).toHaveBeenCalledWith({ queryKey: templateKeys.lists() })
  })
})

describe("useDeleteTemplate", () => {
  beforeEach(() => vi.clearAllMocks())

  it("invalidates template lists after delete", async () => {
    vi.mocked(deleteTemplate).mockResolvedValue(undefined)
    const client = makeClient()
    const spy = vi.spyOn(client, "invalidateQueries")

    const { result } = renderHook(() => useDeleteTemplate(), {
      wrapper: wrap(client),
    })
    result.current.mutate(5)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(deleteTemplate).toHaveBeenCalledWith(5)
    expect(spy).toHaveBeenCalledWith({ queryKey: templateKeys.lists() })
  })
})

describe("useApplyTemplate", () => {
  beforeEach(() => vi.clearAllMocks())

  it("invalidates week, shopping-list, and nutrition caches after apply", async () => {
    vi.mocked(applyTemplate).mockResolvedValue(undefined)
    const client = makeClient()
    const spy = vi.spyOn(client, "invalidateQueries")

    const { result } = renderHook(() => useApplyTemplate(), {
      wrapper: wrap(client),
    })
    result.current.mutate({
      templateId: 1,
      input: { start_date: "2026-05-01", slot_id: 2 },
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(applyTemplate).toHaveBeenCalledWith(1, {
      start_date: "2026-05-01",
      slot_id: 2,
    })

    const calls = spy.mock.calls.map((c) => c[0]?.queryKey)
    expect(calls).toEqual(expect.arrayContaining([plateKeys.all]))
  })
})
