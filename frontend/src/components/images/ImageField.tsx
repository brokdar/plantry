import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import {
  Camera,
  Clipboard,
  Link as LinkIcon,
  Loader2,
  Trash2,
  Upload,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { ImageEntityType } from "@/lib/api/images"
import { imageURL } from "@/lib/image-url"
import {
  useDeleteImage,
  useFetchImageFromUrl,
  useUploadImage,
} from "@/lib/queries/images"
import { toastError } from "@/lib/toast"

import { ImageCropperDialog } from "./ImageCropperDialog"

type BoundProps = {
  mode: "bound"
  entityType: ImageEntityType
  entityId: number
  currentImagePath: string | null
  onImageChange: (path: string | null) => void
  aspect?: number
}

type StagedProps = {
  mode: "staged"
  stagedBlob: Blob | null
  onStagedChange: (blob: Blob | null) => void
  aspect?: number
}

export type ImageFieldProps = BoundProps | StagedProps

export function ImageField(props: ImageFieldProps) {
  const { t } = useTranslation()
  const aspect = props.aspect ?? 4 / 3
  const isStaged = props.mode === "staged"

  const uploadMutation = useUploadImage()
  const deleteMutation = useDeleteImage()
  const fetchUrlMutation = useFetchImageFromUrl()

  const [cropSrc, setCropSrc] = useState<string | null>(null)
  const [urlInput, setUrlInput] = useState("")
  // Cache-bust the preview whenever we write a new image for the same path.
  // Seeded lazily so initial render stays pure; bumped on mutate success.
  const [imgVersion, setImgVersion] = useState(0)
  const cropObjectUrlRef = useRef<string | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)

  const stagedBlob = isStaged ? (props as StagedProps).stagedBlob : null
  const stagedPreviewUrl = useMemo(
    () => (stagedBlob ? URL.createObjectURL(stagedBlob) : null),
    [stagedBlob]
  )

  function openCropper(blob: Blob) {
    if (cropObjectUrlRef.current) URL.revokeObjectURL(cropObjectUrlRef.current)
    const url = URL.createObjectURL(blob)
    cropObjectUrlRef.current = url
    setCropSrc(url)
  }

  async function startCropFlow(blob: Blob) {
    const url = URL.createObjectURL(blob)
    try {
      const img = new Image()
      const loaded = await new Promise<boolean>((resolve) => {
        img.onload = () => resolve(true)
        img.onerror = () => resolve(false)
        img.src = url
      })
      if (loaded && img.naturalWidth > 0 && img.naturalHeight > 0) {
        const natural = img.naturalWidth / img.naturalHeight
        if (Math.abs(natural - aspect) < 0.02) {
          await handleCropped(blob)
          return
        }
      }
    } finally {
      URL.revokeObjectURL(url)
    }
    openCropper(blob)
  }

  function closeCropper() {
    setCropSrc(null)
    if (cropObjectUrlRef.current) {
      URL.revokeObjectURL(cropObjectUrlRef.current)
      cropObjectUrlRef.current = null
    }
  }

  useEffect(() => {
    if (!stagedPreviewUrl) return
    return () => URL.revokeObjectURL(stagedPreviewUrl)
  }, [stagedPreviewUrl])

  useEffect(() => {
    return () => {
      if (cropObjectUrlRef.current)
        URL.revokeObjectURL(cropObjectUrlRef.current)
    }
  }, [])

  useEffect(() => {
    function onPaste(e: ClipboardEvent) {
      if (
        !rootRef.current?.contains(document.activeElement) &&
        document.activeElement !== document.body
      ) {
        return
      }
      const items = Array.from(e.clipboardData?.items ?? [])
      const image = items.find((it) => it.type.startsWith("image/"))
      if (!image) return
      const file = image.getAsFile()
      if (!file) return
      e.preventDefault()
      void startCropFlow(file)
    }
    document.addEventListener("paste", onPaste)
    return () => document.removeEventListener("paste", onPaste)
  }, [])

  function handleFileSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (file) void startCropFlow(file)
  }

  async function handleFetchUrl() {
    const url = urlInput.trim()
    if (!url) return
    try {
      const blob = await fetchUrlMutation.mutateAsync({ url })
      setUrlInput("")
      await startCropFlow(blob)
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleCropped(blob: Blob) {
    closeCropper()
    if (isStaged) {
      ;(props as StagedProps).onStagedChange(blob)
      return
    }
    const bound = props as BoundProps
    try {
      const result = await uploadMutation.mutateAsync({
        entityType: bound.entityType,
        id: bound.entityId,
        file: blob,
      })
      bound.onImageChange(result.image_path)
      setImgVersion(Date.now())
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleDelete() {
    if (isStaged) {
      ;(props as StagedProps).onStagedChange(null)
      return
    }
    const bound = props as BoundProps
    try {
      await deleteMutation.mutateAsync({
        entityType: bound.entityType,
        id: bound.entityId,
      })
      bound.onImageChange(null)
      setImgVersion(Date.now())
    } catch (err) {
      toastError(err, t)
    }
  }

  const boundPath = !isStaged ? (props as BoundProps).currentImagePath : null
  const previewUrl = isStaged
    ? stagedPreviewUrl
    : boundPath
      ? imageURL(boundPath, imgVersion || undefined)
      : null

  const hasImage = previewUrl !== null

  const isBusy =
    uploadMutation.isPending ||
    deleteMutation.isPending ||
    fetchUrlMutation.isPending

  return (
    <div ref={rootRef} className="space-y-3" tabIndex={-1}>
      {hasImage && (
        <div
          className="w-64 overflow-hidden rounded-md bg-surface-container-high"
          style={{ aspectRatio: aspect }}
        >
          <img
            src={previewUrl!}
            alt=""
            className="h-full w-full object-cover"
          />
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isBusy}
          asChild
        >
          <label className="cursor-pointer">
            {uploadMutation.isPending ? (
              <Loader2 className="mr-2 size-4 animate-spin" />
            ) : (
              <Upload className="mr-2 size-4" />
            )}
            {t("image.upload")}
            <input
              type="file"
              accept="image/*"
              className="hidden"
              onChange={handleFileSelect}
              disabled={isBusy}
            />
          </label>
        </Button>

        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isBusy}
          className="md:hidden"
          asChild
        >
          <label className="cursor-pointer">
            <Camera className="mr-2 size-4" />
            {t("image.take_photo")}
            <input
              type="file"
              accept="image/*"
              capture="environment"
              className="hidden"
              onChange={handleFileSelect}
              disabled={isBusy}
            />
          </label>
        </Button>

        {hasImage && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={isBusy}
            onClick={handleDelete}
          >
            <Trash2 className="mr-2 size-4" />
            {t("image.remove")}
          </Button>
        )}
      </div>

      <div className="flex items-center gap-2">
        <LinkIcon className="size-4 text-muted-foreground" />
        <Input
          value={urlInput}
          onChange={(e) => setUrlInput(e.target.value)}
          placeholder={t("image.url_placeholder")}
          disabled={isBusy}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              void handleFetchUrl()
            }
          }}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isBusy || !urlInput.trim()}
          onClick={handleFetchUrl}
        >
          {fetchUrlMutation.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            t("image.fetch")
          )}
        </Button>
      </div>

      <p className="flex items-center gap-1 text-xs text-muted-foreground">
        <Clipboard className="size-3" />
        {t("image.paste_hint")}
      </p>

      <ImageCropperDialog
        open={cropSrc !== null}
        src={cropSrc}
        aspect={aspect}
        onCancel={closeCropper}
        onCropped={(blob) => void handleCropped(blob)}
      />
    </div>
  )
}
