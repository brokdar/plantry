import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  deleteFoodImage,
  fetchImageFromUrl,
  uploadFoodImage,
} from "@/lib/api/images"

import { toastError } from "@/lib/toast"

import { foodKeys } from "./keys"

function invalidateFor(qc: ReturnType<typeof useQueryClient>, id: number) {
  void qc.invalidateQueries({ queryKey: foodKeys.detail(id) })
  void qc.invalidateQueries({ queryKey: foodKeys.lists() })
}

export function useUploadImage() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, file }: { id: number; file: Blob }) =>
      uploadFoodImage(id, file),
    onSuccess: (_, { id }) => invalidateFor(qc, id),
    onError: (err) => toastError(err, t),
  })
}

export function useDeleteImage() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id }: { id: number }) => deleteFoodImage(id),
    onSuccess: (_, { id }) => invalidateFor(qc, id),
    onError: (err) => toastError(err, t),
  })
}

export function useFetchImageFromUrl() {
  return useMutation({
    mutationFn: ({ url }: { url: string }) => fetchImageFromUrl(url),
  })
}
