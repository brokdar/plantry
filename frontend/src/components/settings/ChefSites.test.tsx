import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { ChefSite } from "@/lib/api/chef"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/chef", () => ({
  listChefSites: vi.fn(),
  addChefSite: vi.fn(),
  setChefSiteEnabled: vi.fn(),
  removeChefSite: vi.fn(),
  retryChefSite: vi.fn(),
}))

import {
  addChefSite,
  listChefSites,
  removeChefSite,
  retryChefSite,
  setChefSiteEnabled,
} from "@/lib/api/chef"

import { ChefSites } from "./ChefSites"

const site = (over: Partial<ChefSite>): ChefSite => ({
  domain: "bbcgoodfood.com",
  lang: "en",
  enabled: true,
  default: true,
  blocked_until: null,
  ...over,
})

describe("ChefSites", () => {
  let items: ChefSite[]
  beforeEach(() => {
    vi.clearAllMocks()
    items = [
      site({}),
      site({
        domain: "budgetbytes.com",
        blocked_until: "2026-12-20T10:00:00Z",
      }),
      site({ domain: "chefkoch.de", lang: "de" }),
      site({ domain: "mine.example.com", default: false, enabled: false }),
    ]
    vi.mocked(listChefSites).mockImplementation(async () => ({ items }))
  })

  it("groups the sites by language and lets a user-added one be removed", async () => {
    vi.mocked(removeChefSite).mockImplementation(async () => {
      items = items.filter((s) => s.domain !== "mine.example.com")
      return { items }
    })
    const user = userEvent.setup()
    renderWithRouter(<ChefSites />)
    const en = within(await screen.findByTestId("chef-sites-en"))
    expect(en.getAllByRole("switch")).toHaveLength(3)
    expect(
      within(screen.getByTestId("chef-sites-de")).getByText("chefkoch.de")
    ).toBeInTheDocument()
    expect(screen.queryByTestId("site-remove-bbcgoodfood.com")).toBeNull()

    await user.click(screen.getByTestId("site-remove-mine.example.com"))
    await waitFor(() =>
      expect(removeChefSite).toHaveBeenCalledWith("mine.example.com")
    )
    await waitFor(() =>
      expect(screen.queryByText("mine.example.com")).toBeNull()
    )
  })

  it("toggles a site", async () => {
    vi.mocked(setChefSiteEnabled).mockImplementation(
      async (domain, enabled) => {
        items = items.map((s) => (s.domain === domain ? { ...s, enabled } : s))
        return { items }
      }
    )
    const user = userEvent.setup()
    renderWithRouter(<ChefSites />)
    const toggle = await screen.findByLabelText("Search bbcgoodfood.com")
    expect(toggle).toBeChecked()
    await user.click(toggle)
    await waitFor(() =>
      expect(setChefSiteEnabled).toHaveBeenCalledWith("bbcgoodfood.com", false)
    )
    await waitFor(() =>
      expect(screen.getByLabelText("Search bbcgoodfood.com")).not.toBeChecked()
    )
  })

  it("adds a site with its language", async () => {
    vi.mocked(addChefSite).mockImplementation(async ({ domain, lang }) => {
      items = [...items, site({ domain, lang, default: false })]
      return { items }
    })
    const user = userEvent.setup()
    renderWithRouter(<ChefSites />)
    await user.type(await screen.findByTestId("site-domain-input"), "Foo.de")
    await user.selectOptions(screen.getByTestId("site-lang-select"), "de")
    await user.click(screen.getByTestId("site-add-button"))
    await waitFor(() =>
      expect(addChefSite).toHaveBeenCalledWith({ domain: "Foo.de", lang: "de" })
    )
    expect(
      await within(screen.getByTestId("chef-sites-de")).findByText("Foo.de")
    ).toBeInTheDocument()
    expect(screen.getByTestId("site-domain-input")).toHaveValue("")
  })

  it("says when a site is not searchable and tries it again", async () => {
    vi.mocked(retryChefSite).mockImplementation(async (domain) => {
      items = items.map((s) =>
        s.domain === domain ? { ...s, blocked_until: null } : s
      )
      return { items }
    })
    const user = userEvent.setup()
    renderWithRouter(<ChefSites />)
    const row = within(await screen.findByTestId("site-budgetbytes.com"))
    expect(row.getByText(/Not searchable until/)).toHaveTextContent("20")
    expect(
      within(screen.getByTestId("site-bbcgoodfood.com")).queryByText(
        /Not searchable/
      )
    ).toBeNull()
    await user.click(row.getByRole("button", { name: "Try again" }))
    await waitFor(() =>
      expect(retryChefSite).toHaveBeenCalledWith("budgetbytes.com")
    )
    await waitFor(() =>
      expect(screen.queryByText(/Not searchable until/)).toBeNull()
    )
  })
})
