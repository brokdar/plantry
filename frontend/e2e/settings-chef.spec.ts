import { API, apiRequest, expect, mockSettingValues, test } from "./helpers"

/**
 * chef.enabled is process-global, so this spec runs in the global-settings
 * project and clears the override it writes. The provider is page-mocked:
 * the switch shows only with a usable provider (ON-1).
 */

async function clearChefEnabled() {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete("/api/settings/chef.enabled")
  await ctx.dispose()
}

test.afterEach(clearChefEnabled)

test("the Chef switch turns on with a provider and shows this month's spend", async ({
  page,
}) => {
  await clearChefEnabled()
  await mockSettingValues(page, { "ai.provider": "fake" })
  await page.goto("/settings/assistant")

  const toggle = page.getByLabel("Chef", { exact: true })
  await expect(toggle).not.toBeChecked()
  await expect(page.getByTestId("chef-spend")).toHaveText(
    /^This month: \$\d+\.\d{2,4} of \$5\.00/
  )

  // Not optimistic: the box follows the saved value.
  await toggle.click()
  await expect(toggle).toBeChecked()
  await expect(
    page.getByText("The assistant labels your recipes", { exact: false })
  ).toBeVisible()

  const ctx = await apiRequest.newContext({ baseURL: API })
  const body = (await (await ctx.get("/api/settings")).json()) as {
    items: { key: string; value?: string }[]
  }
  await ctx.dispose()
  expect(body.items.find((i) => i.key === "chef.enabled")?.value).toBe("true")
  // The save's settings refetch may still be in the mocked route.
  await page.unrouteAll({ behavior: "ignoreErrors" })
})

test("the Chef switch is hidden without a provider", async ({ page }) => {
  await mockSettingValues(page, { "ai.provider": "" })
  await page.goto("/settings/assistant")
  await expect(page.getByLabel("Daily suggestion")).toBeVisible()
  await expect(page.getByLabel("Chef", { exact: true })).toHaveCount(0)
})
