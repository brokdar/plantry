import { Loader2 } from "lucide-react"
import { useEffect, useMemo } from "react"
import { useTranslation } from "react-i18next"

import { draftRows } from "@/components/import/draft-rows"
import type { Draft } from "@/lib/api/import"
import { useImportDraftCheck } from "@/lib/queries/import"
import { ImportSheet } from "@/components/recipes/ImportSheet"
import type { Suggestion } from "@/lib/api/discovery"
import { useAcceptSuggestion, useSuggestions } from "@/lib/queries/discovery"
import { toast } from "@/lib/toast"

/** Tag that marks a recipe found through discovery (B6 explore quota). */
const EXPLORE_TAG = "explore"

/**
 * The import review prefilled from a pending suggestion
 * (`/recipes?add=discover&suggestion=<id>`, D18): its draft without the image
 * (on accept the server copies the suggestion's own image), rows
 * matched to the leaves the annotator
 * found, and the `explore` tag. Saving goes through the normal import, then
 * accepts the suggestion with the new food id; closing changes nothing. An
 * id that is no longer pending closes with a note. With the Chef on, the
 * import check runs first (IMP-1) and its changes are in the draft; a failed
 * or refused check opens the review as it was.
 */
export function SuggestionReview({
  id,
  onClose,
}: {
  id: number | undefined
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language.startsWith("en") ? "en" : "de"
  const list = useSuggestions({ status: "pending" }, id !== undefined)
  const accept = useAcceptSuggestion()
  const suggestion = list.data?.items.find((s) => s.id === id)
  const base = useMemo<Draft | undefined>(
    () =>
      suggestion && {
        ...suggestion.draft,
        source_url: suggestion.source_url,
        image_url: "",
      },
    [suggestion]
  )
  const checked = useImportDraftCheck(base, lang)
  const gone =
    id !== undefined && list.isSuccess && !list.isFetching && !suggestion
  const failed = id !== undefined && list.isError

  useEffect(() => {
    if (!gone && !failed) return
    toast.error(t(gone ? "discovery.review.gone" : "discovery.review.failed"))
    onClose()
  }, [gone, failed, onClose, t])

  if (!suggestion || !base) return null
  if (checked.isPending)
    return (
      <div
        role="status"
        className="fixed inset-x-0 bottom-4 z-50 mx-auto flex w-fit items-center gap-2 rounded-md border border-line bg-panel px-4 py-2 text-sm shadow-lg"
      >
        <Loader2 className="size-4 animate-spin" aria-hidden />
        {t("discovery.review.checking")}
      </div>
    )
  const draft = checked.data?.draft ?? base
  return (
    <ImportSheet
      key={suggestion.id}
      open
      onClose={onClose}
      initialDraft={draft}
      initialRows={suggestionRows(suggestion, draft)}
      initialCheck={checked.data?.check}
      extraTags={[EXPLORE_TAG]}
      onSaved={(foodId) => accept.mutate({ id: suggestion.id, foodId })}
    />
  )
}

/** The draft's rows with each annotated match preselected; unmatched rows
 *  stay for the user to resolve. */
function suggestionRows(s: Suggestion, draft: Draft) {
  return draftRows(draft).map((row, i) => {
    const line = s.ingredients[i]
    return line?.food_id
      ? {
          ...row,
          resolution: "existing" as const,
          foodId: line.food_id,
          foodName: line.candidate ?? line.name,
        }
      : row
  })
}
