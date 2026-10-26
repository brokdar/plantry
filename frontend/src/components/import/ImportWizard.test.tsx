import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"
import { mockLookupCandidate } from "@/test/fixtures"
import { ApiError } from "@/lib/api/client"
import type { Draft, FinalizedFood } from "@/lib/api/import"
import type {
  ComposedFood,
  FoodNutritionDetail,
  LeafFood,
} from "@/lib/api/foods"
import type { LookupCandidate } from "@/lib/api/lookup"

vi.mock("@/lib/api/import", () => ({
  extractRecipe: vi.fn(),
  lookupImportLine: vi.fn(),
  resolveImport: vi.fn(),
}))

vi.mock("@/lib/api/foods", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/foods")>()),
  createFood: vi.fn(),
  getFoodNutrition: vi.fn(),
}))

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}))

import {
  extractRecipe,
  lookupImportLine,
  resolveImport,
} from "@/lib/api/import"
import { createFood, getFoodNutrition } from "@/lib/api/foods"
import { toast } from "sonner"

import { ImportWizard } from "./ImportWizard"

const draft: Draft = {
  name: "Zwiebelkuchen",
  description: "Ein herbstlicher Klassiker.",
  source_url: "https://www.chefkoch.de/rezepte/1/zwiebelkuchen.html",
  image_url: "https://img.chefkoch-cdn.de/zwiebelkuchen.jpg",
  prep_minutes: 20,
  cook_minutes: 40,
  total_minutes: 60,
  reference_portions: 4,
  instructions: ["Teig kneten.", "Backen."],
  ingredients: [
    {
      raw_text: "200g Mehl",
      amount: 200,
      unit: "g",
      original_unit: "g",
      name: "Mehl",
      note: "",
      confidence: "parsed",
    },
    {
      raw_text: "Salz und Pfeffer",
      amount: 0,
      unit: "",
      original_unit: "",
      name: "Salz und Pfeffer",
      note: "",
      confidence: "unparsed",
    },
  ],
  tags: [],
  language: "de",
  extract_method: "jsonld",
  warnings: [],
  source_kcal_per_serving: 520,
}

const localMehl: LookupCandidate = {
  ...mockLookupCandidate,
  name: "Mehl",
  source: "local",
  fdc_id: null,
  existing_id: 7,
  exact: true,
}

const prefixLocal: LookupCandidate = {
  ...localMehl,
  name: "Mehlschwitze",
  existing_id: 8,
  exact: false,
}

const externalMehl: LookupCandidate = {
  ...mockLookupCandidate,
  name: "Mehl",
  source_name: "Wheat flour, white",
  kcal_100g: 364,
  protein_100g: 10.3,
  fat_100g: 1,
  carbs_100g: 76.3,
  fiber_100g: null,
  sodium_100g: null,
}

