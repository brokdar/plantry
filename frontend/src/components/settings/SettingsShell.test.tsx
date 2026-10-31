import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from "@tanstack/react-router"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import "@/lib/i18n"
import {
  mockAvailabilityDefaults,
  mockProfile,
  mockSettingsList,
  mockSystemInfo,
  mockTimeSlots,
} from "@/test/fixtures"

vi.mock("@/lib/api/profile", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/profile")>()),
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))
vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
  listAIModels: vi.fn(),
  getSystemInfo: vi.fn(),
}))
vi.mock("@/lib/api/availability", () => ({
  getAvailabilityDefaults: vi.fn(),
  setAvailabilityDefaults: vi.fn(),
}))
vi.mock("@/lib/api/slots", () => ({ listTimeSlots: vi.fn() }))
vi.mock("@/lib/api/presets", () => ({ getPresets: vi.fn() }))

import { getAvailabilityDefaults } from "@/lib/api/availability"
import { getPresets } from "@/lib/api/presets"
import { getProfile } from "@/lib/api/profile"
import { getSystemInfo, listSettings } from "@/lib/api/settings"
import { listTimeSlots } from "@/lib/api/slots"
import { Route as SectionRoute } from "@/routes/settings/$section"
import { Route as IndexRoute } from "@/routes/settings/index"

/** The real settings routes under a bare root, as the generated tree
 *  wires them. */
function renderAt(path: string) {
  const root = createRootRoute()
  const tree = root.addChildren([
    IndexRoute.update({
      id: "/settings/",
      path: "/settings/",
      getParentRoute: () => root,
    } as never),
    SectionRoute.update({
      id: "/settings/$section",
      path: "/settings/$section",
      getParentRoute: () => root,
    } as never),
  ])
  const router = createRouter({
    routeTree: tree,
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={qc}>
      <ThemeProvider>
        <RouterProvider router={router} />
      </ThemeProvider>
    </QueryClientProvider>
  )
  return router
}

function setViewport(phone: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: query.includes("min-width") ? !phone : false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
  )
}

describe("Settings routes and shell", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setViewport(false)
    vi.mocked(getProfile).mockResolvedValue(mockProfile)
    vi.mocked(listSettings).mockResolvedValue({
      ...mockSettingsList(),
      cipher_available: false,
    })
    vi.mocked(getSystemInfo).mockResolvedValue({
      ...mockSystemInfo,
      cipher_available: false,
    })
    vi.mocked(listTimeSlots).mockResolvedValue({ items: mockTimeSlots })
    vi.mocked(getAvailabilityDefaults).mockResolvedValue({
      items: mockAvailabilityDefaults,
    })
    vi.mocked(getPresets).mockResolvedValue({ items: [], total: 0 } as never)
  })

  afterEach(() => vi.unstubAllGlobals())

  it("/settings redirects to You & household", async () => {
    const router = renderAt("/settings")
    await waitFor(() =>
      expect(router.state.location.pathname).toBe("/settings/household")
    )
    expect(
      await screen.findByRole("heading", { level: 1, name: "Settings" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("heading", { level: 2, name: "You & household" })
    ).toBeInTheDocument()
    expect(await screen.findByRole("radio", { name: "Omnivore" })).toBeChecked()
  })

  it("an unknown section redirects to You & household", async () => {
    const router = renderAt("/settings/nope")
    await waitFor(() =>
      expect(router.state.location.pathname).toBe("/settings/household")
    )
  })

  it("lists every section beside the content and marks the open one", async () => {
    renderAt("/settings/cooking")
    const nav = await screen.findByRole("navigation", {
      name: "Settings sections",
    })
    const links = within(nav).getAllByRole("link")
    expect(links.map((l) => l.textContent)).toEqual([
      expect.stringContaining("You & household"),
      expect.stringContaining("Meals & week"),
      expect.stringContaining("Cooking & variety"),
      expect.stringContaining("Assistant"),
      expect.stringContaining("Data & system"),
    ])
    expect(
      within(nav).getByRole("link", { name: /Cooking & variety/ })
    ).toHaveAttribute("aria-current", "page")
  })

  it("on a phone, /settings shows the list and drills into a section with a back link", async () => {
    setViewport(true)
    const user = userEvent.setup()
    const router = renderAt("/settings")
    const nav = await screen.findByRole("navigation", {
      name: "Settings sections",
    })
    expect(router.state.location.pathname).toBe("/settings")
    await user.click(within(nav).getByRole("link", { name: /Assistant/ }))
    await waitFor(() =>
      expect(router.state.location.pathname).toBe("/settings/assistant")
    )
    await user.click(
      await screen.findByRole("link", { name: "Back to all settings" })
    )
    await waitFor(() =>
      expect(router.state.location.pathname).toBe("/settings")
    )
  })

  it("shows no secret-key banner in the assistant or system sections", async () => {
    renderAt("/settings/assistant")
    await screen.findByTestId("ai-provider-select")
    expect(screen.queryByText(/encryption key/i)).toBeNull()
  })
})
