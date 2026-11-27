import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"

import { NeedsCheckEntry } from "./NeedsCheck"

vi.mock("@/lib/api/foods", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/foods")>()),
  getNeedsCheck: vi.fn(),
  updateFood: vi.fn(),
}))
vi.mock("@/lib/api/import", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/import")>()),
  lookupImportLine: vi.fn(),
}))

import { getNeedsCheck, updateFood, type NeedsCheck } from "@/lib/api/foods"
import { lookupImportLine } from "@/lib/api/import"

const legacy: NeedsCheck = {
  count: 2,
  leaves: [
    {
      id: 7,
      name: "Mystery",
      recipes: [
        { id: 1, name: "Legacy stew" },
        { id: 2, name: "Other" },
      ],
    },
  ],
  recipes: [
    {
      id: 1,
      name: "Legacy stew",
      lines: [{ leaf_id: 7, leaf_name: "Mystery", amount: 100, unit: "g" }],
    },
    {
      id: 2,
      name: "Other",
      lines: [{ leaf_id: 7, leaf_name: "Mystery", amount: 50, unit: "g" }],
    },
  ],
}

describe("NeedsCheckEntry", () => {
  beforeEach(() => vi.clearAllMocks())

  it("is absent when nothing needs a check", async () => {
    vi.mocked(getNeedsCheck).mockResolvedValue({
      count: 0,
      leaves: [],
      recipes: [],
    })
    renderWithRouter(<NeedsCheckEntry />)
    await vi.waitFor(() => expect(getNeedsCheck).toHaveBeenCalled())
    expect(screen.queryByTestId("needs-check-entry")).toBeNull()
  })

  it("lists recipes with their problem lines behind a quiet count line", async () => {
    vi.mocked(getNeedsCheck).mockResolvedValue(legacy)
    renderWithRouter(<NeedsCheckEntry />)

    await userEvent.click(await screen.findByText("2 recipes need your check"))

    expect(screen.getByRole("link", { name: "Legacy stew" })).toHaveAttribute(
      "href",
      "/recipes/1"
    )
    expect(screen.getAllByText(/no nutrition data/)).toHaveLength(2)
    expect(
      screen.getAllByText("Fixing this also fixes all 2 recipes that use it.")
    ).toHaveLength(2)
  })

  it("writes entered nutrition onto the existing leaf and refreshes the list", async () => {
    vi.mocked(getNeedsCheck)
      .mockResolvedValueOnce(legacy)
      .mockResolvedValue({ count: 0, leaves: [], recipes: [] })
    vi.mocked(updateFood).mockResolvedValue({} as never)
    renderWithRouter(<NeedsCheckEntry />)

    await userEvent.click(await screen.findByTestId("needs-check-entry"))
    await userEvent.click(
      screen.getAllByRole("button", { name: "Enter nutrition" })[0]
    )
    for (const [label, v] of [
      ["kcal", "50"],
      ["Protein (g)", "1"],
      ["Fat (g)", "2"],
      ["Carbs (g)", "3"],
    ]) {
      await userEvent.type(screen.getByLabelText(label), v)
    }
    await userEvent.click(
      screen.getByRole("button", { name: "Save nutrition" })
    )

    expect(updateFood).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        kind: "leaf",
        name: "Mystery",
        kcal_100g: 50,
        protein_100g: 1,
        fat_100g: 2,
        carbs_100g: 3,
      })
    )
    await vi.waitFor(() =>
      expect(screen.queryByTestId("needs-check-entry")).toBeNull()
    )
  })

  it("applies a picked catalogue result to the leaf, keeping its name", async () => {
    vi.mocked(getNeedsCheck).mockResolvedValue(legacy)
    vi.mocked(updateFood).mockResolvedValue({} as never)
    vi.mocked(lookupImportLine).mockResolvedValue({
      results: [
        {
          source: "fdc",
          name: "Mystery raw",
          source_name: "Mystery raw",
          fdc_id: 99,
          kcal_100g: 80,
          protein_100g: 4,
          fat_100g: 1,
          carbs_100g: 10,
        },
      ],
    } as never)
    renderWithRouter(<NeedsCheckEntry />)

    await userEvent.click(await screen.findByTestId("needs-check-entry"))
    await userEvent.click(
      screen.getAllByRole("button", { name: "Search again" })[0]
    )
    await userEvent.click(await screen.findByText("Mystery raw"))

    expect(updateFood).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        name: "Mystery",
        fdc_id: "99",
        kcal_100g: 80,
      })
    )
  })
})
