import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Upload, Trash2, Loader2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { uploadImage, deleteImage } from "@/lib/api/images"

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
  const [isUploading, setIsUploading] = useState(false)

  async function handleFileSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return

    setIsUploading(true)
    try {
      const result = await uploadImage(ingredientId, file)
      onImageChange(result.image_path)
    } finally {
      setIsUploading(false)
    }

    // Reset the input so the same file can be re-selected
    e.target.value = ""
  }

  async function handleDelete() {
    setIsUploading(true)
    try {
      await deleteImage(ingredientId)
      onImageChange(null)
    } finally {
      setIsUploading(false)
    }
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
    </div>
  )
}
