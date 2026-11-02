import { screen, waitFor } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { ComposedFood } from "@/lib/api/foods"
import type { Draft, FinalizedFood } from "@/lib/api/import"
import i18n from "@/lib/i18n"
import { mockSuggestion } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/discovery", () => ({
  listSuggestions: vi.fn(),
  acceptSuggestion: vi.fn(),
  rejectSuggestion: vi.fn(),
}))
vi.mock("@/lib/api/import", () => ({
  extractRecipe: vi.fn(),
  lookupImportLine: vi.fn(),
  resolveImport: vi.fn(),
}))
vi.mock("@/lib/api/foods", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/foods")>()),
  createFood: vi.fn(),
  getFoodNutrition: vi.fn(),
  listPortions: vi.fn(),
}))
vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}))

import {
  acceptSuggestion,
  listSuggestions,
  rejectSuggestion,
} from "@/lib/api/discovery"
import { createFood, getFoodNutrition, listPortions } from "@/lib/api/foods"
import { lookupImportLine, resolveImport } from "@/lib/api/import"
import { discoveryKeys } from "@/lib/queries/keys"
import { toast } from "sonner"

import { SuggestionReview } from "./SuggestionReview"

const line = (name: string, amount: number) => ({
  raw_text: `${amount} g ${name}`,
  amount,
  unit: "g",
  original_unit: "g",
  name,
  note: "",
  confidence: "parsed" as const,
  portion_grams: null,
  diet_class: "vegan" as const,
})

const base = mockSuggestion()
const draft: Draft = {
  ...base.draft,
  image_url: "http://127.0.0.1:9100/fesenjan.jpg",
  tags: ["persian"],
  ingredients: [line("chicken thighs", 500), line("sumac", 5)],
}

function suggestion(sumacId: number | null) {
  return mockSuggestion({
    draft,
    ingredients: [
      { ...base.ingredients[0], food_id: 12 },
      {
        raw_text: "5 g sumac",
        name: "Sumac",
        amount: 5,
        unit: "g",
        food_id: sumacId,
        candidate: sumacId ? "Sumac" : null,
        known: false,
      },
    ],
  })
}

const finalized: FinalizedFood = {
  name: "Fesenjan",
  kind: "composed",
  role: "main",
  reference_portions: 4,
  prep_minutes: 20,
  cook_minutes: 70,
  notes: null,
  children: [],
  instructions: [],
  tags: ["persian", "explore"],
}

const onClose = vi.fn()

async function renderReview(id: number | undefined = 5) {
  renderWithRouter(<SuggestionReview id={id} onClose={onClose} />)
  return userEvent.setup()
}

beforeEach(async () => {
  vi.clearAllMocks()
  await i18n.changeLanguage("en")
  vi.mocked(listSuggestions).mockResolvedValue({ items: [suggestion(40)] })
  vi.mocked(lookupImportLine).mockResolvedValue({
    results: [],
    recommended_index: -1,
  })
  vi.mocked(getFoodNutrition).mockResolvedValue({
    kcal: 100,
    protein: 10,
    fat: 5,
    carbs: 5,
    fiber: 0,
    sodium: 0,
    complete: true,
    missing: [],
  })
  vi.mocked(listPortions).mockResolvedValue({ items: [] })
  vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
  vi.mocked(createFood).mockResolvedValue({
    ...finalized,
    id: 77,
  } as unknown as ComposedFood)
  vi.mocked(acceptSuggestion).mockResolvedValue(
    mockSuggestion({ status: "accepted", food_id: 77 })
  )
})

describe("SuggestionReview", () => {
  it("opens the review with matched rows and saves, then accepts with the new food id", async () => {
    const user = await renderReview()

    expect(
      await screen.findByRole("heading", { name: /review recipe/i })
    ).toBeVisible()
    expect(listSuggestions).toHaveBeenCalledWith({ status: "pending" })
    expect(screen.getByLabelText("Recipe name")).toHaveValue("Fesenjan")
    const save = screen.getByRole("button", { name: /save recipe/i })
    expect(save).toBeEnabled()
    await user.click(save)

    await waitFor(() => expect(acceptSuggestion).toHaveBeenCalledWith(5, 77))
    expect(vi.mocked(resolveImport).mock.calls[0][0]).toMatchObject({
      tags: ["persian", "explore"],
      children: [{ food_id: 12 }, { food_id: 40 }],
    })
    const payload = vi.mocked(createFood).mock.calls[0][0]
    expect(payload).toMatchObject({ source_url: base.source_url })
    expect("image_url" in payload ? payload.image_url : undefined).toBe(
      undefined
    )
  })

  it("shows the matched leaf's name when it differs from the draft line", async () => {
    await renderReview()
    expect(await screen.findByText("Chicken thigh")).toBeVisible()
  })

  it("stays open while a stale cached list refetches to include the id", async () => {
    const qc = new QueryClient()
    qc.setQueryData(discoveryKeys.suggestionList({ status: "pending" }), {
      items: [],
    })
    renderWithRouter(
      <QueryClientProvider client={qc}>
        <SuggestionReview id={5} onClose={onClose} />
      </QueryClientProvider>
    )
    expect(
      await screen.findByRole("heading", { name: /review recipe/i })
    ).toBeVisible()
    expect(toast.error).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
  })

  it("keeps Save disabled while an ingredient has no match", async () => {
    vi.mocked(listSuggestions).mockResolvedValue({ items: [suggestion(null)] })
    await renderReview()
    expect(
      await screen.findByRole("button", { name: /save recipe/i })
    ).toBeDisabled()
  })

  it("closing the review decides nothing", async () => {
    const user = await renderReview()
    await screen.findByRole("heading", { name: /review recipe/i })
    await user.click(screen.getByTestId("import-sheet-close"))

    expect(onClose).toHaveBeenCalled()
    expect(acceptSuggestion).not.toHaveBeenCalled()
    expect(rejectSuggestion).not.toHaveBeenCalled()
    expect(resolveImport).not.toHaveBeenCalled()
    expect(createFood).not.toHaveBeenCalled()
  })

  it("a failed suggestions query toasts and closes instead of rendering nothing", async () => {
    vi.mocked(listSuggestions).mockRejectedValue(new Error("down"))
    await renderReview()
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(toast.error).toHaveBeenCalledWith(
      "Couldn't open this idea. Try again."
    )
  })

  it("closes with a note when the idea is no longer pending", async () => {
    vi.mocked(listSuggestions).mockResolvedValue({ items: [] })
    await renderReview()
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(toast.error).toHaveBeenCalledWith(
      "This idea is no longer available."
    )
  })
})
