import { render, screen, fireEvent, waitFor, act } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { vi, describe, it, expect, beforeEach, afterEach } from "vitest"
import "@/lib/i18n"
import { BarcodeScannerModal } from "./BarcodeScannerModal"

// Render Dialog synchronously — Radix Portal delays mount via animation state,
// which leaves videoRef.current null when the camera effect fires in jsdom.
vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <div data-slot="dialog-content">{children}</div>
  ),
  DialogHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  DialogTitle: ({ children }: { children: React.ReactNode }) => (
    <h2>{children}</h2>
  ),
}))

const mockDetect = vi.fn()

vi.mock("barcode-detector/pure", () => ({
  BarcodeDetector: vi.fn().mockImplementation(function () {
    return { detect: mockDetect }
  }),
}))

function makeMockStream() {
  const track = { stop: vi.fn() }
  return {
    getTracks: () => [track],
    _track: track,
  } as unknown as MediaStream & { _track: { stop: ReturnType<typeof vi.fn> } }
}

function setMediaDevices(gum: ReturnType<typeof vi.fn> | undefined) {
  Object.defineProperty(navigator, "mediaDevices", {
    value: gum !== undefined ? { getUserMedia: gum } : undefined,
    configurable: true,
    writable: true,
  })
}

// Capture original play so we can restore it manually without vi.restoreAllMocks().
// vi.restoreAllMocks() also resets vi.fn() implementations (like BarcodeDetector mock),
// breaking tests that run after it.
const originalPlay = HTMLMediaElement.prototype.play

describe("BarcodeScannerModal", () => {
  beforeEach(() => {
    mockDetect.mockResolvedValue([])
    // Replace directly — no spyOn so no restoreAllMocks needed
    HTMLMediaElement.prototype.play = vi.fn().mockImplementation(function () {
      return Promise.resolve(undefined)
    }) as unknown as typeof HTMLMediaElement.prototype.play
  })

  afterEach(() => {
    HTMLMediaElement.prototype.play = originalPlay
    vi.clearAllMocks()
  })

  it("does not call getUserMedia when open=false", () => {
    const gum = vi.fn()
    setMediaDevices(gum)
    render(
      <BarcodeScannerModal
        open={false}
        onOpenChange={vi.fn()}
        onScan={vi.fn()}
      />
    )
    expect(gum).not.toHaveBeenCalled()
  })

  it("shows requesting state while getUserMedia is pending", async () => {
    setMediaDevices(vi.fn().mockReturnValue(new Promise(() => {})))
    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={vi.fn()}
        onScan={vi.fn()}
      />
    )
    expect(
      await screen.findByText("Requesting camera access…")
    ).toBeInTheDocument()
  })

  it("camera granted: no error shown after getUserMedia resolves", async () => {
    const gum = vi.fn().mockResolvedValue(makeMockStream())
    setMediaDevices(gum)
    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={vi.fn()}
        onScan={vi.fn()}
      />
    )
    await waitFor(() => expect(gum).toHaveBeenCalled())
    await waitFor(() =>
      expect(screen.queryByText("No camera available")).toBeNull()
    )
    expect(screen.queryByText("Camera access denied")).toBeNull()
  })

  it("calls onScan and onOpenChange when a barcode is detected", async () => {
    const onScan = vi.fn()
    const onOpenChange = vi.fn()
    setMediaDevices(vi.fn().mockResolvedValue(makeMockStream()))
    mockDetect.mockResolvedValue([{ rawValue: "5901234123457" }])

    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={onOpenChange}
        onScan={onScan}
      />
    )

    await waitFor(() => expect(onScan).toHaveBeenCalledWith("5901234123457"))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("shows permission denied error + fallback input on NotAllowedError", async () => {
    setMediaDevices(
      vi
        .fn()
        .mockRejectedValue(
          Object.assign(new Error(), { name: "NotAllowedError" })
        )
    )
    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={vi.fn()}
        onScan={vi.fn()}
      />
    )
    expect(await screen.findByText("Camera access denied")).toBeInTheDocument()
    expect(screen.getByPlaceholderText("0123456789012")).toBeInTheDocument()
  })

  it("shows no camera error when mediaDevices is undefined", async () => {
    setMediaDevices(undefined)
    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={vi.fn()}
        onScan={vi.fn()}
      />
    )
    expect(await screen.findByText("No camera available")).toBeInTheDocument()
    expect(screen.getByPlaceholderText("0123456789012")).toBeInTheDocument()
  })

  it("shows no camera error on generic getUserMedia failure", async () => {
    setMediaDevices(vi.fn().mockRejectedValue(new Error("not found")))
    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={vi.fn()}
        onScan={vi.fn()}
      />
    )
    expect(await screen.findByText("No camera available")).toBeInTheDocument()
  })

  it("fallback: submit calls onScan with trimmed value and closes", async () => {
    setMediaDevices(undefined)
    const onScan = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={onOpenChange}
        onScan={onScan}
      />
    )
    const input = await screen.findByPlaceholderText("0123456789012")
    await userEvent.type(input, "  1234567890123  ")
    fireEvent.submit(input.closest("form")!)
    expect(onScan).toHaveBeenCalledWith("1234567890123")
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("fallback: submit button disabled when input is empty", async () => {
    setMediaDevices(undefined)
    render(
      <BarcodeScannerModal
        open={true}
        onOpenChange={vi.fn()}
        onScan={vi.fn()}
      />
    )
    const btn = await screen.findByRole("button", { name: /scan barcode/i })
    expect(btn).toBeDisabled()
  })

  it("stops camera stream when modal closes", async () => {
    const stream = makeMockStream()
    const gum = vi.fn().mockResolvedValue(stream)
    setMediaDevices(gum)

    function Wrapper() {
      const [open, setOpen] = useState(true)
      return (
        <>
          <button onClick={() => setOpen(false)}>close</button>
          <BarcodeScannerModal
            open={open}
            onOpenChange={setOpen}
            onScan={vi.fn()}
          />
        </>
      )
    }

    render(<Wrapper />)
    // Wait until getUserMedia resolved (stream is active)
    await waitFor(() => expect(gum).toHaveBeenCalled())
    await waitFor(() =>
      expect(screen.queryByText("Requesting camera access…")).toBeNull()
    )

    act(() => {
      fireEvent.click(screen.getByText("close"))
    })

    expect(stream._track.stop).toHaveBeenCalled()
  })
})
