import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest"

// jsdom lacks pointer-capture APIs that Radix Select uses. Polyfill the
// minimum surface so the select trigger opens without throwing.
beforeAll(() => {
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false
  }
  if (!Element.prototype.setPointerCapture) {
    Element.prototype.setPointerCapture = () => {}
  }
  if (!Element.prototype.releasePointerCapture) {
    Element.prototype.releasePointerCapture = () => {}
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {}
  }
})

import type { SettingItem, SettingsList, SystemInfo } from "@/lib/api/settings"
import { TooltipProvider } from "@/components/ui/tooltip"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/settings", () => ({
  listSettings: vi.fn(),
  setSetting: vi.fn(),
  clearSetting: vi.fn(),
  listAIModels: vi.fn(),
  getSystemInfo: vi.fn(),
}))

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import {
  clearSetting,
  listAIModels,
  listSettings,
  setSetting,
  getSystemInfo,
} from "@/lib/api/settings"

import { AITab } from "./AITab"

function item(
  key: string,
  value: string,
  source: SettingItem["source"] = "default",
  extra: Partial<SettingItem> = {}
): SettingItem {
  return {
    key,
    value,
    source,
    is_secret: false,
    env_also_set: false,
    ...extra,
  }
}

function settingsFixture(overrides: Partial<SettingItem>[] = []): SettingsList {
  const base: SettingItem[] = [
    item("ai.provider", "", "default"),
    item("ai.model", "", "default"),
    {
      ...item("ai.api_key", "", "default"),
      is_secret: true,
    },
    item("ai.rate_limit_per_min", "10", "default"),
    item("ai.fake_script", "", "default"),
    {
      ...item("fdc.api_key", "", "default"),
      is_secret: true,
    },
  ]
  const merged = base.map((it) => {
    const override = overrides.find((o) => o.key === it.key)
    return override ? { ...it, ...override } : it
  })
  return { items: merged, cipher_available: true }
}

const systemInfo: SystemInfo = {
  port: 8080,
  db_path: "/data/plantry.db",
  log_level: "info",
  image_path: "/data/images",
  dev_mode: false,
  version: "test",
  build_commit: "test",
  cipher_available: true,
}

function renderTab() {
  return renderWithRouter(
    <TooltipProvider>
      <AITab />
    </TooltipProvider>
  )
}

describe("AITab", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listSettings).mockResolvedValue(settingsFixture())
    vi.mocked(getSystemInfo).mockResolvedValue(systemInfo)
    vi.mocked(setSetting).mockResolvedValue(undefined)
    vi.mocked(clearSetting).mockResolvedValue(undefined)
    vi.mocked(listAIModels).mockResolvedValue({
      models: [
        { id: "fake-default", display_name: "Fake (default)" },
        { id: "fake-tools", display_name: "Fake (tools)" },
      ],
      validated: true,
    })
  })

  it("disables step 2 API-key input until a provider is selected", async () => {
    renderTab()
    const input = await screen.findByTestId("ai-api-key-input")
    expect(input).toBeDisabled()
  })

  it("enables API key input once a provider is chosen", async () => {
    vi.mocked(listSettings).mockResolvedValue(
      settingsFixture([{ key: "ai.provider", value: "fake", source: "db" }])
    )
    renderTab()
    const input = await screen.findByTestId("ai-api-key-input")
    await waitFor(() => expect(input).not.toBeDisabled())
  })

  it("populates models and enables step 3 after Test connection succeeds", async () => {
    const user = userEvent.setup()
    vi.mocked(listSettings).mockResolvedValue(
      settingsFixture([{ key: "ai.provider", value: "fake", source: "db" }])
    )
    renderTab()

    const input = await screen.findByTestId("ai-api-key-input")
    await user.type(input, "sk-dummy")
    await user.click(await screen.findByTestId("ai-test-connection"))

    await waitFor(() => expect(listAIModels).toHaveBeenCalled())
    const modelSelect = await screen.findByTestId("ai-model-select")
    await waitFor(() => expect(modelSelect).not.toBeDisabled())
  })

  it("shows the secret-key banner when the cipher is unavailable", async () => {
    vi.mocked(listSettings).mockResolvedValue({
      ...settingsFixture(),
      cipher_available: false,
    })
    vi.mocked(getSystemInfo).mockResolvedValue({
      ...systemInfo,
      cipher_available: false,
    })
    renderTab()
    await screen.findByTestId("secret-key-banner")
  })

  it("issues DELETE /api/settings/<key> when 'Clear override' is clicked", async () => {
    const user = userEvent.setup()
    vi.mocked(listSettings).mockResolvedValue(
      settingsFixture([
        {
          key: "ai.provider",
          value: "anthropic",
          source: "db",
          env_also_set: true,
        },
      ])
    )
    renderTab()

    const clear = await screen.findByTestId("clear-override-ai.provider")
    await user.click(clear)

    await waitFor(() =>
      expect(clearSetting).toHaveBeenCalledWith("ai.provider")
    )
  })

  it("posts a DB override when the provider selector changes", async () => {
    const user = userEvent.setup()
    renderTab()

    const providerSelect = await screen.findByTestId("ai-provider-select")
    await user.click(providerSelect)
    // Radix Select renders options to a portal; find by role.
    const fakeOption = await screen.findByRole("option", {
      name: /fake \(testing\)/i,
    })
    await user.click(fakeOption)

    await waitFor(() =>
      expect(setSetting).toHaveBeenCalledWith("ai.provider", "fake")
    )
  })
})
