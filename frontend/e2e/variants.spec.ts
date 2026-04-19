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
      // Editor is the only detail surface — navigate there.
      await page.goto(`/components/${parent.id}/edit`)
      await expect(page.getByLabel(/^name/i)).toHaveValue(
        `Chicken Curry ${tag}`
      )

      // "Other variants" section lists the variant card.
      const section = page.getByTestId("component-variants-section")
      await expect(section).toBeVisible()
      const variantCard = page.getByTestId(`variant-card-${variant.id}`)
      await expect(variantCard).toBeVisible()

      // Click the variant card to navigate to its editor.
      await variantCard.click()
      await expect(page).toHaveURL(
        new RegExp(`/components/${variant.id}/edit$`)
      )
      await expect(page.getByLabel(/^name/i)).toHaveValue(variant.name)

      // The variant editor surfaces the parent in its variants section.
      await expect(page.getByTestId(`variant-card-${parent.id}`)).toBeVisible()
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
      await page.goto(`/components/${parent.id}/edit`)
      await expect(page.getByLabel(/^name/i)).toHaveValue(`Tofu Bowl ${tag}`)

      const responsePromise = page.waitForResponse(
        (res) =>
          res.url().includes(`/api/components/${parent.id}/variant`) &&
          res.request().method() === "POST"
      )
      await page.getByTestId("component-create-variant").click()
      const response = await responsePromise
      expect(response.status()).toBe(201)

      const variant = (await response.json()) as { id: number; name: string }
      variantId = variant.id

      // Should navigate to the variant's edit page.
      await expect(page).toHaveURL(new RegExp(`/components/${variantId}/edit$`))
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
      await page.goto(`/components/${comp.id}/edit`)
      await expect(page.getByLabel(/^name/i)).toHaveValue(
        `Solo Component ${tag}`
      )

      await expect(page.getByTestId("component-variants-section")).toHaveCount(
        0
      )
    } finally {
      await cleanupComponent(comp.id)
    }
  })
})
