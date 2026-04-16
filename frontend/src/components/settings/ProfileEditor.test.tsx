import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Profile } from "@/lib/api/profile"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/profile", () => ({
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))

import { getProfile, updateProfile } from "@/lib/api/profile"
import { ProfileEditor } from "./ProfileEditor"

const mockProfile: Profile = {
  kcal_target: null,
  protein_pct: null,
  fat_pct: null,
  carbs_pct: null,
  dietary_restrictions: [],
  preferences: {},
  system_prompt: null,
  locale: "en",
  updated_at: "2026-01-01T00:00:00Z",
}

describe("ProfileEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getProfile).mockResolvedValue(mockProfile)
    vi.mocked(updateProfile).mockResolvedValue({
      ...mockProfile,
      kcal_target: 1800,
      protein_pct: 35,
      fat_pct: 30,
      carbs_pct: 35,
    })
  })

  it("renders with loaded profile values", async () => {
    renderWithRouter(<ProfileEditor />)
    await screen.findByRole("button", { name: /cut/i })
    expect(
      screen.getByRole("button", { name: /maintain/i })
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /bulk/i })).toBeInTheDocument()
  })

  it("clicking Cut preset populates correct field values", async () => {
    renderWithRouter(<ProfileEditor />)
    await screen.findByRole("button", { name: /cut/i })

    await userEvent.click(screen.getByRole("button", { name: /cut/i }))

    await waitFor(() => {
      expect(
        (screen.getByLabelText(/calorie target/i) as HTMLInputElement).value
      ).toBe("1800")
    })
    expect(
      (screen.getByLabelText(/protein %/i) as HTMLInputElement).value
    ).toBe("35")
  })

  it("clicking Maintain preset populates maintain values", async () => {
    renderWithRouter(<ProfileEditor />)
    await screen.findByRole("button", { name: /maintain/i })

    await userEvent.click(screen.getByRole("button", { name: /maintain/i }))

    await waitFor(() => {
      expect(
        (screen.getByLabelText(/calorie target/i) as HTMLInputElement).value
      ).toBe("2200")
    })
  })

  it("custom override of a pct field updates independently", async () => {
    renderWithRouter(<ProfileEditor />)
    await screen.findByRole("button", { name: /cut/i })

    await userEvent.click(screen.getByRole("button", { name: /cut/i }))
    await waitFor(() => {
      expect(
        (screen.getByLabelText(/protein %/i) as HTMLInputElement).value
      ).toBe("35")
    })

    const proteinInput = screen.getByLabelText(/protein %/i)
    await userEvent.clear(proteinInput)
    await userEvent.type(proteinInput, "40")

    expect((proteinInput as HTMLInputElement).value).toBe("40")
  })

  it("save calls updateProfile with correct payload", async () => {
    renderWithRouter(<ProfileEditor />)
    await screen.findByRole("button", { name: /cut/i })

    await userEvent.click(screen.getByRole("button", { name: /cut/i }))
    await waitFor(() => {
      expect(
        (screen.getByLabelText(/calorie target/i) as HTMLInputElement).value
      ).toBe("1800")
    })

    await userEvent.click(screen.getByRole("button", { name: /save profile/i }))

    await waitFor(() => {
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({
          kcal_target: 1800,
          protein_pct: 35,
          fat_pct: 30,
          carbs_pct: 35,
        })
      )
    })
  })
})
