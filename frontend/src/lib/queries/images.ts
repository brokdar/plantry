import { useMutation, useQueryClient } from "@tanstack/react-query"

import {
  deleteImage,
  fetchImageFromUrl,
  uploadImage,
  type ImageEntityType,
} from "@/lib/api/images"

import { componentKeys, ingredientKeys } from "./keys"

function invalidateFor(
  qc: ReturnType<typeof useQueryClient>,
  entityType: ImageEntityType,
  id: number
) {
  const keys =
    entityType === "ingredients"
      ? [ingredientKeys.detail(id), ingredientKeys.lists()]
      : [componentKeys.detail(id), componentKeys.lists()]
  keys.forEach((k) => {
    void qc.invalidateQueries({ queryKey: k })
  })
}

export function useUploadImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      entityType,
      id,
      file,
    }: {
      entityType: ImageEntityType
      id: number
      file: Blob
    }) => uploadImage(entityType, id, file),
    onSuccess: (_, { entityType, id }) => invalidateFor(qc, entityType, id),
  })
}

export function useDeleteImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      entityType,
      id,
    }: {
      entityType: ImageEntityType
      id: number
    }) => deleteImage(entityType, id),
    onSuccess: (_, { entityType, id }) => invalidateFor(qc, entityType, id),
  })
}

export function useFetchImageFromUrl() {
  return useMutation({
    mutationFn: ({ url }: { url: string }) => fetchImageFromUrl(url),
  })
}
