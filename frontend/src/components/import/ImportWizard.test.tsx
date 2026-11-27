import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { renderWithRouter } from "@/test/render"
import { mockLookupCandidate } from "@/test/fixtures"
import { ApiError } from "@/lib/api/client"
import type { Draft, DraftIngredient, FinalizedFood } from "@/lib/api/import"
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
  listPortions: vi.fn(),
}))

vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}))

import {
  extractRecipe,
  lookupImportLine,
  resolveImport,
} from "@/lib/api/import"
import { createFood, getFoodNutrition, listPortions } from "@/lib/api/foods"
import { toast } from "sonner"

import { draftRows } from "./draft-rows"
import { ImportWizard } from "./ImportWizard"

const localMehl: LookupCandidate = {
  ...mockLookupCandidate,
  name: "Mehl",
  source: "local",
  fdc_id: null,
  existing_id: 7,
  exact: true,
}

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
      portion_grams: null,
      diet_class: "vegan",
      needs_weight: false,
      warning: "",
      match: {
        status: "matched",
        tried: ["catalogue"],
        candidates: [localMehl],
        pick: 0,
        grams: 200,
        name: "Mehl",
      },
    },
  ],
  tags: [],
  language: "de",
  extract_method: "jsonld",
  warnings: [],
  source_kcal_per_serving: 520,
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
  archived: false,
  diet_class: "vegan",
  diet_class_source: "auto",
  no_cook: false,
  no_cook_source: "derived",
  shop_section: "produce",
  shop_section_source: "auto",
  used_in_count: 0,
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
    {
      child_id: 7,
      amount: 200,
      unit: "g",
      grams: 200,
      grams_source: "direct",
      sort_order: 0,
    },
  ],
  instructions: [
    { step_number: 1, text: "Teig kneten." },
    { step_number: 2, text: "Backen." },
  ],
  tags: [],
}

function mockLookup(candidates: LookupCandidate[]) {
  vi.mocked(lookupImportLine).mockImplementation(async (query) => {
    const results =
      query === "Mehl"
        ? candidates
        : query === "Salz"
          ? [{ ...localMehl, name: "Salz", existing_id: 9 }]
          : query === "Olivenöl"
            ? [{ ...localMehl, name: "Olivenöl", existing_id: 10 }]
            : []
    return { results, recommended_index: results.length > 0 ? 0 : -1 }
  })
}

const saltLine: DraftIngredient = {
  ...draft.ingredients[0]!,
  raw_text: "Salz",
  amount: 0,
  unit: "",
  original_unit: "",
  name: "Salz",
  to_taste: true,
  match: {
    status: "matched",
    tried: ["catalogue"],
    candidates: [{ ...localMehl, name: "Salz", existing_id: 9 }],
    pick: 0,
    grams: null,
    name: "Salz",
  },
}

const oilLine: DraftIngredient = {
  ...draft.ingredients[0]!,
  raw_text: "Olivenöl",
  amount: 1,
  unit: "tbsp",
  original_unit: "",
  name: "Olivenöl",
  proposed: true,
  match: {
    status: "matched",
    tried: ["catalogue"],
    candidates: [{ ...localMehl, name: "Olivenöl", existing_id: 10 }],
    pick: 0,
    grams: 15,
    name: "Olivenöl",
  },
}

/** The draft's "Mehl" line with no accepted match: the review leaves it
 *  for the user to pick. */