const createdLeaf: LeafFood = {
  id: 31,
  kind: "leaf",
  name: "Mehl",
  image_path: null,
  favorite: false,
  cook_count: 0,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

const mehlNutrition: FoodNutritionDetail = {
  kcal: 350,
  protein: 10,
  fat: 1,
  carbs: 72,
  fiber: 3,
  sodium: 0,
  complete: true,
  missing: [],
}

const finalized: FinalizedFood = {
  name: "Zwiebelkuchen",
  kind: "composed",
  role: "main",
  reference_portions: 4,
  prep_minutes: 20,
  cook_minutes: 40,
  notes: null,
  children: [
    { child_id: 7, amount: 200, unit: "g", grams: 200, sort_order: 0 },
  ],
  instructions: [
    { step_number: 1, text: "Teig kneten." },
    { step_number: 2, text: "Backen." },
  ],
  tags: [],
}

function mockLookup(candidates: LookupCandidate[]) {
  vi.mocked(lookupImportLine).mockImplementation(async (query) => ({
    results: query === "Mehl" ? candidates : [],
    recommended_index: query === "Mehl" && candidates.length > 0 ? 0 : -1,
  }))
}

async function openReview() {
  const user = userEvent.setup()
  renderWithRouter(<ImportWizard />)
  await user.type(
    await screen.findByLabelText(/recipe url/i),
    "https://www.chefkoch.de/rezepte/1/zwiebelkuchen.html"
  )
  await user.click(screen.getByRole("button", { name: /extract recipe/i }))
  await screen.findByRole("heading", { name: /review recipe/i })
  return user
}

describe("ImportWizard", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(extractRecipe).mockResolvedValue({ draft })
    vi.mocked(getFoodNutrition).mockResolvedValue(mehlNutrition)
  })

  it("creates a leaf with the nutrition of a candidate that is not in the library", async () => {
    mockLookup([externalMehl])
    vi.mocked(createFood).mockResolvedValue(createdLeaf)
    const user = await openReview()

    const candidate = await screen.findByRole("button", {
      name: /wheat flour, white/i,
    })
    expect(candidate).toBeEnabled()
    await user.click(candidate)

    await waitFor(() => expect(createFood).toHaveBeenCalledTimes(1))
    expect(vi.mocked(createFood).mock.calls[0][0]).toMatchObject({
      kind: "leaf",
      name: "Mehl",
      source: "fdc",
      fdc_id: "171077",
      kcal_100g: 364,
      protein_100g: 10.3,
      fat_100g: 1,
      carbs_100g: 76.3,
      // Missing values stay null so the completeness check can flag them.
      fiber_100g: null,
      sodium_100g: null,
    })
    expect(await screen.findByText(/linked/i)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /save recipe/i })).toBeEnabled()
  })

  it("reuses the existing leaf when the create hits a duplicate name", async () => {
    mockLookup([externalMehl])
    vi.mocked(createFood).mockRejectedValue(
      new ApiError(409, "error.food.duplicate_name", 42)
    )
    vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
    const user = await openReview()

    await user.click(
      await screen.findByRole("button", { name: /wheat flour, white/i })
    )

    const save = screen.getByRole("button", { name: /save recipe/i })
    await waitFor(() => expect(save).toBeEnabled())
    expect(toast.error).not.toHaveBeenCalled()

    vi.mocked(createFood).mockResolvedValue({
      ...finalized,
      id: 99,
    } as unknown as ComposedFood)
    await user.click(save)
    await waitFor(() => expect(resolveImport).toHaveBeenCalled())
    expect(vi.mocked(resolveImport).mock.calls[0][0].children[0]).toMatchObject(
      { resolution: "existing", food_id: 42 }
    )
  })

  it("shows a toast when creating the ingredient fails", async () => {
    mockLookup([])
    vi.mocked(createFood).mockRejectedValue(new ApiError(500, "error.server"))
    const user = await openReview()

    await user.click(
      await screen.findByRole("button", { name: /create as new ingredient/i })
    )

    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
    expect(screen.getByText("1 ingredient needs a choice")).toBeInTheDocument()
  })

  it("disables save while rows are unresolved and says how many", async () => {
    mockLookup([])
    await openReview()

    expect(
      await screen.findByText("1 ingredient needs a choice")
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /save recipe/i })).toBeDisabled()
  })

  it("auto-links an exact catalogue match", async () => {
    mockLookup([localMehl])
    await openReview()

    expect(await screen.findByText(/linked/i)).toBeInTheDocument()
    expect(
      screen.queryByText("1 ingredient needs a choice")
    ).not.toBeInTheDocument()
  })

  it("does not auto-link a catalogue match that only starts with the name", async () => {
    mockLookup([prefixLocal])
    await openReview()

    // The prefix match stays pickable but is not chosen for the user.
    expect(
      await screen.findByRole("button", { name: /mehlschwitze/i })
    ).toBeInTheDocument()
    expect(screen.getByText("1 ingredient needs a choice")).toBeInTheDocument()
    expect(screen.queryByText(/linked/i)).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: /save recipe/i })).toBeDisabled()
  })

  it.each([
    ["resolving the import", "resolve"],
    ["creating the recipe", "create"],
  ] as const)(
    "reports a save failure while %s once, as a toast",
    async (_, failing) => {
      mockLookup([localMehl])
      const err = new ApiError(500, "error.server")
      if (failing === "resolve") {
        vi.mocked(resolveImport).mockRejectedValue(err)
      } else {
        vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
        vi.mocked(createFood).mockRejectedValue(err)
      }
      const user = await openReview()

      const save = screen.getByRole("button", { name: /save recipe/i })
      await waitFor(() => expect(save).toBeEnabled())
      await user.click(save)

      await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
      expect(
        screen.queryByText(/something went wrong/i)
      ).not.toBeInTheDocument()
      expect(save).toBeEnabled()
    }
  )

  it("shows the computed kcal per portion beside the source value", async () => {
    mockLookup([localMehl])
    vi.mocked(getFoodNutrition).mockResolvedValue({
      ...mehlNutrition,
      complete: false,
      missing: [{ id: 7, name: "Mehl" }],
    })
    await openReview()

    // 200 g × 350 kcal/100 g ÷ 4 portions = 175 kcal.
    expect(await screen.findByText("175 kcal per portion")).toBeInTheDocument()
    expect(screen.getByText("Source: 520 kcal")).toBeInTheDocument()
    expect(
      await screen.findByTestId("nutrition-completeness-warning")
    ).toHaveTextContent("Mehl")
  })

  it("opens the completeness warning's links in a new tab", async () => {
    mockLookup([localMehl])
    vi.mocked(getFoodNutrition).mockResolvedValue({
      ...mehlNutrition,
      complete: false,
      missing: [{ id: 7, name: "Mehl" }],
    })
    await openReview()

    // Same-tab navigation would discard the unsaved review.
    const link = await screen.findByRole("link", { name: /mehl/i })
    expect(link).toHaveAttribute("target", "_blank")
    expect(link).toHaveAttribute("rel", "noopener noreferrer")
    expect(link).toHaveAccessibleName(/^Mehl\s*\(opens in a new tab\)$/)
  })

  it("creates a manual ingredient with null macros", async () => {
    mockLookup([])
    vi.mocked(createFood).mockResolvedValue(createdLeaf)
    const user = await openReview()

    await user.click(
      await screen.findByRole("button", { name: /create as new ingredient/i })
    )

    await waitFor(() => expect(createFood).toHaveBeenCalledTimes(1))
    expect(vi.mocked(createFood).mock.calls[0][0]).toMatchObject({
      kind: "leaf",
      name: "Mehl",
      source: "manual",
      kcal_100g: null,
      protein_100g: null,
      fat_100g: null,
      carbs_100g: null,
    })
  })

  it("marks a skipped quantity-less line as optional, not as a parse error", async () => {
    mockLookup([localMehl])
    vi.mocked(extractRecipe).mockResolvedValue({
      draft: {
        ...draft,
        ingredients: [
          ...draft.ingredients,
          {
            raw_text: "2 Handvoll Kräuter",
            amount: 2,
            unit: "",
            original_unit: "Handvoll",
            name: "Kräuter",
            note: "",
            confidence: "unparsed",
          },
        ],
      },
    })
    await openReview()

    expect(await screen.findByText("Optional, skipped")).toBeInTheDocument()
    // Only the line with an amount still asks for a review.
    expect(screen.getAllByText("Could not parse — please review")).toHaveLength(
      1
    )
  })

  it("labels each row's controls with the ingredient name", async () => {
    mockLookup([localMehl])
    await openReview()

    expect(
      await screen.findByRole("spinbutton", { name: "Amount for Mehl" })
    ).toHaveValue(200)
    expect(
      screen.getByRole("combobox", { name: "Unit for Mehl" })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("combobox", { name: "How to use Salz und Pfeffer" })
    ).toBeInTheDocument()
  })

  it("lets the portions field be cleared and checks it on save", async () => {
    mockLookup([localMehl])
    vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
    const user = await openReview()

    const portions = screen.getByLabelText("Reference portions")
    await user.clear(portions)
    expect(portions).toHaveValue(null)
    await user.type(portions, "6")
    expect(portions).toHaveValue(6)

    await user.clear(portions)
    const save = screen.getByRole("button", { name: /save recipe/i })
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)

    expect(
      await screen.findByText("Enter at least 1 portion.")
    ).toBeInTheDocument()
    expect(portions).toHaveAttribute("aria-invalid", "true")
    expect(resolveImport).not.toHaveBeenCalled()
  })

  it("saves the description, source URL and image, not notes", async () => {
    mockLookup([localMehl])
    vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
    vi.mocked(createFood).mockResolvedValue({
      ...finalized,
      id: 99,
      warnings: ["image_download_failed"],
    } as unknown as ComposedFood)
    const user = await openReview()

    const save = screen.getByRole("button", { name: /save recipe/i })
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)

    await waitFor(() => expect(createFood).toHaveBeenCalledTimes(1))
    expect(vi.mocked(resolveImport).mock.calls[0][0].notes).toBeNull()
    const payload = vi.mocked(createFood).mock.calls[0][0]
    expect(payload).toMatchObject({
      kind: "composed",
      description: draft.description,
      source_url: draft.source_url,
      image_url: draft.image_url,
    })
    expect("notes" in payload ? payload.notes : null).not.toBe(
      draft.description
    )
    await waitFor(() => expect(toast.warning).toHaveBeenCalledTimes(1))
  })
})
