import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  getProfile,
  profileToInput,
  updateProfile,
  type Profile,
  type ProfileInput,
} from "@/lib/api/profile"
import { toastError } from "@/lib/toast"

import { profileKeys } from "./keys"

export function useProfile() {
  return useQuery({
    queryKey: profileKeys.detail,
    queryFn: getProfile,
  })
}

const PATCH_PROFILE_KEY = ["profile", "patch"] as const

/**
 * Saves some profile fields. `PUT /api/profile` is a full replace, so the
 * patch is merged into the cached profile. The cache is updated at once (the
 * control shows the new value while the request runs), and patches run one
 * after another so a later one never carries a stale copy of an earlier one.
 * A failure toasts and refetches the stored profile.
 */
export function usePatchProfile() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationKey: PATCH_PROFILE_KEY,
    scope: { id: "profile" },
    mutationFn: (patch: ProfileInput) => {
      const current = qc.getQueryData<Profile>(profileKeys.detail)
      if (!current) return updateProfile(patch)
      return updateProfile({ ...profileToInput(current), ...patch })
    },
    // The cache write happens before the first await, so a controlled input
    // re-renders with the new value in the same event.
    onMutate: async (patch) => {
      const cancelled = qc.cancelQueries({ queryKey: profileKeys.detail })
      qc.setQueryData<Profile>(profileKeys.detail, (prev) =>
        prev ? ({ ...prev, ...patch } as Profile) : prev
      )
      await cancelled
    },
    onSuccess: (data) => {
      // A queued patch is already applied to the cache; writing this
      // response would flash its old value until the patch lands.
      if (qc.isMutating({ mutationKey: PATCH_PROFILE_KEY }) <= 1) {
        qc.setQueryData(profileKeys.detail, data)
      }
    },
    onError: (err) => {
      toastError(err, t)
      void qc.invalidateQueries({ queryKey: profileKeys.detail })
    },
  })
}
