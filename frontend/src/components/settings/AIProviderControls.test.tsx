import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { mockSettingsList } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
  listAIModels: vi.fn(),
  getSystemInfo: vi.fn(),
}))

import { listAIModels, listSettings } from "@/lib/api/settings"

import { AIProviderControls } from "./AIProviderControls"

function renderWithProvider(provider: string, variant?: "wizard") {
  vi.mocked(listSettings).mockResolvedValue(
    mockSettingsList([{ key: "ai.provider", value: provider, source: "db" }])
  )
  return renderWithRouter(
    <TooltipProvider>
      <AIProviderControls variant={variant} />
    </TooltipProvider>
  )
}

const models = (n: number) =>
  Array.from({ length: n }, (_, i) => ({ id: `model-${i}` }))

describe("AIProviderControls key check (plan D17)", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it.each([
    ["openai", 3, "Key works, 3 models available"],
    ["fake", 1, "Key works, 1 model available"],
    ["anthropic", 4, "Key saved. 4 models available"],
  ])("%s shows “%s”", async (provider, count, text) => {
    vi.mocked(listAIModels).mockResolvedValue({
      models: models(count),
      validated: true,
    })
    const user = userEvent.setup()
    renderWithProvider(provider)

    await user.type(await screen.findByTestId("ai-api-key-input"), "sk-test")
    await user.click(screen.getByTestId("ai-test-connection"))

    expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it("the wizard variant keeps every row in one column", async () => {
    renderWithProvider("openai", "wizard")
    const select = await screen.findByTestId("ai-provider-select")
    const row = select.closest(".grid") as HTMLElement
    expect(row.className).toContain("lg:grid-cols-1")
    expect(row.className).not.toContain("lg:grid-cols-[")
  })

  it("in Settings the steps are h4 under the group and show the source", async () => {
    renderWithProvider("openai")
    expect(
      await screen.findByRole("heading", { level: 4, name: "Pick a provider" })
    ).toBeInTheDocument()
    expect(screen.getByText("Database")).toBeInTheDocument()
    expect(screen.getByText(/PLANTRY_SECRET_KEY/)).toBeInTheDocument()
  })

  it("in the wizard the steps are h2 under the step title, without developer details", async () => {
    renderWithProvider("openai", "wizard")
    expect(
      await screen.findByRole("heading", { level: 2, name: "Pick a provider" })
    ).toBeInTheDocument()
    expect(screen.queryAllByRole("heading", { level: 4 })).toHaveLength(0)
    expect(screen.queryByText("Database")).toBeNull()
    expect(screen.queryByTestId("clear-override-ai.provider")).toBeNull()
    expect(screen.queryByText(/PLANTRY_SECRET_KEY/)).toBeNull()
    expect(
      screen.getByText(
        "Your key from the provider. Plantry stores it encrypted."
      )
    ).toBeInTheDocument()
  })
})
