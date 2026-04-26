import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, test, vi } from "vitest"

import type { Plate } from "@/lib/api/plates"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/queries/feedback", () => ({
  useRecordFeedback: vi.fn(),
  useClearFeedback: vi.fn(),
}))

import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"

import { PlateFeedbackBar } from "./PlateFeedbackBar"

type MutationStub = {
  mutate: ReturnType<typeof vi.fn>
  isPending: boolean
}

const basePlate: Plate = {
  id: 42,
  week_id: 1,
  day: 0,
  slot_id: 1,
  date: "2024-01-01",
  note: null,
  skipped: false,
  components: [],
  created_at: "2024-01-01T00:00:00Z",
}

function stubMutations() {
  const record: MutationStub = { mutate: vi.fn(), isPending: false }
  const clear: MutationStub = { mutate: vi.fn(), isPending: false }
  vi.mocked(useRecordFeedback).mockReturnValue(
    record as unknown as ReturnType<typeof useRecordFeedback>
  )
  vi.mocked(useClearFeedback).mockReturnValue(
    clear as unknown as ReturnType<typeof useClearFeedback>
  )
  return { record, clear }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe("PlateFeedbackBar", () => {
  test("renders all four status buttons", async () => {
    stubMutations()
    renderWithRouter(<PlateFeedbackBar plate={basePlate} weekId={1} />)

    for (const label of ["Cooked", "Skipped", "Loved", "Disliked"]) {
      expect(
        await screen.findByRole("button", { name: label })
      ).toBeInTheDocument()
    }
  })

  test("clicking a status records feedback", async () => {
    const { record } = stubMutations()
    renderWithRouter(<PlateFeedbackBar plate={basePlate} weekId={1} />)

    const loved = await screen.findByRole("button", { name: "Loved" })
    await userEvent.click(loved)

    expect(record.mutate).toHaveBeenCalledWith({
      plateId: 42,
      input: { status: "loved", note: null },
    })
  })

  test("clicking the active status clears feedback", async () => {
    const { clear } = stubMutations()
    const plate: Plate = {
      ...basePlate,
      feedback: {
        plate_id: 42,
        status: "loved",
        note: null,
        rated_at: "2024-01-01T00:00:00Z",
      },
    }
    renderWithRouter(<PlateFeedbackBar plate={plate} weekId={1} />)

    const loved = await screen.findByRole("button", { name: "Loved" })
    expect(loved).toHaveAttribute("aria-pressed", "true")

    await userEvent.click(loved)
    expect(clear.mutate).toHaveBeenCalledWith(42)
  })

  test("note button is disabled until a status is set", async () => {
    stubMutations()
    renderWithRouter(<PlateFeedbackBar plate={basePlate} weekId={1} />)
    const noteBtn = await screen.findByRole("button", { name: "Add note" })
    expect(noteBtn).toBeDisabled()
  })

  test("saving a note re-records feedback with the note", async () => {
    const { record } = stubMutations()
    const plate: Plate = {
      ...basePlate,
      feedback: {
        plate_id: 42,
        status: "cooked",
        note: null,
        rated_at: "2024-01-01T00:00:00Z",
      },
    }
    renderWithRouter(<PlateFeedbackBar plate={plate} weekId={1} />)

    await userEvent.click(
      await screen.findByRole("button", { name: "Add note" })
    )
    const textarea = await screen.findByRole("textbox", { name: "Add note" })
    await userEvent.type(textarea, "extra cilantro")
    await userEvent.click(
      await screen.findByRole("button", { name: "Save note" })
    )

    expect(record.mutate).toHaveBeenCalledWith({
      plateId: 42,
      input: { status: "cooked", note: "extra cilantro" },
    })
  })
})
