import { useEffect, useMemo } from "react"
import { useTranslation } from "react-i18next"

import { ImportSheet } from "@/components/recipes/ImportSheet"
import type { Draft } from "@/lib/api/import"
import { useFind, useKeepFind } from "@/lib/queries/finds"
import { toast } from "@/lib/toast"

/**
 * The import review of a ready (or needs-check) find
 * (`/recipes?add=find&find=<id>`, REV-1): its draft without an image
 * (FIND-14), the normal review save, then the find is kept with the new
 * food's id. Closing changes nothing. A find that is gone or already
 * decided closes with a note.
 */
export function FindKeep({
  id,
  onClose,
}: {
  id: number | undefined
  onClose: () => void
}) {
  const { t } = useTranslation()
  const find = useFind(id)
  const keep = useKeepFind()
  const f = find.data
  const draft = useMemo<Draft | undefined>(
    () =>
      f?.draft
        ? { ...f.draft, source_url: f.source_url, image_url: "" }
        : undefined,
    [f]
  )
  const open = f?.state === "ready" || f?.state === "needs_check"
  const gone =
    id !== undefined &&
    !keep.isPending &&
    !keep.isSuccess &&
    (find.isError || (find.isSuccess && !find.isFetching && !open))

  useEffect(() => {
    if (!gone) return
    toast.error(t("discovery.find.gone"))
    onClose()
  }, [gone, onClose, t])

  if (!f || !draft || !open) return null
  return (
    <ImportSheet
      key={f.id}
      open
      onClose={onClose}
      initialDraft={draft}
      onSaved={(foodId) => keep.mutate({ id: f.id, foodId })}
    />
  )
}
