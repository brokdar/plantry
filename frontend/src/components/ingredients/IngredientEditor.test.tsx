import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { mockChickenBreast, mockLookupResponse } from "@/test/fixtures"

vi.mock("@/lib/api/foods", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/foods")>()
  return {
    ...actual,
    listFoods: vi.fn(),
    getFood: vi.fn(),
    createFood: vi.fn(),
    updateFood: vi.fn(),
    deleteFood: vi.fn(),
    refetchFood: vi.fn(),
    listPortions: vi.fn().mockResolvedValue({ items: [] }),
    upsertPortion: vi.fn(),
    deletePortion: vi.fn(),
  }
})

vi.mock("@/lib/api/lookup", () => ({
  lookupFoods: vi.fn(),
  resolveCandidate: vi.fn(),
}))

vi.mock("@/lib/api/images", () => ({
  uploadFoodImage: vi.fn(),
  deleteFoodImage: vi.fn(),
  fetchImageFromUrl: vi.fn(),
}))

const toastErrorMock = vi.fn()
const toastSuccessMock = vi.fn()
vi.mock("@/lib/toast", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/toast")>()
  return {
    ...actual,
    toastError: (...args: unknown[]) => toastErrorMock(...args),
    toast: {
      success: (...args: unknown[]) => toastSuccessMock(...args),
      error: (...args: unknown[]) => toastErrorMock(...args),
    },
  }
})

type ImageFieldMockProps =
  | {
      mode: "staged"
      stagedBlob: Blob | null
      onStagedChange: (blob: Blob | null) => void
    }
  | { mode: "bound" }

vi.mock("@/components/images/ImageField", () => ({
  ImageField: (props: ImageFieldMockProps) => {
    if (props.mode === "staged") {
      return (
        <div>
          <button
            type="button"
            data-testid="stage-image"
            onClick={() =>
              props.onStagedChange(new Blob(["fake"], { type: "image/jpeg" }))
            }
          >
            Stage
          </button>
          <span data-testid="staged-blob-state">
            {props.stagedBlob ? "has-blob" : "no-blob"}
          </span>
        </div>
      )
    }
    return <div data-testid="bound-image-field" />
  },
}))

