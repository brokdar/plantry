import { QueryClient } from "@tanstack/react-query"

import { ApiError } from "@/lib/api/client"

/** Retries network errors and 5xx up to 3 times; a 4xx won't change on
 *  retry, so it fails at once. */
export function retryTransient(failureCount: number, error: Error): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
    return false
  }
  return failureCount < 3
}

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: retryTransient,
    },
  },
})
