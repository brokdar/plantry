import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { BarcodeDetector, type BarcodeFormat } from "barcode-detector/pure"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"

type ScanError = "permission_denied" | "no_camera" | "unavailable"

interface BarcodeScannerModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onScan: (barcode: string) => void
}

const FORMATS: BarcodeFormat[] = [
  "ean_13",
  "upc_a",
  "upc_e",
  "ean_8",
  "code_128",
]

export function BarcodeScannerModal({
  open,
  onOpenChange,
  onScan,
}: BarcodeScannerModalProps) {
  const { t } = useTranslation()
  const [requesting, setRequesting] = useState(false)
  const [error, setError] = useState<ScanError | null>(null)
  const [fallbackValue, setFallbackValue] = useState("")

  const videoRef = useRef<HTMLVideoElement>(null)
  const streamRef = useRef<MediaStream | null>(null)
  const rafRef = useRef<number>(0)
  const detectorRef = useRef<BarcodeDetector | null>(null)

  function stopCamera() {
    cancelAnimationFrame(rafRef.current)
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
  }

  function detectFrame() {
    const video = videoRef.current
    const detector = detectorRef.current
    if (!video || !detector) {
      rafRef.current = requestAnimationFrame(detectFrame)
      return
    }
    detector
      .detect(video)
      .then((barcodes) => {
        if (barcodes.length > 0) {
          stopCamera()
          onScan(barcodes[0].rawValue)
          onOpenChange(false)
        } else {
          rafRef.current = requestAnimationFrame(detectFrame)
        }
      })
      .catch(() => {
        rafRef.current = requestAnimationFrame(detectFrame)
      })
  }

  useEffect(() => {
    if (!open) {
      stopCamera()
      setRequesting(false)
      setError(null)
      setFallbackValue("")
      return
    }

    if (!navigator.mediaDevices?.getUserMedia) {
      setError("no_camera")
      return
    }

    let cancelled = false

    async function startCamera() {
      setRequesting(true)
      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: "environment" },
        })
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop())
          return
        }
        streamRef.current = stream
        const video = videoRef.current
        if (!video) {
          stream.getTracks().forEach((t) => t.stop())
          streamRef.current = null
          setRequesting(false)
          return
        }
        try {
          video.srcObject = stream as unknown as MediaStream
          await video.play()
        } catch {
          // srcObject/play are display-only — detection still works without them
        }
        setRequesting(false)

        detectorRef.current = new BarcodeDetector({ formats: FORMATS })
        detectFrame()
      } catch (err) {
        if (cancelled) return

        console.error("[BarcodeScannerModal] startCamera caught:", err)
        setRequesting(false)
        const name = err instanceof Error ? err.name : ""
        setError(name === "NotAllowedError" ? "permission_denied" : "no_camera")
      }
    }

    startCamera()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  function handleFallbackSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = fallbackValue.trim()
    if (!trimmed) return
    stopCamera()
    onScan(trimmed)
    onOpenChange(false)
  }

  function errorMessage(): string {
    switch (error) {
      case "permission_denied":
        return t("lookup.scanner_error_permission")
      case "no_camera":
        return t("lookup.scanner_error_no_camera")
      case "unavailable":
        return t("lookup.scanner_error_unavailable")
    }
    return ""
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm overflow-hidden p-0">
        <DialogHeader className="px-6 pt-6">
          <DialogTitle>
            {error
              ? t("lookup.scanner_fallback_title")
              : t("lookup.scan_barcode")}
          </DialogTitle>
        </DialogHeader>

        {!error && (
          <div className="relative bg-black">
            {requesting && (
              <div className="flex h-48 items-center justify-center">
                <p className="text-sm text-white/70">
                  {t("lookup.scanner_requesting")}
                </p>
              </div>
            )}
            <video
              ref={videoRef}
              className={`w-full object-cover ${requesting ? "hidden" : "aspect-video"}`}
              muted
              playsInline
            />
            {!requesting && (
              <p className="absolute right-0 bottom-2 left-0 text-center text-xs text-white/70">
                {t("lookup.scanner_hint")}
              </p>
            )}
          </div>
        )}

        {error && (
          <div className="space-y-4 px-6 pb-6">
            <p className="text-sm text-destructive">{errorMessage()}</p>
            <form onSubmit={handleFallbackSubmit} className="space-y-3">
              <Input
                value={fallbackValue}
                onChange={(e) => setFallbackValue(e.target.value)}
                placeholder="0123456789012"
                autoFocus
              />
              <div className="flex gap-2">
                <Button type="submit" disabled={!fallbackValue.trim()}>
                  {t("lookup.scan_barcode")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => onOpenChange(false)}
                >
                  {t("common.cancel")}
                </Button>
              </div>
            </form>
          </div>
        )}

        {!error && (
          <div className="px-6 pt-4 pb-6">
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
