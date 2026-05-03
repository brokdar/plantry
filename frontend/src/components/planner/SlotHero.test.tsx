import { render } from "@testing-library/react"
import { describe, expect, test } from "vitest"

import "@/lib/i18n"
import { SlotHero, type SlotHeroComponent } from "./SlotHero"

function comp(over: Partial<SlotHeroComponent> = {}): SlotHeroComponent {
  return {
    name: "Grilled Chicken",
    role: "main",
    imagePath: null,
    ...over,
  }
}

describe("SlotHero — collage layouts", () => {
  test("renders a single tile with no count badge for one component", () => {
    const { container, queryByText } = render(
      <SlotHero
        components={[comp({ name: "Risotto", role: "main" })]}
        heroRoleLabel="MAIN"
      />
    )
    // 1 component → no count badge, role label still surfaces.
    expect(queryByText("1")).not.toBeInTheDocument()
    expect(container.textContent).toContain("MAIN")
  })

  test("renders a count badge once a plate has more than one component", () => {
    const { getByText } = render(
      <SlotHero
        components={[
          comp({ name: "Risotto" }),
          comp({ name: "Salad", role: "side_veg" }),
          comp({ name: "Bread", role: "side_starch" }),
        ]}
        heroRoleLabel="MAIN"
      />
    )
    expect(getByText("3")).toBeInTheDocument()
  })

  test("3+ components fall back to the same 3-up collage", () => {
    const { getByText, container } = render(
      <SlotHero
        components={[
          comp({ name: "A" }),
          comp({ name: "B" }),
          comp({ name: "C" }),
          comp({ name: "D" }),
        ]}
        heroRoleLabel="MAIN"
      />
    )
    // Count badge reflects the real total even when only 3 panes render.
    expect(getByText("4")).toBeInTheDocument()
    // 3 panes (placeholders) inside the collage.
    expect(
      container.querySelectorAll("[aria-hidden='true']").length
    ).toBeGreaterThanOrEqual(3)
  })
})
