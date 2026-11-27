import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import i18n from "@/lib/i18n"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/settings", () => ({ listSettings: vi.fn() }))
vi.mock("@/lib/api/finds", () => ({ requestFind: vi.fn(), getFind: vi.fn() }))
vi.mock("@/lib/api/discovery", () => ({ discover: vi.fn() }))
vi.mock("@/lib/api/kitchen", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/kitchen")>()),
  getKitchen: vi.fn(),
}))

import { discover } from "@/lib/api/discovery"
import { getFind, requestFind } from "@/lib/api/finds"
import { listSettings } from "@/lib/api/settings"

import { DiscoverSheet } from "./DiscoverSheet"

const settings = (chef: string) =>
  vi.mocked(listSettings).mockResolvedValue({
    items: [
      {
        key: "chef.enabled",
        value: chef,
        source: "db",
        is_secret: false,
        env_also_set: false,
      },
    ],
    cipher_available: true,
  })

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
})

describe("DiscoverSheet with the Chef", () => {
  it("on: asks for a dish and starts a find, not a discovery run", async () => {
    settings("true")
    vi.mocked(requestFind).mockResolvedValue({ id: 5 } as never)
    vi.mocked(getFind).mockResolvedValue({
      id: 5,
      state: "searching",
      steps: [{ key: "searched", params: { sites: "chefkoch.de" } }],
    } as never)
    renderWithRouter(<DiscoverSheet open onClose={vi.fn()} />)
    const user = userEvent.setup()
    await user.type(await screen.findByTestId("discover-dish"), " Dal ")
    await user.click(screen.getByTestId("discover-find"))
    expect(requestFind).toHaveBeenCalledWith("Dal")
    expect(await screen.findByText("Searched chefkoch.de")).toBeVisible()
    expect(discover).not.toHaveBeenCalled()
  })

  it("off: the modes stay as they were", async () => {
    settings("false")
    renderWithRouter(<DiscoverSheet open onClose={vi.fn()} />)
    await waitFor(() => expect(listSettings).toHaveBeenCalled())
    expect(await screen.findByTestId("discover-start")).toBeVisible()
    expect(screen.queryByTestId("discover-dish")).toBeNull()
  })
})
