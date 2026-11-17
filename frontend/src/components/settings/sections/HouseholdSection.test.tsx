import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { mockProfile } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/profile", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/profile")>()),
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))

import { getProfile, updateProfile, type Profile } from "@/lib/api/profile"

import { HouseholdSection } from "./HouseholdSection"

function echoUpdate() {
  vi.mocked(updateProfile).mockImplementation(
    async (input) =>
      ({
        ...mockProfile,
        ...input,
      }) as Profile
  )
}

describe("HouseholdSection", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getProfile).mockResolvedValue(mockProfile)
    echoUpdate()
  })

  it("renders every field with the defaults and its effect", async () => {
    renderWithRouter(<HouseholdSection />)
    expect(await screen.findByRole("radio", { name: "Omnivore" })).toBeChecked()
    expect(
      screen.getByText("Plantry plans from all your recipes.")
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Each plate is planned for 1 portion\./)
    ).toBeInTheDocument()
    expect(
      screen.getByText("None set. Plantry plans from all your recipes.")
    ).toBeInTheDocument()
    expect(screen.getByTestId("meat-days-select")).toHaveTextContent("No limit")
    expect(
      screen.getByText(
        "No goal set. Plantry doesn't balance plates for nutrition."
      )
    ).toBeInTheDocument()
    for (const name of [/^Cut/, /^Maintain/, /^Bulk/, /^Custom/]) {
      expect(screen.getByRole("radio", { name })).not.toBeChecked()
    }
  })

  it("the household helper writes household_portions with the full profile", async () => {
    renderWithRouter(<HouseholdSection />)
    await userEvent.click(
      await screen.findByRole("button", { name: "Increase Children" })
    )
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({
          household_portions: 1.5,
          diet_type: "omnivore",
          meat_days_per_week_max: null,
          locale: "en",
        })
      )
    )
    expect(
      await screen.findByText(/Each plate is planned for 1\.5 portions\./)
    ).toBeInTheDocument()
    expect(await screen.findByText("Saved")).toBeInTheDocument()
  })

  it("saves the diet type and hides the meat-days limit for vegans", async () => {
    renderWithRouter(<HouseholdSection />)
    await userEvent.click(await screen.findByRole("radio", { name: "Vegan" }))
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({ diet_type: "vegan" })
      )
    )
    expect(
      await screen.findByText("Plantry plans only plant-based recipes.")
    ).toBeInTheDocument()
    expect(screen.queryByTestId("meat-days-select")).toBeNull()
  })

  it("picking the Cut card saves the D6 numbers", async () => {
    renderWithRouter(<HouseholdSection />)
    await userEvent.click(await screen.findByRole("radio", { name: /^Cut/ }))
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({
          kcal_target: 1800,
          protein_pct: 35,
          fat_pct: 30,
          carbs_pct: 35,
        })
      )
    )
    expect(
      await screen.findByText(
        "Plantry aims for 1800 kcal a day: 35 % protein, 30 % fat, 35 % carbs."
      )
    ).toBeInTheDocument()
  })

  it("custom macros over 100 % show an error and are not saved", async () => {
    const user = userEvent.setup()
    renderWithRouter(<HouseholdSection />)
    await user.click(await screen.findByRole("radio", { name: /^Custom/ }))
    await user.type(screen.getByLabelText(/calorie target/i), "2000")
    await user.type(screen.getByLabelText(/protein %/i), "60")
    await user.type(screen.getByLabelText(/fat %/i), "30")
    await user.type(screen.getByLabelText(/carbs %/i), "30")
    await user.click(document.body)
    expect(
      await screen.findByText(
        "Protein, fat and carbs add up to more than 100 %."
      )
    ).toBeInTheDocument()
    expect(updateProfile).not.toHaveBeenCalled()
  })

  it("valid custom targets save when focus leaves the fields", async () => {
    const user = userEvent.setup()
    renderWithRouter(<HouseholdSection />)
    await user.click(await screen.findByRole("radio", { name: /^Custom/ }))
    await user.type(screen.getByLabelText(/calorie target/i), "2000")
    await user.type(screen.getByLabelText(/protein %/i), "40")
    await user.type(screen.getByLabelText(/fat %/i), "30")
    await user.type(screen.getByLabelText(/carbs %/i), "30")
    expect(updateProfile).not.toHaveBeenCalled()
    await user.click(document.body)
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({
          kcal_target: 2000,
          protein_pct: 40,
          fat_pct: 30,
          carbs_pct: 30,
        })
      )
    )
  })

  it("adds an allergy chip and saves dietary_restrictions", async () => {
    const user = userEvent.setup()
    renderWithRouter(<HouseholdSection />)
    await user.type(
      await screen.findByLabelText("Allergies and restrictions"),
      "peanuts{Enter}"
    )
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({ dietary_restrictions: ["peanuts"] })
      )
    )
    expect(
      await screen.findByRole("button", { name: "Remove peanuts" })
    ).toBeInTheDocument()
  })

  it("saves the goal with the full profile", async () => {
    renderWithRouter(<HouseholdSection />)
    await userEvent.click(
      await screen.findByRole("radio", { name: "Lose weight" })
    )
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({ goal: "lose", kcal_target: null })
      )
    )
  })

  it("asks for body data before suggesting a target", async () => {
    renderWithRouter(<HouseholdSection />)
    expect(
      await screen.findByText(/Enter weight, height, birth year/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: /Use as my kcal target/ })
    ).toBeNull()
  })

  it("suggests an estimated target and applies it", async () => {
    vi.mocked(getProfile).mockResolvedValue({
      ...mockProfile,
      goal: "lose",
      weight_kg: 80,
      height_cm: 180,
      birth_year: new Date().getFullYear() - 40,
      sex: "male",
      activity_level: 1.6,
    })
    renderWithRouter(<HouseholdSection />)
    expect(await screen.findByText(/Estimated need: about/)).toHaveTextContent(
      /2,270 kcal/
    )
    expect(
      screen.getByText(/An estimate \(Mifflin-St Jeor/)
    ).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole("button", { name: /Use as my kcal target/ })
    )
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({ kcal_target: 2270, goal: "lose" })
      )
    )
  })

  it("saves a body field on blur and clears it when emptied", async () => {
    renderWithRouter(<HouseholdSection />)
    const weight = await screen.findByLabelText(/^Weight/)
    await userEvent.type(weight, "72.5")
    await userEvent.tab()
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({ weight_kg: 72.5 })
      )
    )
  })

  it("saves the protein factor override and ignores values outside 0.8 to 2.2", async () => {
    renderWithRouter(<HouseholdSection />)
    const factor = await screen.findByLabelText(/^Protein per kg/)
    await userEvent.type(factor, "0.5")
    await userEvent.tab()
    expect(updateProfile).not.toHaveBeenCalled()
    await userEvent.clear(factor)
    await userEvent.type(factor, "1.8")
    await userEvent.tab()
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({ protein_g_per_kg: 1.8 })
      )
    )
  })
})
