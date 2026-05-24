import { useMutation, useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { getProfile, updateProfile, type ProfileInput } from "@/lib/api/profile"
import { queryClient } from "@/lib/query-client"
import { toastError } from "@/lib/toast"

import { profileKeys } from "./keys"

export function useProfile() {
  return useQuery({
    queryKey: profileKeys.detail,
    queryFn: getProfile,
  })
}

export function useUpdateProfile() {
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (input: ProfileInput) => updateProfile(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: profileKeys.detail })
    },
    onError: (err) => toastError(err, t),
  })
}
