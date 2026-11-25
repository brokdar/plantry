import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { mockKitchen } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/kitchen", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/kitchen")>()),
  getKitchen: vi.fn(),
  updateKitchen: vi.fn(),
  addBasic: vi.fn(),
  removeBasic: vi.fn(),
}))
vi.mock("@/lib/api/foods", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/foods")>()),
  listFoods: vi.fn(),
}))

import { listFoods } from "@/lib/api/foods"
import {
  addBasic,
  getKitchen,
  removeBasic,
  updateKitchen,
  type Kitchen,
} from "@/lib/api/kitchen"

import { KitchenSection } from "./KitchenSection"

/** A kitchen "server": GET returns what the writes stored. */
function serve(initial: Partial<Kitchen> = {}) {
  let kitchen: Kitchen = { ...mockKitchen, ...initial }
  vi.mocked(getKitchen).mockImplementation(async () => kitchen)
  vi.mocked(updateKitchen).mockImplementation(async (input) => {
    kitchen = { ...kitchen, ...input }
    return kitchen
  })
  vi.mocked(addBasic).mockImplementation(async (id) => {
    kitchen = { ...kitchen, basics: [...kitchen.basics, { id, name: "Zimt" }] }
  })
  vi.mocked(removeBasic).mockImplementation(async (id) => {
    kitchen = { ...kitchen, basics: kitchen.basics.filter((b) => b.id !== id) }
  })
}

function renderSection() {
  return renderWithRouter(
    <TooltipProvider>
      <KitchenSection />
    </TooltipProvider>
  )
}

describe("KitchenSection", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listFoods).mockResolvedValue({ items: [], total: 0 } as never)
  })

  it("shows the defaults without a key and says what they do", async () => {
    serve()
    renderSection()
    expect(
      await screen.findByText(
        "Plantry prefers recipes with up to 30 minutes of active cooking on weekdays."
      )
    ).toBeInTheDocument()
    expect(screen.getByTestId("kitchen-minutes-weekend")).toHaveTextContent(
      "60 min"
    )
    expect(
      screen.getByText("Not set. Plantry treats you as a home cook.")
    ).toBeInTheDocument()
    expect(screen.getByLabelText("Oven")).toBeChecked()
    expect(screen.getByLabelText("Stovetop")).toBeChecked()
    expect(screen.getByLabelText("Wok")).not.toBeChecked()
    expect(screen.getByText("No basics yet.")).toBeInTheDocument()
  })

  it("saves skill, heat and a ticked tool as a full replace", async () => {
    serve()
    const u = userEvent.setup()
    renderSection()
    await u.click(await screen.findByRole("radio", { name: "Confident" }))
    await waitFor(() =>
      expect(updateKitchen).toHaveBeenLastCalledWith(
        expect.objectContaining({ skill: "confident", minutes_weekday: 30 })
      )
    )
    await u.click(screen.getByRole("radio", { name: "Medium" }))
    await u.click(screen.getByLabelText("Wok"))
    await waitFor(() =>
      expect(updateKitchen).toHaveBeenLastCalledWith(
        expect.objectContaining({
          skill: "confident",
          heat: "medium",
          equipment: ["oven", "stovetop", "wok"],
        })
      )
    )
  })

  it("adds up to five leanings and then stops", async () => {
    serve({ leanings: ["a", "b", "c", "d"] })
    const u = userEvent.setup()
    renderSection()
    const input = await screen.findByLabelText("Flavour leanings")
    await u.type(input, "lemony{Enter}")
    await waitFor(() =>
      expect(updateKitchen).toHaveBeenCalledWith(
        expect.objectContaining({ leanings: ["a", "b", "c", "d", "lemony"] })
      )
    )
    expect(
      await screen.findByText(/That is the limit of 5/)
    ).toBeInTheDocument()
    expect(screen.queryByLabelText("Flavour leanings")).toBeNull()
  })

  it("lists the basics and removes one", async () => {
    serve({ basics: [{ id: 7, name: "Salz" }] })
    const u = userEvent.setup()
    renderSection()
    expect(await screen.findByText("Salz")).toBeInTheDocument()
    await u.click(
      screen.getByRole("button", { name: "Remove Salz from the basics" })
    )
    await waitFor(() => expect(removeBasic).toHaveBeenCalledWith(7))
    expect(await screen.findByText("No basics yet.")).toBeInTheDocument()
  })

  it("adds a found ingredient to the basics", async () => {
    serve()
    vi.mocked(listFoods).mockImplementation(
      async (params) =>
        ({
          items: params?.search ? [{ id: 9, name: "Zimt", kind: "leaf" }] : [],
          total: 1,
        }) as never
    )
    const u = userEvent.setup()
    renderSection()
    await u.type(await screen.findByLabelText("Add a food"), "zim")
    await u.click(
      await screen.findByRole("button", { name: "Add Zimt to the basics" })
    )
    await waitFor(() => expect(addBasic).toHaveBeenCalledWith(9))
    expect(await screen.findByTestId("kitchen-basics")).toHaveTextContent(
      "Zimt"
    )
  })
})
