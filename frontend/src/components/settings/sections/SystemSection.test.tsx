import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import i18n from "@/lib/i18n"
import { mockProfile, mockSettingsList, mockSystemInfo } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
  listAIModels: vi.fn(),
  getSystemInfo: vi.fn(),
}))
vi.mock("@/lib/api/profile", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/profile")>()),
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))

import { getProfile, updateProfile, type Profile } from "@/lib/api/profile"
import { getSystemInfo, listSettings, setSetting } from "@/lib/api/settings"

import { SystemSection } from "./SystemSection"

function renderSection() {
  return renderWithRouter(
    <ThemeProvider>
      <TooltipProvider>
        <SystemSection />
      </TooltipProvider>
    </ThemeProvider>
  )
}

describe("SystemSection", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => ({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }))
    )
    vi.mocked(listSettings).mockResolvedValue(mockSettingsList())
    vi.mocked(setSetting).mockResolvedValue(undefined)
    vi.mocked(getSystemInfo).mockResolvedValue(mockSystemInfo)
    vi.mocked(getProfile).mockResolvedValue(mockProfile)
    vi.mocked(updateProfile).mockImplementation(
      async (input) => ({ ...mockProfile, ...input }) as Profile
    )
  })

  it("renders language, theme, FDC key, setup link, developer flag and version", async () => {
    renderSection()
    expect(await screen.findByRole("radio", { name: "English" })).toBeChecked()
    expect(
      screen.getByText("Plantry shows its labels in English.")
    ).toBeInTheDocument()
    expect(screen.getByRole("radio", { name: "System" })).toBeInTheDocument()
    expect(screen.getByTestId("fdc-key-input")).toBeInTheDocument()
    expect(
      screen.getByRole("link", { name: /re-run setup wizard/i })
    ).toHaveAttribute("href", "/setup")
    expect(screen.getByTestId("debug-workflow-toggle")).not.toBeChecked()
    expect(await screen.findByText("1.2.3")).toBeInTheDocument()
    expect(screen.getByText("abc1234")).toBeInTheDocument()
  })

  it("has no secret-key banner when the cipher is unavailable", async () => {
    vi.mocked(getSystemInfo).mockResolvedValue({
      ...mockSystemInfo,
      cipher_available: false,
    })
    renderSection()
    await screen.findByText("1.2.3")
    expect(screen.queryByText(/encryption key/i)).toBeNull()
  })

  it("switching the language saves profile.locale with the full profile", async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(await screen.findByRole("radio", { name: "Deutsch" }))
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({
          locale: "de",
          diet_type: "omnivore",
          household_portions: 1,
          meat_days_per_week_max: null,
        })
      )
    )
    await i18n.changeLanguage("en")
  })

  it("saves a new FDC key with its button", async () => {
    const user = userEvent.setup()
    renderSection()
    await user.type(await screen.findByTestId("fdc-key-input"), "fdc-123")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith("fdc.api_key", "fdc-123")
    )
  })
})
