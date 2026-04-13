import { test, expect, request as apiRequest } from "@playwright/test"

const API = "http://localhost:8080"

function uid() {
  return crypto.randomUUID().slice(0, 8)
}

async function seedComponent(data: {
  name: string
  role: string
  reference_portions?: number
}) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post("/api/components", {
    data: { reference_portions: 1, ...data },
  })
  const body = await res.json()
  expect(
    res.ok(),
    `Seed component "${data.name}" failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string }
}

async function createVariantViaAPI(parentId: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  const res = await ctx.post(`/api/components/${parentId}/variant`)
  const body = await res.json()
  expect(
    res.ok(),
    `Create variant failed: ${res.status()} ${JSON.stringify(body)}`
  ).toBeTruthy()
  await ctx.dispose()
  return body as { id: number; name: string }
}

async function cleanupComponent(id: number) {
  const ctx = await apiRequest.newContext({ baseURL: API })
  await ctx.delete(`/api/components/${id}`)
  await ctx.dispose()
}

test.describe("Variant Components", () => {
  test("create variant, navigate between siblings via Other variants", async ({
    page,
  }) => {
    const tag = uid()
    const parent = await seedComponent({
      name: `Chicken Curry ${tag}`,
      role: "main",
      reference_portions: 2,
    })

    const variant = await createVariantViaAPI(parent.id)

    try {
      // Navigate to parent detail page.
      await page.goto(`/components/${parent.id}`)
      await expect(
        page.getByText(`Chicken Curry ${tag}`, { exact: false })
      ).toBeVisible()

      // "Other variants" section should show the variant.
      await expect(page.getByText(variant.name)).toBeVisible()

      // Click the variant card to navigate.
      const variantDetailPromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/components/${variant.id}`) &&
          res.request().method() === "GET" &&
          !res.url().includes("/variants")
      )
      await page.getByText(variant.name).click()
      await variantDetailPromise

      // Variant detail page shows the variant name.
      await expect(page.getByText(variant.name)).toBeVisible()

      // Variant's "Other variants" section shows the parent.
      await expect(
        page.getByText(`Chicken Curry ${tag}`, { exact: false })
      ).toBeVisible()
    } finally {
      await cleanupComponent(variant.id)
      await cleanupComponent(parent.id)
    }
  })

  test("create variant via UI button navigates to edit page", async ({
    page,
  }) => {
    const tag = uid()
    const parent = await seedComponent({
      name: `Tofu Bowl ${tag}`,
      role: "standalone",
    })

    try {
      await page.goto(`/components/${parent.id}`)
      await expect(page.getByText(`Tofu Bowl ${tag}`)).toBeVisible()

      // Click "Create variant" button.
      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/components/${parent.id}/variant`) &&
          res.request().method() === "POST"
      )
      await page.getByRole("button", { name: /create variant/i }).click()
      const response = await responsePromise
      expect(response.status()).toBe(201)

      const variant = (await response.json()) as { id: number; name: string }

      // Should navigate to the variant's edit page.
      await expect(page.getByLabel(/^name/i)).toBeVisible()

      // Cleanup the variant too.
      await cleanupComponent(variant.id)
    } finally {
      await cleanupComponent(parent.id)
    }
  })

  test("component with no variants shows no Other variants section", async ({
    page,
  }) => {
    const tag = uid()
    const comp = await seedComponent({
      name: `Solo Component ${tag}`,
      role: "sauce",
    })

    try {
      await page.goto(`/components/${comp.id}`)
      await expect(page.getByText(`Solo Component ${tag}`)).toBeVisible()

      // "Other variants" heading should not appear.
      await expect(
        page.getByRole("heading", { name: /other variants/i })
      ).toHaveCount(0)
    } finally {
      await cleanupComponent(comp.id)
    }
  })
})
