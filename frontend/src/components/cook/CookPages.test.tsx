import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { ScaledRecipe } from "@/lib/api/foods"
import i18n from "@/lib/i18n"
import {
  mockChili,
  mockScaled,
  mockSession,
  mockTimeSlots,
} from "@/test/fixtures"
import { renderWithRouter } from "@/test/render"

vi.mock("@/lib/api/sessions", () => ({
  getSession: vi.fn(),
  updateSession: vi.fn(),
  setSessionCooked: vi.fn(),
}))
vi.mock("@/lib/api/foods", () => ({
  getFood: vi.fn(),
  getScaled: vi.fn(),
}))
vi.mock("@/lib/api/slots", () => ({ listTimeSlots: vi.fn() }))
vi.mock("@/lib/toast", () => ({ toast: vi.fn(), toastError: vi.fn() }))

import { getFood, getScaled } from "@/lib/api/foods"
import { getSession, setSessionCooked, updateSession } from "@/lib/api/sessions"
import { listTimeSlots } from "@/lib/api/slots"

import { CookRecipePage, CookSessionPage } from "./CookPages"

/** The scaled list for any portions: Chili is 400 g beans, 2 onions per 4. */
function scaledFor(_id: number, portions: number): Promise<ScaledRecipe> {
  const f = portions / 4
  return Promise.resolve({
    ...mockScaled,
    portions,
    factor: f,
    ingredients: [
      {
        ...mockScaled.ingredients[0],
        amount: 400 * f,
        display: { amount: 400 * f, unit: "g" },
      },
      {
        ...mockScaled.ingredients[1],
        amount: 2 * f,
        display: { amount: 2 * f, unit: "piece" },
        buy: { amount: Math.ceil(2 * f), unit: "piece" },
      },
    ],
  })
}

