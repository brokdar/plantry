import { useMutation, useQuery } from "@tanstack/react-query"

import { getProfile, updateProfile, type ProfileInput } from "@/lib/api/profile"
import { queryClient } from "@/lib/query-client"

import { profileKeys } from "./keys"

export function useProfile() {
  return useQuery({
    queryKey: profileKeys.detail,
    queryFn: getProfile,
  })
}

export function useUpdateProfile() {
  return useMutation({
    mutationFn: (input: ProfileInput) => updateProfile(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: profileKeys.detail })
    },
  })
}
