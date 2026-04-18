import { test, expect } from "./helpers"

import {
  cleanupComponent,
  createVariantViaAPI,
  seedComponent,
  uid,
} from "./helpers"

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
        page.getByRole("heading", { name: `Chicken Curry ${tag}` })
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
      await expect(
        page.getByRole("heading", { name: variant.name })
      ).toBeVisible()

      // Variant's "Other variants" section shows the parent.
      await expect(
        page.getByRole("link", { name: new RegExp(`Chicken Curry ${tag}`) })
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

    let variantId: number | undefined
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
      variantId = variant.id

      // Should navigate to the variant's edit page.
      await expect(page.getByLabel(/^name/i)).toBeVisible()
    } finally {
      if (variantId) await cleanupComponent(variantId)
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
