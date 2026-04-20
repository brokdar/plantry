import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import {
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

interface ImageFieldProps {
  entityType: ImageEntityType
  entityId: number
  currentImagePath: string | null
  onImageChange: (path: string | null) => void
  aspect?: number
}

export function ImageField({
  entityType,
  entityId,
  currentImagePath,
  onImageChange,
  aspect = 4 / 3,
}: ImageFieldProps) {
  const { t } = useTranslation()
  const uploadMutation = useUploadImage()
  const deleteMutation = useDeleteImage()
  const fetchUrlMutation = useFetchImageFromUrl()

  const [cropSrc, setCropSrc] = useState<string | null>(null)
  const [urlInput, setUrlInput] = useState("")
  const objectUrlRef = useRef<string | null>(null)
  const rootRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    return () => {
      if (objectUrlRef.current) URL.revokeObjectURL(objectUrlRef.current)
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
      openCropper(file)
    }
    document.addEventListener("paste", onPaste)
    return () => document.removeEventListener("paste", onPaste)
  }, [])

  function openCropper(blob: Blob) {
    if (objectUrlRef.current) URL.revokeObjectURL(objectUrlRef.current)
    const url = URL.createObjectURL(blob)
    objectUrlRef.current = url
    setCropSrc(url)
  }

  function closeCropper() {
    setCropSrc(null)
    if (objectUrlRef.current) {
      URL.revokeObjectURL(objectUrlRef.current)
      objectUrlRef.current = null
    }
  }

  function handleFileSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (file) openCropper(file)
  }

  async function handleFetchUrl() {
    const url = urlInput.trim()
    if (!url) return
    try {
      const blob = await fetchUrlMutation.mutateAsync({ url })
      setUrlInput("")
      openCropper(blob)
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleCropped(blob: Blob) {
    closeCropper()
    try {
      const result = await uploadMutation.mutateAsync({
        entityType,
        id: entityId,
        file: blob,
      })
      onImageChange(result.image_path)
    } catch (err) {
      toastError(err, t)
    }
  }

  async function handleDelete() {
    try {
      await deleteMutation.mutateAsync({ entityType, id: entityId })
      onImageChange(null)
    } catch (err) {
      toastError(err, t)
    }
  }

  const isBusy =
    uploadMutation.isPending ||
    deleteMutation.isPending ||
    fetchUrlMutation.isPending

  return (
    <div ref={rootRef} className="space-y-3" tabIndex={-1}>
      {currentImagePath && (
        <div
          className="w-64 overflow-hidden rounded-md bg-surface-container-high"
          style={{ aspectRatio: aspect }}
        >
          <img
            src={imageURL(currentImagePath, Date.now())}
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

        {currentImagePath && (
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