import { createFood, refetchFood, updateFood } from "@/lib/api/foods"
import { uploadFoodImage } from "@/lib/api/images"
import { lookupFoods } from "@/lib/api/lookup"
import { ApiError } from "@/lib/api/client"
import { IngredientEditor } from "./IngredientEditor"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("IngredientEditor", () => {
  test("renders create mode with empty fields", async () => {
    renderWithRouter(<IngredientEditor />)

    const nameInput = await screen.findByLabelText("Name")
    expect(nameInput).toHaveValue("")

    const kcalInput = screen.getByLabelText("Calories (kcal)")
    expect(kcalInput).toHaveValue(0)
  })

  test("renders edit mode with pre-filled values", async () => {
    renderWithRouter(<IngredientEditor ingredient={mockChickenBreast} />)

    expect(await screen.findByLabelText("Name")).toHaveValue("Chicken breast")
    expect(
      screen.getByRole("spinbutton", { name: "Calories (kcal)" })
    ).toHaveValue(165)
    expect(screen.getByRole("spinbutton", { name: "Protein (g)" })).toHaveValue(
      31
    )
    expect(screen.getByRole("spinbutton", { name: "Fat (g)" })).toHaveValue(3.6)
  })

  test("shows validation error for empty name", async () => {
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor />)

    await screen.findByLabelText("Name")
    const saveButton = screen.getByRole("button", { name: "Save" })
    // Save is disabled when name empty
    expect(saveButton).toBeDisabled()
    await user.click(saveButton)
    expect(createFood).not.toHaveBeenCalled()
  })

  test("calls createFood on submit", async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    vi.mocked(createFood).mockResolvedValue({
      ...mockChickenBreast,
      id: 2,
      name: "Tofu",
    })

    renderWithRouter(<IngredientEditor onSuccess={onSuccess} />)

    const nameInput = await screen.findByLabelText("Name")
    await user.type(nameInput, "Tofu")

    const kcalInput = screen.getByLabelText("Calories (kcal)")
    await user.clear(kcalInput)
    await user.type(kcalInput, "76")

    const saveButton = screen.getByRole("button", { name: "Save" })
    await user.click(saveButton)

    await waitFor(() => {
      expect(createFood).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "Tofu",
          kcal_100g: 76,
        })
      )
    })

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
  })

  test("calls updateFood on submit in edit mode", async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    vi.mocked(updateFood).mockResolvedValue({
      ...mockChickenBreast,
      kcal_100g: 170,
    })

    renderWithRouter(
      <IngredientEditor ingredient={mockChickenBreast} onSuccess={onSuccess} />,
      "/ingredients/1/edit"
    )

    const kcalInput = await screen.findByLabelText("Calories (kcal)")
    await user.clear(kcalInput)
    await user.type(kcalInput, "170")

    const saveButton = screen.getByRole("button", { name: "Save" })
    await user.click(saveButton)

    await waitFor(() => {
      expect(updateFood).toHaveBeenCalledWith(
        1,
        expect.objectContaining({
          name: "Chicken breast",
          kcal_100g: 170,
        })
      )
    })

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
  })

  test("lookup panel populates form from candidate", async () => {
    const user = userEvent.setup()
    vi.mocked(lookupFoods).mockResolvedValue(mockLookupResponse)

    renderWithRouter(<IngredientEditor />)

    const input = await screen.findByPlaceholderText(
      /search by name or barcode/i
    )
    await user.type(input, "chicken")

    // Wait for the detail preview to render, then hit Apply.
    await screen.findByText("Chicken Breast, Raw")
    const applyButton = await screen.findByRole("button", {
      name: /use this match/i,
    })
    await user.click(applyButton)

    expect(await screen.findByLabelText("Name")).toHaveValue(
      "Chicken Breast, Raw"
    )
    expect(
      screen.getByRole("spinbutton", { name: "Calories (kcal)" })
    ).toHaveValue(120)
    expect(screen.getByRole("spinbutton", { name: "Protein (g)" })).toHaveValue(
      22.5
    )
    expect(screen.getByRole("spinbutton", { name: "Fat (g)" })).toHaveValue(2.6)
  })

  test("applying a lookup candidate without image_url preserves staged image", async () => {
    const user = userEvent.setup()
    vi.mocked(lookupFoods).mockResolvedValue(mockLookupResponse)

    renderWithRouter(<IngredientEditor />)

    await user.click(await screen.findByTestId("stage-image"))
    expect(screen.getByTestId("staged-blob-state")).toHaveTextContent(
      "has-blob"
    )

    const input = screen.getByPlaceholderText(/search by name or barcode/i)
    await user.type(input, "chicken")

    await screen.findByText("Chicken Breast, Raw")
    await user.click(
      await screen.findByRole("button", { name: /use this match/i })
    )

    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toHaveValue("Chicken Breast, Raw")
    })
    // Staged image must still be present — candidate has no image_url.
    expect(screen.getByTestId("staged-blob-state")).toHaveTextContent(
      "has-blob"
    )
  })

  test("shows server error message", async () => {
    const user = userEvent.setup()
    vi.mocked(createFood).mockRejectedValue(
      new ApiError(409, "error.ingredient.duplicate_name")
    )

    renderWithRouter(<IngredientEditor />)

    const nameInput = await screen.findByLabelText("Name")
    await user.type(nameInput, "Chicken breast")

    const saveButton = screen.getByRole("button", { name: "Save" })
    await user.click(saveButton)

    expect(
      await screen.findByText("An ingredient with this name already exists.")
    ).toBeInTheDocument()
  })

  test("refetch button is hidden when no source IDs are stored", async () => {
    renderWithRouter(<IngredientEditor ingredient={mockChickenBreast} />)
    await screen.findByLabelText("Name")
    expect(screen.queryByTestId("ingredient-refetch")).not.toBeInTheDocument()
  })

  test("refetch button calls API and updates form values", async () => {
    const user = userEvent.setup()
    const withFdc = { ...mockChickenBreast, fdc_id: "171077" }
    vi.mocked(refetchFood).mockResolvedValue({
      ...withFdc,
      kcal_100g: 170,
      protein_100g: 32,
      sugar_100g: 0.5,
    })

    renderWithRouter(<IngredientEditor ingredient={withFdc} />)

    const refetchButton = await screen.findByTestId("ingredient-refetch")
    await user.click(refetchButton)

    await waitFor(() => {
      expect(refetchFood).toHaveBeenCalledWith(withFdc.id, undefined)
    })
    await waitFor(() => {
      expect(
        screen.getByRole("spinbutton", { name: "Calories (kcal)" })
      ).toHaveValue(170)
    })
    expect(screen.getByRole("spinbutton", { name: "Protein (g)" })).toHaveValue(
      32
    )
  })

  test("refetch button surfaces API errors via toast", async () => {
    const user = userEvent.setup()
    const withFdc = { ...mockChickenBreast, fdc_id: "171077" }
    const apiError = new ApiError(404, "error.ingredient.refetch.no_results")
    vi.mocked(refetchFood).mockRejectedValue(apiError)

    renderWithRouter(<IngredientEditor ingredient={withFdc} />)

    const refetchButton = await screen.findByTestId("ingredient-refetch")
    await user.click(refetchButton)

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith(
        apiError,
        expect.any(Function)
      )
    })
  })

  test("staged image uploads after create using returned id", async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    vi.mocked(createFood).mockResolvedValue({
      ...mockChickenBreast,
      id: 42,
      name: "Tofu",
    })
    vi.mocked(uploadFoodImage).mockResolvedValue({ image_path: "p.jpg" })

    renderWithRouter(<IngredientEditor onSuccess={onSuccess} />)

    const nameInput = await screen.findByLabelText("Name")
    await user.type(nameInput, "Tofu")

    await user.click(screen.getByTestId("stage-image"))
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() => {
      expect(createFood).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(uploadFoodImage).toHaveBeenCalledWith(42, expect.any(Blob))
    })
    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
  })

  test("upload failure after create surfaces toast and still calls onSuccess", async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    vi.mocked(createFood).mockResolvedValue({
      ...mockChickenBreast,
      id: 99,
      name: "Lentils",
    })
    vi.mocked(uploadFoodImage).mockRejectedValue(
      new ApiError(500, "error.server")
    )

    renderWithRouter(<IngredientEditor onSuccess={onSuccess} />)

    const nameInput = await screen.findByLabelText("Name")
    await user.type(nameInput, "Lentils")

    await user.click(screen.getByTestId("stage-image"))
    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() => {
      expect(uploadFoodImage).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
  })

  test("staged portions are saved after create using returned id", async () => {
    const user = userEvent.setup()
    const onSuccess = vi.fn()
    vi.mocked(createFood).mockResolvedValue({
      ...mockChickenBreast,
      id: 77,
      name: "Sourdough",
    })
    const { upsertPortion } = await import("@/lib/api/foods")
    vi.mocked(upsertPortion).mockResolvedValue({
      food_id: 77,
      unit: "slice",
      grams: 45,
    })

    renderWithRouter(<IngredientEditor onSuccess={onSuccess} />)

    const nameInput = await screen.findByLabelText("Name")
    await user.type(nameInput, "Sourdough")

    await user.click(screen.getByTestId("portion-unit"))
    await user.click(await screen.findByTestId("unit-option-slice"))
    await user.type(screen.getByTestId("portion-grams"), "45")
    await user.click(screen.getByRole("button", { name: /add portion/i }))

    // Row appears in the staged list.
    expect(await screen.findByText("45 g")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Save" }))

    await waitFor(() => {
      expect(createFood).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(upsertPortion).toHaveBeenCalledWith(77, {
        unit: "slice",
        grams: 45,
      })
    })
    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled()
    })
  })

  test("shows tooltip explaining save is disabled when name is empty", async () => {
    const user = userEvent.setup()
    renderWithRouter(<IngredientEditor />)

    const wrapper = await screen.findByTestId("save-disabled-wrapper")
    await user.hover(wrapper)

    expect(
      await screen.findByRole("tooltip", { name: /enter a name to save/i })
    ).toBeInTheDocument()
  })
})