function useUnmatchedMehl() {
  vi.mocked(extractRecipe).mockResolvedValue({
    draft: {
      ...draft,
      ingredients: [{ ...draft.ingredients[0]!, match: null }],
    },
  })
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
    vi.mocked(listPortions).mockResolvedValue({ items: [] })
  })

  it("creates a leaf with the nutrition of a candidate that is not in the library", async () => {
    useUnmatchedMehl()
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
    useUnmatchedMehl()
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
      { resolution: "new", food_id: 42 }
    )
  })

  it("shows a toast when creating the ingredient fails", async () => {
    useUnmatchedMehl()
    mockLookup([])
    vi.mocked(createFood).mockRejectedValue(new ApiError(500, "error.server"))
    const user = await openReview()

    await user.click(
      await screen.findByRole("button", { name: /create with nutrition/i })
    )
    for (const [label, v] of [
      ["kcal", "364"],
      ["Protein (g)", "10"],
      ["Fat (g)", "1"],
      ["Carbs (g)", "76"],
    ])
      await user.type(screen.getByLabelText(label), v)
    await user.click(screen.getByRole("button", { name: "Create ingredient" }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1))
    expect(screen.getByText("1 ingredient needs a choice")).toBeInTheDocument()
  })

  it("disables save while rows are unresolved and says how many", async () => {
    useUnmatchedMehl()
    mockLookup([])
    await openReview()

    expect(
      await screen.findByText("1 ingredient needs a choice")
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /save recipe/i })).toBeDisabled()
  })

  it("shows each line's derived diet class, read-only (D30)", async () => {
    mockLookup([])
    await openReview()

    const mehl = (await screen.findByText("200g Mehl")).parentElement!
    expect(mehl).toHaveTextContent("Vegan")
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
    useUnmatchedMehl()
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

  it("creates a new ingredient only with nutrition per 100 g (P3)", async () => {
    useUnmatchedMehl()
    mockLookup([])
    vi.mocked(createFood).mockResolvedValue(createdLeaf)
    const user = await openReview()

    await user.click(
      await screen.findByRole("button", { name: /create with nutrition/i })
    )
    const create = screen.getByRole("button", { name: "Create ingredient" })
    await user.type(screen.getByLabelText("kcal"), "364")
    expect(create).toBeDisabled()
    await user.type(screen.getByLabelText("Protein (g)"), "10")
    await user.type(screen.getByLabelText("Fat (g)"), "1")
    await user.type(screen.getByLabelText("Carbs (g)"), "76")
    await user.click(create)

    await waitFor(() => expect(createFood).toHaveBeenCalledTimes(1))
    expect(vi.mocked(createFood).mock.calls[0][0]).toMatchObject({
      kind: "leaf",
      name: "Mehl",
      source: "manual",
      kcal_100g: 364,
      protein_100g: 10,
      fat_100g: 1,
      carbs_100g: 76,
    })
    expect(await screen.findByText(/linked/i)).toBeInTheDocument()
  })

  it("does not link a same-name leaf the backend's match rejected (P3)", async () => {
    // "Mehl" has a catalogue leaf of that name, but the match found it
    // unusable (no nutrition): the row stays a problem, Save stays disabled.
    mockLookup([localMehl])
    vi.mocked(extractRecipe).mockResolvedValue({
      draft: {
        ...draft,
        ingredients: [
          {
            ...draft.ingredients[0]!,
            match: {
              status: "unmatched",
              reason: "no_nutrition",
              tried: ["catalogue"],
              candidates: [],
              pick: -1,
              grams: null,
              name: "Mehl",
            },
          },
        ],
      },
    })
    await openReview()
    await waitFor(() => expect(lookupImportLine).toHaveBeenCalled())
    expect(screen.getByRole("button", { name: /save recipe/i })).toBeDisabled()
    expect(screen.queryByText(/linked/i)).toBeNull()
  })

  it("links the candidate the backend's match picked", async () => {
    mockLookup([prefixLocal])
    vi.mocked(extractRecipe).mockResolvedValue({
      draft: {
        ...draft,
        ingredients: [
          {
            ...draft.ingredients[0]!,
            match: {
              status: "matched",
              tried: ["catalogue"],
              candidates: [prefixLocal],
              pick: 0,
              grams: 200,
              name: "Mehl",
            },
          },
        ],
      },
    })
    await openReview()
    expect(await screen.findByText(/linked/i)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /save recipe/i })).toBeEnabled()
  })

  it("offers no Skip: an unmatched line blocks Save until it is removed (AC-10)", async () => {
    mockLookup([])
    vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
    vi.mocked(extractRecipe).mockResolvedValue({
      draft: {
        ...draft,
        ingredients: [
          {
            ...draft.ingredients[0]!,
            match: {
              status: "unmatched",
              reason: "no_candidate",
              tried: ["catalogue", "fdc:flour"],
              candidates: [],
              pick: -1,
              grams: null,
              name: "Mehl",
            },
          },
          saltLine,
          oilLine,
        ],
      },
    })
    const user = await openReview()

    const save = screen.getByRole("button", { name: /save recipe/i })
    expect(save).toBeDisabled()
    expect(screen.queryByText(/skip/i)).toBeNull()
    expect(screen.getByText(/Tried: In library · USDA “flour”/)).toBeVisible()
    await user.click(
      screen.getByRole("button", { name: "Remove Mehl from the recipe" })
    )
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)

    await waitFor(() => expect(resolveImport).toHaveBeenCalled())
    expect(vi.mocked(resolveImport).mock.calls[0][0].children).toEqual([
      expect.objectContaining({ food_id: 9, to_taste: true }),
      expect.objectContaining({
        food_id: 10,
        amount: 1,
        unit: "tbsp",
        to_taste: false,
      }),
    ])
  })

  it("shows a to-taste seasoning without an amount and a proposed amount as proposed", async () => {
    mockLookup([])
    vi.mocked(extractRecipe).mockResolvedValue({
      draft: { ...draft, ingredients: [saltLine, oilLine] },
    })
    const user = await openReview()

    expect(await screen.findByText("to taste")).toBeVisible()
    expect(screen.queryByLabelText("Amount for Salz")).toBeNull()
    expect(screen.getByText("proposed")).toBeVisible()
    await user.type(screen.getByLabelText("Amount for Olivenöl"), "2")
    expect(screen.queryByText("proposed")).toBeNull()
  })

  it("lists problem lines first and edits, adds, reorders and deletes steps", async () => {
    mockLookup([])
    vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
    vi.mocked(extractRecipe).mockResolvedValue({
      // Mehl without an amount is a problem line: it moves up.
      draft: {
        ...draft,
        ingredients: [saltLine, { ...draft.ingredients[0]!, amount: 0 }],
      },
    })
    const user = await openReview()

    const lines = screen.getAllByText(/^(200g Mehl|Salz)$/, {
      selector: "div.text-xs",
    })
    expect(lines.map((l) => l.textContent)).toEqual(["200g Mehl", "Salz"])

    await user.click(screen.getByRole("button", { name: "Move step 2 up" }))
    await user.click(screen.getByRole("button", { name: "Add a step" }))
    await user.type(screen.getByLabelText("Step 3"), "Servieren.")
    await user.click(screen.getByRole("button", { name: "Delete step 1" }))
    await user.clear(screen.getByLabelText("Prep (min)"))
    await user.type(screen.getByLabelText("Prep (min)"), "15")
    await user.click(
      screen.getByRole("button", { name: "Remove Mehl from the recipe" })
    )
    await user.click(screen.getByRole("button", { name: /save recipe/i }))

    await waitFor(() => expect(resolveImport).toHaveBeenCalled())
    expect(vi.mocked(resolveImport).mock.calls[0][0]).toMatchObject({
      prep_minutes: 15,
      instructions: [
        { step_number: 1, text: "Teig kneten." },
        { step_number: 2, text: "Servieren." },
      ],
    })
  })

  it("names the host and the next step when the page cannot be loaded", async () => {
    vi.mocked(extractRecipe).mockRejectedValue(
      new ApiError(502, "error.import.blocked", undefined, {
        host: "chefkoch.de",
      })
    )
    const user = userEvent.setup()
    renderWithRouter(<ImportWizard />)
    await user.type(
      await screen.findByLabelText(/recipe url/i),
      "https://chefkoch.de/x"
    )
    await user.click(screen.getByRole("button", { name: /extract recipe/i }))
    expect(
      await screen.findByText(
        "Couldn't load chefkoch.de (blocked by the site). Paste the recipe text instead."
      )
    ).toBeInTheDocument()
  })

  it("blocks save on a count line nobody could estimate until a weight is entered", async () => {
    mockLookup([localMehl])
    vi.mocked(extractRecipe).mockResolvedValue({
      draft: {
        ...draft,
        ingredients: [
          {
            raw_text: "1 Stk Mehl",
            amount: 1,
            unit: "piece",
            original_unit: "Stück",
            name: "Mehl",
            note: "",
            confidence: "approximate",
            portion_grams: null,
            needs_weight: true,
            warning: "",
            diet_class: "vegan",
            match: {
              status: "matched",
              tried: ["catalogue"],
              candidates: [localMehl],
              pick: 0,
              grams: null,
              name: "Mehl",
            },
          },
        ],
      },
    })
    await openReview()

    await screen.findByText(/enter a weight per unit/i)
    expect(screen.getByRole("button", { name: /save recipe/i })).toBeDisabled()
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

  describe("prefilled (discovery, D18)", () => {
    const prefilled = { ...draft, image_url: "", tags: ["herbst"] }

    function renderPrefilled(
      overrides: Partial<React.ComponentProps<typeof ImportWizard>> = {}
    ) {
      const onSaved = vi.fn()
      const rows = draftRows(prefilled).map((row, i) =>
        i === 0 ? { ...row, foodId: 7, foodName: "Mehl" } : row
      )
      renderWithRouter(
        <ImportWizard
          initialDraft={prefilled}
          initialRows={rows}
          extraTags={["explore"]}
          onSaved={onSaved}
          {...overrides}
        />
      )
      return { onSaved, user: userEvent.setup() }
    }

    beforeEach(() => {
      mockLookup([])
      vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
      vi.mocked(createFood).mockResolvedValue({
        ...finalized,
        id: 77,
      } as unknown as ComposedFood)
    })

    it("opens at the review with the rows preselected and Save enabled", async () => {
      renderPrefilled()
      expect(
        await screen.findByRole("heading", { name: /review recipe/i })
      ).toBeInTheDocument()
      expect(screen.getByLabelText("Recipe name")).toHaveValue("Zwiebelkuchen")
      expect(screen.getByText("Mehl", { selector: "span" })).toBeInTheDocument()
      expect(screen.getByRole("button", { name: /save recipe/i })).toBeEnabled()
      // Nothing to go back to: there was no link step.
      expect(screen.queryByRole("button", { name: "Back" })).toBeNull()
      expect(extractRecipe).not.toHaveBeenCalled()
    })

    it("keeps Save disabled while a row is unmatched", async () => {
      renderPrefilled({ initialRows: draftRows(prefilled) })
      expect(
        await screen.findByRole("button", { name: /save recipe/i })
      ).toBeDisabled()
      expect(screen.getByText("1 ingredient needs a choice")).toBeVisible()
    })

    it("sends the explore tag and source URL, no image, then reports the food id", async () => {
      const { onSaved, user } = renderPrefilled()
      await user.click(
        await screen.findByRole("button", { name: /save recipe/i })
      )

      await waitFor(() => expect(onSaved).toHaveBeenCalledWith(77))
      expect(vi.mocked(resolveImport).mock.calls[0][0]).toMatchObject({
        tags: ["herbst", "explore"],
        children: [{ resolution: "existing", food_id: 7 }],
      })
      const payload = vi.mocked(createFood).mock.calls[0][0]
      expect(payload).toMatchObject({ source_url: prefilled.source_url })
      expect("image_url" in payload ? payload.image_url : undefined).toBe(
        undefined
      )
    })
  })

  describe("count units", () => {
    const zwiebel: LookupCandidate = {
      ...localMehl,
      name: "Zwiebel",
      existing_id: 12,
    }
    const roteZwiebel: LookupCandidate = {
      ...localMehl,
      name: "Rote Zwiebel",
      existing_id: 13,
      exact: false,
    }
    const countDraft: Draft = {
      ...draft,
      ingredients: [
        {
          raw_text: "2 Zwiebel(n)",
          amount: 2,
          unit: "piece",
          original_unit: "Stück",
          name: "Zwiebel",
          note: "",
          confidence: "approximate",
          portion_grams: 120,
          diet_class: "vegan",
          needs_weight: false,
          warning: "",
          match: {
            status: "matched",
            tried: ["catalogue"],
            candidates: [zwiebel],
            pick: 0,
            grams: 240,
            name: "Zwiebel",
          },
        },
      ],
    }

    beforeEach(() => {
      vi.mocked(extractRecipe).mockResolvedValue({ draft: countDraft })
      vi.mocked(lookupImportLine).mockImplementation(async (query) => ({
        results: query === "Zwiebel" ? [zwiebel, roteZwiebel] : [],
        recommended_index: query === "Zwiebel" ? 0 : -1,
      }))
      vi.mocked(resolveImport).mockResolvedValue({ food: finalized })
      vi.mocked(createFood).mockResolvedValue({
        ...finalized,
        id: 99,
      } as unknown as ComposedFood)
    })

    it("shows the editable estimate and saves the edited weight", async () => {
      const user = await openReview()

      const grams = await screen.findByRole("spinbutton", {
        name: "Grams per piece of Zwiebel",
      })
      await waitFor(() => expect(grams).toHaveValue(120))
      expect(screen.getByText("1 piece ≈")).toBeInTheDocument()
      expect(screen.getByText("estimate")).toBeInTheDocument()
      expect(
        screen.getByRole("combobox", { name: "Unit for Zwiebel" })
      ).toHaveTextContent("piece")
      // 2 × 120 g × 350 kcal/100 g ÷ 4 portions = 210 kcal.
      expect(
        await screen.findByText("210 kcal per portion")
      ).toBeInTheDocument()

      await user.clear(grams)
      await user.type(grams, "150")
      expect(screen.queryByText("estimate")).not.toBeInTheDocument()
      // 2 × 150 g × 350 kcal/100 g ÷ 4 portions ≈ 263 kcal.
      expect(
        await screen.findByText("263 kcal per portion")
      ).toBeInTheDocument()

      await user.click(screen.getByRole("button", { name: /save recipe/i }))
      await waitFor(() => expect(resolveImport).toHaveBeenCalled())
      expect(
        vi.mocked(resolveImport).mock.calls[0][0].children[0]
      ).toMatchObject({
        food_id: 12,
        amount: 2,
        unit: "piece",
        portion_grams: 150,
      })
    })

    it("uses the leaf's saved weight and leaves it untouched on save", async () => {
      vi.mocked(listPortions).mockResolvedValue({
        items: [{ food_id: 12, unit: "piece", grams: 90 }],
      })
      const user = await openReview()

      const grams = await screen.findByRole("spinbutton", {
        name: "Grams per piece of Zwiebel",
      })
      await waitFor(() => expect(grams).toHaveValue(90))
      expect(screen.getByText("saved for this ingredient")).toBeInTheDocument()

      await user.click(screen.getByRole("button", { name: /save recipe/i }))
      await waitFor(() => expect(resolveImport).toHaveBeenCalled())
      expect(
        vi.mocked(resolveImport).mock.calls[0][0].children[0]
      ).toMatchObject({ unit: "piece", portion_grams: null })
    })

    it("blocks saving while the weight per piece is empty", async () => {
      const user = await openReview()

      const grams = await screen.findByRole("spinbutton", {
        name: "Grams per piece of Zwiebel",
      })
      await waitFor(() => expect(grams).toHaveValue(120))
      await user.clear(grams)

      expect(grams).toHaveAttribute("aria-invalid", "true")
      expect(
        screen.getByRole("button", { name: /save recipe/i })
      ).toBeDisabled()
      expect(
        screen.getByText("Enter a weight per unit for 1 ingredient")
      ).toBeInTheDocument()
      expect(screen.queryByText(/needs a choice/)).not.toBeInTheDocument()
    })

    it("drops an edited weight when the row is linked to another leaf", async () => {
      vi.mocked(listPortions).mockImplementation(async (id) => ({
        items: id === 13 ? [{ food_id: 13, unit: "piece", grams: 80 }] : [],
      }))
      const user = await openReview()

      const grams = await screen.findByRole("spinbutton", {
        name: "Grams per piece of Zwiebel",
      })
      await waitFor(() => expect(grams).toHaveValue(120))
      await user.clear(grams)
      await user.type(grams, "150")

      // Unlinking falls back to the estimate.
      await user.click(screen.getByRole("button", { name: "Change" }))
      await user.click(
        await screen.findByRole("button", { name: /rote zwiebel/i })
      )

      // The new leaf's saved weight replaces the 150 g typed for the old one.
      const relinked = await screen.findByRole("spinbutton", {
        name: "Grams per piece of Zwiebel",
      })
      await waitFor(() => expect(relinked).toHaveValue(80))
      expect(screen.getByText("saved for this ingredient")).toBeInTheDocument()

      await user.click(screen.getByRole("button", { name: /save recipe/i }))
      await waitFor(() => expect(resolveImport).toHaveBeenCalled())
      expect(
        vi.mocked(resolveImport).mock.calls[0][0].children[0]
      ).toMatchObject({ food_id: 13, unit: "piece", portion_grams: null })
    })
  })
})
