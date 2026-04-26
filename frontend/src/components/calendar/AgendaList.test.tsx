import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { vi } from "vitest"

import { AgendaGroup } from "./AgendaGroup"
import { AgendaList } from "./AgendaList"
import { mockPlateW16a, mockPlateW16b, mockPlateW17 } from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/plates")
vi.mock("@/lib/queries/weeks", () => ({
  useCopyWeek: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}))

// ---------------------------------------------------------------------------
// AgendaList — grouping
// ---------------------------------------------------------------------------

describe("AgendaList", () => {
  it("groups plates into 2 ISO-week buckets when plates span 2 weeks", async () => {
    const plates = [mockPlateW16a, mockPlateW16b, mockPlateW17]
    renderWithRouter(
      <AgendaList
        plates={plates}
        hasNextPage={false}
        isFetchingNextPage={false}
        fetchNextPage={vi.fn()}
        search=""
        weekStartsOn={1}
      />
    )

    // Two distinct week group labels should appear.
    // mockPlateW16a + W16b → one group; mockPlateW17 → another group.
    const groups = await screen.findAllByRole("group")
    expect(groups).toHaveLength(2)
  })

  it("shows 'Load older' button when hasNextPage=true and calls fetchNextPage on click", async () => {
    const fetchNextPage = vi.fn()
    renderWithRouter(
      <AgendaList
        plates={[mockPlateW16a]}
        hasNextPage={true}
        isFetchingNextPage={false}
        fetchNextPage={fetchNextPage}
        search=""
        weekStartsOn={1}
      />
    )

    const btn = await screen.findByRole("button", { name: /load older/i })
    expect(btn).toBeInTheDocument()

    await userEvent.click(btn)
    expect(fetchNextPage).toHaveBeenCalledOnce()
  })

  it("hides 'Load older' button when hasNextPage=false", async () => {
    renderWithRouter(
      <AgendaList
        plates={[mockPlateW16a]}
        hasNextPage={false}
        isFetchingNextPage={false}
        fetchNextPage={vi.fn()}
        search=""
        weekStartsOn={1}
      />
    )

    // Wait for render, then assert button absent
    await screen.findAllByRole("group")
    expect(
      screen.queryByRole("button", { name: /load older/i })
    ).not.toBeInTheDocument()
  })

  it("shows empty-state copy when plates=[]", async () => {
    renderWithRouter(
      <AgendaList
        plates={[]}
        hasNextPage={false}
        isFetchingNextPage={false}
        fetchNextPage={vi.fn()}
        search=""
        weekStartsOn={1}
      />
    )

    expect(
      await screen.findByText(/no meals in this range/i)
    ).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// AgendaGroup — collapse / expand
// ---------------------------------------------------------------------------

describe("AgendaGroup", () => {
  it("starts open when defaultOpen=true and shows plate rows", async () => {
    renderWithRouter(
      <AgendaGroup
        weekLabel="2026 W16"
        plates={[mockPlateW16a, mockPlateW16b]}
        defaultOpen={true}
      />
    )

    // Both plate rows visible (they are inside an open <details>)
    const items = await screen.findAllByRole("listitem")
    expect(items).toHaveLength(2)
  })

  it("starts closed when defaultOpen=false and content is not visible", async () => {
    renderWithRouter(
      <AgendaGroup
        weekLabel="2026 W16"
        plates={[mockPlateW16a]}
        defaultOpen={false}
      />
    )

    // <details> has role="group" — query it via accessible role
    const details = (await screen.findByRole("group")) as HTMLDetailsElement
    expect(details.open).toBe(false)
  })

  it("toggles open/closed when summary is clicked", async () => {
    renderWithRouter(
      <AgendaGroup
        weekLabel="2026 W16"
        plates={[mockPlateW16a]}
        defaultOpen={true}
      />
    )

    const details = (await screen.findByRole("group")) as HTMLDetailsElement
    expect(details.open).toBe(true)

    // Click the summary to collapse
    const summary = details.querySelector("summary")!
    await userEvent.click(summary)
    expect(details.open).toBe(false)

    // Click again to expand
    await userEvent.click(summary)
    expect(details.open).toBe(true)
  })

  it("renders copy button with correct testid when showCopyButton=true", async () => {
    renderWithRouter(
      <AgendaGroup
        weekLabel="2026 W16"
        plates={[mockPlateW16a]}
        defaultOpen={true}
        showCopyButton={true}
      />
    )

    const btn = await screen.findByTestId(
      `copy-to-current-agenda-${mockPlateW16a.week_id}`
    )
    expect(btn).toBeInTheDocument()
  })

  it("does not render copy button when showCopyButton=false (default)", async () => {
    renderWithRouter(
      <AgendaGroup
        weekLabel="2026 W16"
        plates={[mockPlateW16a]}
        defaultOpen={true}
      />
    )

    await screen.findAllByRole("listitem")
    expect(
      screen.queryByTestId(`copy-to-current-agenda-${mockPlateW16a.week_id}`)
    ).not.toBeInTheDocument()
  })
})
