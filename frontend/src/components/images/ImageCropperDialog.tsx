import { useCallback, useEffect, useState } from "react"
import Cropper, { type Area } from "react-easy-crop"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

interface ImageCropperDialogProps {
  open: boolean
  src: string | null
  aspect?: number
  onCancel: () => void
  onCropped: (blob: Blob) => void
}

const OUTPUT_WIDTH = 1200
const JPEG_QUALITY = 0.9

export function ImageCropperDialog({
  open,
  src,
  aspect = 4 / 3,
  onCancel,
  onCropped,
}: ImageCropperDialogProps) {
  const { t } = useTranslation()
  const [crop, setCrop] = useState({ x: 0, y: 0 })
  const [zoom, setZoom] = useState(1)
  const [croppedPixels, setCroppedPixels] = useState<Area | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (open) {
      setCrop({ x: 0, y: 0 })
      setZoom(1)
      setCroppedPixels(null)
      setBusy(false)
    }
  }, [open, src])

  const onCropComplete = useCallback(
    (_: Area, pixels: Area) => setCroppedPixels(pixels),
    []
  )

  async function handleApply() {
    if (!src || !croppedPixels) return
    setBusy(true)
    try {
      const blob = await renderCroppedImage(src, croppedPixels, aspect)
      onCropped(blob)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onCancel()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("image.crop.title")}</DialogTitle>
        </DialogHeader>
        <div className="relative h-80 w-full overflow-hidden rounded-md bg-black/80">
          {src && (
            <Cropper
              image={src}
              crop={crop}
              zoom={zoom}
              aspect={aspect}
              onCropChange={setCrop}
              onZoomChange={setZoom}
              onCropComplete={onCropComplete}
            />
          )}
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm text-muted-foreground">
            {t("image.crop.zoom")}
          </span>
          <input
            type="range"
            min={1}
            max={5}
            step={0.01}
            value={zoom}
            onChange={(e) => setZoom(Number(e.target.value))}
            className="flex-1"
            aria-label={t("image.crop.zoom")}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("image.crop.cancel")}
          </Button>
          <Button
            type="button"
            onClick={handleApply}
            disabled={busy || !croppedPixels}
          >
            {t("image.crop.apply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

async function renderCroppedImage(
  src: string,
  area: Area,
  aspect: number
): Promise<Blob> {
  const image = await loadImage(src)
  const outW = OUTPUT_WIDTH
  const outH = Math.round(outW / aspect)

  const canvas = document.createElement("canvas")
  canvas.width = outW
  canvas.height = outH
  const ctx = canvas.getContext("2d")
  if (!ctx) throw new Error("canvas 2d unavailable")

  ctx.drawImage(
    image,
    area.x,
    area.y,
    area.width,
    area.height,
    0,
    0,
    outW,
    outH
  )

  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error("toBlob failed"))),
      "image/jpeg",
      JPEG_QUALITY
    )
  })
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.crossOrigin = "anonymous"
    img.onload = () => resolve(img)
    img.onerror = reject
    img.src = src
  })
}
