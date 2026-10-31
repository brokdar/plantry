import { act, render, screen } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"

import { BatchBadge, type BatchBadgeProps } from "./BatchBadge"

afterEach(async () => {
  await act(() => i18n.changeLanguage("en"))
})

const cases: Array<{
  props: BatchBadgeProps
  en: string
  de: string
}> = [
  {
    props: { sessionId: 7, colour: 1, variant: "cook", portions: 6 },
    en: "Cook × 6",
    de: "Kochen × 6",
  },
  {
    props: { sessionId: 7, colour: 1, variant: "cook", portions: 3.5 },
    en: "Cook × 3½",
    de: "Kochen × 3½",
  },
  {
    props: {
      sessionId: 7,
      colour: 1,
      variant: "cook",
      portions: 6,
      strip: { weekday: "Sun", name: "Chili" },
    },
    en: "Sun · Chili × 6",
    de: "Sun · Chili × 6",
  },
  {
    props: {
      sessionId: 7,
      colour: 1,
      variant: "leftover",
      position: 2,
      meals: 3,
    },
    en: "2/3",
    de: "2/3",
  },
  {
    props: {
      sessionId: 7,
      colour: 1,
      variant: "frozen",
      position: 2,
      meals: 3,
    },
    en: "2/3 · frozen",
    de: "2/3 · gefroren",
  },
  {
    props: { sessionId: 7, colour: 1, variant: "surplus", count: 2 },
    en: "2 spare",
    de: "2 übrig",
  },
  {
    props: { sessionId: 7, colour: 1, variant: "toFreeze", count: 2 },
    en: "2 to freeze",
    de: "2 zum Einfrieren",
  },
]

describe("BatchBadge texts (F2-AC2)", () => {
  it.each(cases)("$props.variant → en $en", ({ props, en }) => {
    const { container } = render(<BatchBadge {...props} />)
    expect(container.querySelector("[data-batch-badge]")?.textContent).toBe(en)
  })

  it.each(cases)("$props.variant → de $de", async ({ props, de }) => {
    await act(() => i18n.changeLanguage("de"))
    const { container } = render(<BatchBadge {...props} />)
    expect(container.querySelector("[data-batch-badge]")?.textContent).toBe(de)
  })
})

describe("BatchBadge appearance", () => {
  it("fills solid variants with the batch colour and ground text", () => {
    render(
      <BatchBadge
        sessionId={7}
        colour={3}
        variant="leftover"
        position={2}
        meals={3}
      />
    )
    const badge = screen.getByText("2/3").closest("[data-batch-badge]")!
    expect(badge).toHaveClass("bg-batch-3", "text-ground", "rounded-tag")
    expect(badge).toHaveAttribute("data-colour", "3")
  })

  it("outlines surplus badges with a dashed border in the batch colour", () => {
    render(<BatchBadge sessionId={7} colour={2} variant="surplus" count={2} />)
    const badge = screen.getByText("2 spare").closest("[data-batch-badge]")!
    expect(badge).toHaveClass("border-dashed", "border-batch-2", "text-batch-2")
    expect(badge).not.toHaveClass("bg-batch-2")
  })

  it("marks frozen portions with a snowflake icon next to the text", () => {
    render(
      <BatchBadge
        sessionId={7}
        colour={1}
        variant="frozen"
        position={2}
        meals={3}
      />
    )
    const badge = screen
      .getByText("2/3 · frozen")
      .closest("[data-batch-badge]")!
    expect(badge.querySelector("svg")).toHaveAttribute("aria-hidden", "true")
  })
})
