import { useMutation, useQueryClient } from "@tanstack/react-query"
import { uploadImage, deleteImage } from "@/lib/api/images"
import { ingredientKeys } from "./keys"

export const imageKeys = {
  all: ["images"] as const,
}

export function useUploadImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      ingredientId,
      file,
    }: {
      ingredientId: number
      file: File
    }) => uploadImage(ingredientId, file),
    onSuccess: (_, { ingredientId }) => {
      void qc.invalidateQueries({
        queryKey: ingredientKeys.detail(ingredientId),
      })
    },
  })
}

export function useDeleteImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ ingredientId }: { ingredientId: number }) =>
      deleteImage(ingredientId),
    onSuccess: (_, { ingredientId }) => {
      void qc.invalidateQueries({
        queryKey: ingredientKeys.detail(ingredientId),
      })
    },
  })
}
