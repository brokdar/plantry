import { render, screen, within } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"

import i18n from "@/lib/i18n"
import { mockSuggestion } from "@/test/fixtures"

import { SuggestionCard } from "./SuggestionCard"

beforeEach(async () => {
  await i18n.changeLanguage("en")
})

describe("SuggestionCard (F2-AC1)", () => {
  it("marks only unknown ingredients as new and shows the legend", () => {
    render(<SuggestionCard suggestion={mockSuggestion()} />)
    const rows = screen.getAllByRole("listitem")
    expect(within(rows[0]).queryByText("new")).toBeNull()
    expect(within(rows[1]).getByText("new")).toBeInTheDocument()
    expect(screen.getAllByTestId("suggestion-new")).toHaveLength(1)
    expect(screen.getByTestId("discovery-legend")).toBeInTheDocument()
  })

  it("has no legend when every ingredient is known", () => {
    const s = mockSuggestion()
    render(
      <SuggestionCard
        suggestion={{
          ...s,
          ingredients: s.ingredients.map((i) => ({ ...i, known: true })),
        }}
      />
    )
    expect(screen.queryByTestId("suggestion-new")).toBeNull()
    expect(screen.queryByTestId("discovery-legend")).toBeNull()
  })

  it("shows the badge only when grounded", () => {
    const { rerender } = render(
      <SuggestionCard suggestion={mockSuggestion()} />
    )
    expect(screen.getByTestId("suggestion-grounded")).toBeInTheDocument()
    rerender(
      <SuggestionCard suggestion={mockSuggestion({ grounded: false })} />
    )
    expect(screen.queryByTestId("suggestion-grounded")).toBeNull()
  })

  it("says nutrition is incomplete and names what is missing", () => {
    render(<SuggestionCard suggestion={mockSuggestion()} />)
    const note = screen.getByTestId("suggestion-incomplete")
    expect(note).toHaveTextContent("Nutrition incomplete")
    expect(note).toHaveTextContent("pomegranate molasses")
  })

  it("shows the macros when nutrition is complete", () => {
    render(
      <SuggestionCard
        suggestion={mockSuggestion({ nutrition_complete: true, missing: [] })}
      />
    )
    expect(screen.queryByTestId("suggestion-incomplete")).toBeNull()
    expect(screen.getByText(/520 kcal · 28 g protein/)).toBeInTheDocument()
  })

  it("links the source in a new tab with noopener", () => {
    render(<SuggestionCard suggestion={mockSuggestion()} />)
    const link = screen.getByRole("link", { name: /original recipe/i })
    expect(link).toHaveAttribute("target", "_blank")
    expect(link.getAttribute("rel")).toContain("noopener")
    expect(link).toHaveAttribute("href", mockSuggestion().source_url)
  })

  it("uses the typographic placeholder without a photo", () => {
    const { rerender } = render(
      <SuggestionCard suggestion={mockSuggestion()} />
    )
    expect(screen.getByTestId("suggestion-placeholder")).toHaveTextContent(
      "Fesenjan"
    )
    rerender(
      <SuggestionCard
        suggestion={mockSuggestion({ image_url: "http://x/y.jpg" })}
      />
    )
    expect(screen.queryByTestId("suggestion-placeholder")).toBeNull()
    expect(screen.getByRole("img")).toHaveAttribute("src", "http://x/y.jpg")
  })
})
