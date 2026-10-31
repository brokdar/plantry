import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  addLeftovers,
  deleteSession,
  getLeftoverCells,
  getSession,
  setSessionCooked,
  updateSession,
  type Cell,
  type UpdateSessionInput,
} from "@/lib/api/sessions"
import { toastError } from "@/lib/toast"

import { sessionKeys } from "./keys"
import { invalidatePlanViews, useUndoableRemoval } from "./plan"

function invalidateSession(qc: QueryClient, id: number) {
  return Promise.all([
    invalidatePlanViews(qc),
    qc.invalidateQueries({ queryKey: sessionKeys.detail(id) }),
  ])
}

export function useSession(id: number) {
  return useQuery({
    queryKey: sessionKeys.detail(id),
    queryFn: () => getSession(id),
    enabled: id > 0,
  })
}

/** The next `count` eligible leftover cells, to preselect in the sheet. */
export function useLeftoverCells(id: number, count: number) {
  return useQuery({
    queryKey: sessionKeys.leftoverCells(id, count),
    queryFn: () => getLeftoverCells(id, count),
    enabled: id > 0 && count > 0,
  })
}

/** Portions, frozen portions and note (a full replace). Updates run one at
 *  a time in call order (one mutation scope), so the last edit wins. */
export function useUpdateSession() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    scope: { id: "session-update" },
    mutationFn: ({ id, input }: { id: number; input: UpdateSessionInput }) =>
      updateSession(id, input),
    onSuccess: (_data, { id }) => invalidateSession(qc, id),
    onError: (err) => toastError(err, t),
  })
}

export function useSetSessionCooked() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, cooked }: { id: number; cooked: boolean }) =>
      setSessionCooked(id, cooked),
    onSuccess: (_data, { id }) => invalidateSession(qc, id),
    onError: (err) => toastError(err, t),
  })
}

export function useAddLeftovers() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: ({ id, cells }: { id: number; cells: Cell[] }) =>
      addLeftovers(id, cells),
    onSuccess: (_data, { id }) => invalidateSession(qc, id),
    onError: (err) => toastError(err, t),
  })
}

/** "Remove batch", with Undo. */
export function useDeleteSession() {
  const qc = useQueryClient()
  const { t } = useTranslation()
  const offerUndo = useUndoableRemoval()
  return useMutation({
    mutationFn: (id: number) => deleteSession(id),
    onSuccess: (res, id) => {
      void invalidateSession(qc, id)
      offerUndo(res, "batch_removed")
    },
    onError: (err) => toastError(err, t),
  })
}