describe("cook pages", () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18n.changeLanguage("en")
    vi.mocked(getFood).mockResolvedValue({
      ...mockChili,
      instructions: [
        { id: 2, food_id: 42, step_number: 2, text: "Simmer." },
        { id: 1, food_id: 42, step_number: 1, text: "Chop." },
      ],
    })
    vi.mocked(getScaled).mockImplementation(scaledFor)
    vi.mocked(listTimeSlots).mockResolvedValue({ items: mockTimeSlots })
    vi.mocked(getSession).mockResolvedValue(mockSession)
    vi.mocked(updateSession).mockImplementation(async (_id, input) => ({
      ...mockSession,
      ...input,
    }))
    vi.mocked(setSessionCooked).mockResolvedValue({
      ...mockSession,
      cooked_at: "2026-11-01T18:30:00Z",
    })
  })

  describe("session mode", () => {
    // F6-AC1 (component level; the e2e reloads).
    it("scales to the batch and writes the new portions", async () => {
      renderWithRouter(<CookSessionPage sessionId={7} />)

      const rows = await screen.findAllByTestId("cook-ingredient")
      expect(rows[0]).toHaveTextContent("600 g")
      expect(rows[1]).toHaveTextContent("3 pcs")
      expect(screen.getByTestId("cook-coverage")).toHaveTextContent(
        "Covers Sun dinner, Mon lunch, Mon dinner"
      )

      const more = screen.getByRole("button", { name: /increase portions/i })
      await userEvent.click(more)
      await userEvent.click(more)

      expect(screen.getByTestId("cook-badge")).toHaveTextContent("Cooking × 8")
      await waitFor(() =>
        expect(screen.getAllByTestId("cook-ingredient")[0]).toHaveTextContent(
          "800 g"
        )
      )
      expect(screen.getAllByTestId("cook-ingredient")[1]).toHaveTextContent(
        "4 pcs"
      )
      expect(screen.getByTestId("cook-coverage")).toHaveTextContent("2 spare")

      // Debounced: one write for both taps.
      await waitFor(() => expect(updateSession).toHaveBeenCalledTimes(1))
      expect(updateSession).toHaveBeenCalledWith(7, {
        portions: 8,
        frozen_portions: 0,
        note: null,
      })
    })

    it("has a main landmark holding the steps", async () => {
      renderWithRouter(<CookSessionPage sessionId={7} />)
      const step = await screen.findByTestId("step-text")
      expect(screen.getByRole("main")).toContainElement(step)
    })

    it("lists the steps in order", async () => {
      renderWithRouter(<CookSessionPage sessionId={7} />)
      expect(await screen.findByTestId("step-text")).toHaveTextContent("Chop.")
    })

    // F6-AC4 (the write; e2e covers Today).
    it("Done cooking marks the batch cooked", async () => {
      renderWithRouter(<CookSessionPage sessionId={7} />)
      await userEvent.click(await screen.findByRole("button", { name: "Next" }))
      await userEvent.click(screen.getByTestId("cook-done"))
      await waitFor(() =>
        expect(setSessionCooked).toHaveBeenCalledWith(7, true)
      )
    })

    it("Done cooking within the debounce writes the pending portions first", async () => {
      let settlePut!: () => void
      vi.mocked(updateSession).mockImplementationOnce(
        (_id, input) =>
          new Promise((resolve) => {
            settlePut = () => resolve({ ...mockSession, ...input })
          })
      )
      renderWithRouter(<CookSessionPage sessionId={7} />)
      await userEvent.click(
        await screen.findByRole("button", { name: /increase portions/i })
      )
      await userEvent.click(screen.getByRole("button", { name: "Next" }))
      await userEvent.click(screen.getByTestId("cook-done"))

      // POST /cooked waits for the PUT (one mutation scope).
      await waitFor(() => expect(updateSession).toHaveBeenCalledTimes(1))
      expect(setSessionCooked).not.toHaveBeenCalled()
      settlePut()
      await waitFor(() =>
        expect(setSessionCooked).toHaveBeenCalledWith(7, true)
      )
      expect(updateSession).toHaveBeenCalledTimes(1)
      expect(updateSession).toHaveBeenCalledWith(
        7,
        expect.objectContaining({ portions: 7 })
      )
      expect(vi.mocked(updateSession).mock.invocationCallOrder[0]).toBeLessThan(
        vi.mocked(setSessionCooked).mock.invocationCallOrder[0]
      )
    })

    it("says so when the batch is gone", async () => {
      vi.mocked(getSession).mockRejectedValue(new Error("404"))
      renderWithRouter(<CookSessionPage sessionId={99} />)
      expect(
        await screen.findByText("This batch no longer exists.")
      ).toBeVisible()
    })

    it.each([0, -3, Number.NaN, 1.5])(
      "says so for the invalid id %s without fetching",
      async (id) => {
        renderWithRouter(<CookSessionPage sessionId={id} />)
        expect(
          await screen.findByText("This batch no longer exists.")
        ).toBeVisible()
        expect(getSession).not.toHaveBeenCalled()
      }
    )

    it("announces the loading skeleton", async () => {
      vi.mocked(getSession).mockReturnValue(new Promise(() => {}))
      renderWithRouter(<CookSessionPage sessionId={7} />)
      expect(
        await screen.findByRole("status", { name: "Loading the recipe" })
      ).toBeInTheDocument()
    })
  })

  describe("recipe mode", () => {
    // F6-AC5
    it("scales to ?portions without writing anything", async () => {
      renderWithRouter(<CookRecipePage id={42} portions={2} />)

      const rows = await screen.findAllByTestId("cook-ingredient")
      expect(getScaled).toHaveBeenCalledWith(42, 2)
      expect(rows[0]).toHaveTextContent("200 g")
      expect(rows[1]).toHaveTextContent("1 pcs")
      expect(screen.queryByTestId("cook-coverage")).toBeNull()

      await userEvent.click(
        screen.getByRole("button", { name: /increase portions/i })
      )
      await waitFor(() => expect(getScaled).toHaveBeenCalledWith(42, 3))
      expect(updateSession).not.toHaveBeenCalled()
      expect(setSessionCooked).not.toHaveBeenCalled()
    })

    it("defaults to the recipe's reference portions", async () => {
      renderWithRouter(<CookRecipePage id={42} />)
      expect(await screen.findByTestId("cook-badge")).toHaveTextContent(
        "Cooking × 4"
      )
      expect(getScaled).toHaveBeenCalledWith(42, 4)
    })
  })
})
