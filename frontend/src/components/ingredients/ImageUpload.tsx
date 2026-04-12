import { useTranslation } from "react-i18next"
import { Upload, Trash2, Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useUploadImage, useDeleteImage } from "@/lib/queries/images"

interface ImageUploadProps {
  ingredientId: number
  currentImagePath: string | null
  onImageChange: (path: string | null) => void
}

export function ImageUpload({
  ingredientId,
  currentImagePath,
  onImageChange,
}: ImageUploadProps) {
  const { t } = useTranslation()
  const uploadMutation = useUploadImage()
  const deleteMutation = useDeleteImage()
  const isUploading = uploadMutation.isPending || deleteMutation.isPending

  function handleFileSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return

    uploadMutation.mutate(
      { ingredientId, file },
      {
        onSuccess: (result) => onImageChange(result.image_path),
      }
    )

    // Reset the input so the same file can be re-selected
    e.target.value = ""
  }

  function handleDelete() {
    deleteMutation.mutate(
      { ingredientId },
      {
        onSuccess: () => onImageChange(null),
      }
    )
  }

  return (
    <div className="space-y-3">
      {currentImagePath && (
        <img
          src={`/images/${currentImagePath}`}
          alt=""
          className="h-32 w-32 rounded-md object-cover"
        />
      )}

      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isUploading}
          asChild
        >
          <label className="cursor-pointer">
            {isUploading ? (
              <Loader2 className="mr-2 size-4 animate-spin" />
            ) : (
              <Upload className="mr-2 size-4" />
            )}
            {isUploading ? t("image.uploading") : t("image.upload")}
            <input
              type="file"
              accept="image/*"
              className="hidden"
              onChange={handleFileSelect}
              disabled={isUploading}
            />
          </label>
        </Button>

        {currentImagePath && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={isUploading}
            onClick={handleDelete}
          >
            <Trash2 className="mr-2 size-4" />
            {t("image.remove")}
          </Button>
        )}
      </div>

      {(uploadMutation.isError || deleteMutation.isError) && (
        <p className="text-sm text-destructive">
          {t("error.image.upload_failed")}
        </p>
      )}
    </div>
  )
}
