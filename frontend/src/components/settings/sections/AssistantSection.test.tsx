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

vi.mock("@/lib/api/chef", () => ({
  getChefSpend: vi.fn(),
  getLabellingEstimate: vi.fn(),
  startLibraryLabelling: vi.fn(),
  listChefSites: vi.fn(),
  addChefSite: vi.fn(),
  setChefSiteEnabled: vi.fn(),
  removeChefSite: vi.fn(),
  retryChefSite: vi.fn(),
}))

vi.mock("@/lib/api/profile", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/profile")>()),
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
}))

import {
  getChefSpend,
  getLabellingEstimate,
  listChefSites,
  startLibraryLabelling,
} from "@/lib/api/chef"
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
    vi.mocked(getChefSpend).mockResolvedValue({
      month: "2026-11",
      spent_usd: 1.25,
      cap_usd: 5,
      at_cap: false,
    })
    vi.mocked(getLabellingEstimate).mockResolvedValue({
      recipes: 24,
      calls: 3,
      cost_usd: 0.12,
      minutes: 1,
    })
    vi.mocked(startLibraryLabelling).mockResolvedValue({ jobs: 3 })
    vi.mocked(listChefSites).mockResolvedValue({ items: [] })
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

  describe("Chef switch (ON-1)", () => {
    it("is hidden without a provider key", async () => {
      serveSettings([{ key: "ai.provider", value: "openai", source: "db" }])
      renderSection()
      await screen.findByLabelText("Daily suggestion")
      expect(screen.queryByLabelText("Chef")).toBeNull()
      expect(getChefSpend).not.toHaveBeenCalled()
    })

    it("turns on with a key and shows this month's spend", async () => {
      serveSettings([
        { key: "ai.provider", value: "anthropic", source: "db" },
        {
          key: "ai.api_key",
          value: "",
          source: "db",
          masked_preview: "sk-****abcd",
        },
      ])
      const user = userEvent.setup()
      renderSection()
      const toggle = await screen.findByLabelText("Chef")
      expect(toggle).not.toBeChecked()
      expect(await screen.findByTestId("chef-spend")).toHaveTextContent(
        "This month: $1.25 of $5.00"
      )
      await user.click(toggle)
      // ON-2: the estimate first, the switch saves only on Turn on.
      expect(await screen.findByTestId("chef-on-estimate")).toHaveTextContent(
        "24 recipes, about $0.12 and 1 min."
      )
      expect(setSetting).not.toHaveBeenCalled()
      await user.click(screen.getByRole("button", { name: "Turn on" }))
      await waitFor(() => expect(startLibraryLabelling).toHaveBeenCalled())
      expect(setSetting).toHaveBeenCalledWith("chef.enabled", "true")
      await waitFor(() =>
        expect(screen.queryByTestId("chef-on-dialog")).toBeNull()
      )
      expect(toggle).toBeChecked()
    })

    it("stays off when the estimate is cancelled", async () => {
      serveSettings([{ key: "ai.provider", value: "fake", source: "db" }])
      vi.mocked(getLabellingEstimate).mockResolvedValue({
        recipes: 2,
        calls: 1,
        cost_usd: null,
        minutes: 1,
      })
      const user = userEvent.setup()
      renderSection()
      await user.click(await screen.findByLabelText("Chef"))
      expect(await screen.findByTestId("chef-on-estimate")).toHaveTextContent(
        "Your model has no price"
      )
      await user.click(screen.getByRole("button", { name: "Cancel" }))
      expect(setSetting).not.toHaveBeenCalled()
      expect(startLibraryLabelling).not.toHaveBeenCalled()
      expect(screen.getByLabelText("Chef")).not.toBeChecked()
    })

    it("cannot turn on without an estimate", async () => {
      serveSettings([{ key: "ai.provider", value: "fake", source: "db" }])
      vi.mocked(getLabellingEstimate).mockRejectedValue(new Error("boom"))
      const user = userEvent.setup()
      renderSection()
      await user.click(await screen.findByLabelText("Chef"))
      expect(
        await screen.findByTestId("chef-on-estimate")
      ).not.toHaveTextContent("recipes")
      expect(screen.getByRole("button", { name: "Turn on" })).toBeDisabled()
    })

    it("turns off without asking", async () => {
      serveSettings([
        { key: "ai.provider", value: "fake", source: "db" },
        { key: "chef.enabled", value: "true", source: "db" },
      ])
      const user = userEvent.setup()
      renderSection()
      await user.click(await screen.findByLabelText("Chef"))
      await waitFor(() =>
        expect(setSetting).toHaveBeenCalledWith("chef.enabled", "false")
      )
      expect(screen.queryByTestId("chef-on-dialog")).toBeNull()
    })

    it("edits the monthly limit", async () => {
      serveSettings([{ key: "ai.provider", value: "fake", source: "db" }])
      const user = userEvent.setup()
      renderSection()
      const input = await screen.findByLabelText("Monthly limit")
      expect(input).toHaveValue(5)
      await user.clear(input)
      await user.type(input, "8")
      await user.tab()
      await waitFor(() =>
        expect(setSetting).toHaveBeenCalledWith("chef.monthly_cap", "8")
      )
    })

    it("says when the cap is reached", async () => {
      serveSettings([{ key: "ai.provider", value: "fake", source: "db" }])
      vi.mocked(getChefSpend).mockResolvedValue({
        month: "2026-11",
        spent_usd: 5.1,
        cap_usd: 5,
        at_cap: true,
      })
      renderSection()
      expect(await screen.findByTestId("chef-spend")).toHaveTextContent(
        "The monthly limit is reached"
      )
    })
  })

  describe("library review (LIB-1)", () => {
    it("is hidden while the Chef is off", async () => {
      serveSettings([{ key: "ai.provider", value: "fake", source: "db" }])
      renderSection()
      await screen.findByLabelText("Chef")
      expect(screen.queryByLabelText("Monthly library check-up")).toBeNull()
      expect(listChefSites).not.toHaveBeenCalled()
    })

    it("is off by default and switches on with the Chef on", async () => {
      serveSettings([
        { key: "ai.provider", value: "fake", source: "db" },
        { key: "chef.enabled", value: "true", source: "db" },
      ])
      const user = userEvent.setup()
      renderSection()
      const toggle = await screen.findByLabelText("Monthly library check-up")
      expect(toggle).not.toBeChecked()
      await user.click(toggle)
      await waitFor(() =>
        expect(setSetting).toHaveBeenCalledWith(
          "chef.library_review_enabled",
          "true"
        )
      )
    })
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

  it("adds and removes cuisine chips as discovery.cuisines", async () => {
    serveSettings([{ key: "discovery.cuisines", value: '["persian"]' }])
    const user = userEvent.setup()
    renderSection()

    const input = await screen.findByLabelText("Cuisines to explore")
    expect(screen.getByText("persian")).toBeVisible()
    await user.type(input, "  Thai {Enter}")
    await waitFor(() =>
      expect(setSetting).toHaveBeenLastCalledWith(
        "discovery.cuisines",
        '["persian","thai"]'
      )
    )
    expect(input).toHaveValue("")
    // A duplicate is not added again.
    await user.type(input, "PERSIAN")
    await user.click(screen.getByRole("button", { name: "Add" }))
    expect(setSetting).toHaveBeenCalledTimes(1)

    await user.click(
      await screen.findByRole("button", { name: "Remove persian" })
    )
    await waitFor(() =>
      expect(setSetting).toHaveBeenLastCalledWith(
        "discovery.cuisines",
        '["thai"]'
      )
    )
  })

  it("stops adding cuisines at 10", async () => {
    const ten = Array.from({ length: 10 }, (_, i) => `c${i}`)
    serveSettings([{ key: "discovery.cuisines", value: JSON.stringify(ten) }])
    renderSection()

    expect(await screen.findByText("Up to 10 cuisines.")).toBeVisible()
    expect(screen.getByLabelText("Cuisines to explore")).toBeDisabled()
    expect(screen.getByRole("button", { name: "Add" })).toBeDisabled()
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
