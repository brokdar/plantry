import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import type { SettingItem } from "@/lib/api/settings"
import { mockProfile, mockSettingsList } from "@/test/fixtures"
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
import {
  clearSetting,
  listAIModels,
  listSettings,
  setSetting,
} from "@/lib/api/settings"

import { AssistantSection } from "./AssistantSection"

/** A settings "server": GET returns what PUT stored. */
function serveSettings(overrides: Partial<SettingItem>[] = []) {
  const stored = new Map<string, Partial<SettingItem>>(
    overrides.map((o) => [o.key!, o])
  )
  vi.mocked(listSettings).mockImplementation(async () =>
    mockSettingsList([...stored.values()])
  )
  vi.mocked(setSetting).mockImplementation(async (key, value) => {
    stored.set(key, { key, value, source: "db" })
  })
}

function renderSection() {
  return renderWithRouter(
    <TooltipProvider>
      <AssistantSection />
    </TooltipProvider>
  )
}

describe("AssistantSection", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    serveSettings()
    vi.mocked(clearSetting).mockResolvedValue(undefined)
    vi.mocked(getProfile).mockResolvedValue(mockProfile)
    vi.mocked(updateProfile).mockImplementation(
      async (input) => ({ ...mockProfile, ...input }) as Profile
    )
    vi.mocked(listAIModels).mockResolvedValue({
      models: [
        { id: "fake-default", display_name: "Fake (default)" },
        { id: "fake-tools", display_name: "Fake (tools)" },
      ],
      validated: true,
    })
  })

  it("renders the new fields with their defaults", async () => {
    renderSection()
    expect(await screen.findByLabelText("Notes to your assistant")).toHaveValue(
      ""
    )
    expect(screen.getByLabelText("Daily suggestion")).toBeChecked()
    expect(
      screen.getByText("Plantry suggests one new recipe each day.")
    ).toBeInTheDocument()
    expect(screen.getByTestId("run-retention-input")).toHaveValue(180)
    expect(
      screen.getByText("Assistant run details older than 180 days are deleted.")
    ).toBeInTheDocument()
    expect(screen.getByTestId("ai-rate-limit-input")).toHaveValue(10)
  })

  it("never shows a secret-key banner, even without a cipher", async () => {
    vi.mocked(listSettings).mockResolvedValue({
      ...mockSettingsList(),
      cipher_available: false,
    })
    renderSection()
    await screen.findByTestId("ai-provider-select")
    expect(screen.queryByTestId("secret-key-banner")).toBeNull()
    expect(screen.queryByText(/encryption key/i)).toBeNull()
  })

  it("saves notes to your assistant on blur as system_prompt", async () => {
    const user = userEvent.setup()
    renderSection()
    const notes = await screen.findByLabelText("Notes to your assistant")
    await waitFor(() => expect(notes).toBeEnabled())
    await user.type(notes, "No coriander")
    expect(updateProfile).not.toHaveBeenCalled()
    await user.tab()
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith(
        expect.objectContaining({
          system_prompt: "No coriander",
          diet_type: "omnivore",
          household_portions: 1,
        })
      )
    )
  })

  it("turns the daily suggestion off", async () => {
    const user = userEvent.setup()
    renderSection()
    await user.click(await screen.findByLabelText("Daily suggestion"))
    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith(
        "discovery.daily_enabled",
        "false"
      )
    )
  })

  it("saves the run retention on blur", async () => {
    const user = userEvent.setup()
    renderSection()
    const input = await screen.findByTestId("run-retention-input")
    await user.clear(input)
    await user.type(input, "30")
    await user.tab()
    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith("ai.run_retention_days", "30")
    )
  })

  describe("provider controls (moved from the AI tab)", () => {
    it("disables the API-key input until a provider is selected", async () => {
      renderSection()
      expect(await screen.findByTestId("ai-api-key-input")).toBeDisabled()
    })

    it("enables the API key input once a provider is chosen", async () => {
      serveSettings([{ key: "ai.provider", value: "fake", source: "db" }])
      renderSection()
      const input = await screen.findByTestId("ai-api-key-input")
      await waitFor(() => expect(input).not.toBeDisabled())
    })

    it("populates models after Test connection succeeds", async () => {
      const user = userEvent.setup()
      serveSettings([{ key: "ai.provider", value: "fake", source: "db" }])
      renderSection()
      const input = await screen.findByTestId("ai-api-key-input")
      await waitFor(() => expect(input).not.toBeDisabled())
      await user.type(input, "sk-dummy")
      await user.click(await screen.findByTestId("ai-test-connection"))
      await waitFor(() => expect(listAIModels).toHaveBeenCalled())
      const modelSelect = await screen.findByTestId("ai-model-select")
      await waitFor(() => expect(modelSelect).not.toBeDisabled())
    })

    it("issues DELETE /api/settings/<key> when 'Clear override' is clicked", async () => {
      const user = userEvent.setup()
      serveSettings([
        {
          key: "ai.provider",
          value: "anthropic",
          source: "db",
          env_also_set: true,
        },
      ])
      renderSection()
      await user.click(await screen.findByTestId("clear-override-ai.provider"))
      await waitFor(() =>
        expect(clearSetting).toHaveBeenCalledWith("ai.provider")
      )
    })

    it("posts a DB override when the provider changes", async () => {
      const user = userEvent.setup()
      renderSection()
      await user.click(await screen.findByTestId("ai-provider-select"))
      await user.click(
        await screen.findByRole("option", { name: /fake \(testing\)/i })
      )
      await waitFor(() =>
        expect(setSetting).toHaveBeenCalledWith("ai.provider", "fake")
      )
    })
  })

  it("links to Insights and holds the Prices group", async () => {
    renderSection()
    expect(
      await screen.findByRole("link", { name: "Open insights" })
    ).toHaveAttribute("href", "/settings/assistant/insights")
    expect(screen.getByRole("heading", { name: "Prices" })).toBeInTheDocument()
    expect(screen.getByText("Choose a model to see its price.")).toBeVisible()
  })
})
