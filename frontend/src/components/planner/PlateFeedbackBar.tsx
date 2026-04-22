import { Check, Heart, StickyNote, ThumbsDown, X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Textarea } from "@/components/ui/textarea"
import type { PlateFeedbackStatus } from "@/lib/api/feedback"
import type { Plate } from "@/lib/api/plates"
import { useClearFeedback, useRecordFeedback } from "@/lib/queries/feedback"
import { cn } from "@/lib/utils"

interface PlateFeedbackBarProps {
  plate: Plate
  weekId: number
}

const STATUSES: {
  key: PlateFeedbackStatus
  Icon: typeof Check
  translationKey: string
}[] = [
  { key: "cooked", Icon: Check, translationKey: "plate.feedback.cooked" },
  { key: "skipped", Icon: X, translationKey: "plate.feedback.skipped" },
  { key: "loved", Icon: Heart, translationKey: "plate.feedback.loved" },
  {
    key: "disliked",
    Icon: ThumbsDown,
    translationKey: "plate.feedback.disliked",
  },
]

export function PlateFeedbackBar({ plate, weekId }: PlateFeedbackBarProps) {
  const { t } = useTranslation()
  const record = useRecordFeedback(weekId)
  const clear = useClearFeedback(weekId)
  const current = plate.feedback?.status
  const [noteDraft, setNoteDraft] = useState(plate.feedback?.note ?? "")
  const [noteOpen, setNoteOpen] = useState(false)

  function handleClick(status: PlateFeedbackStatus) {
    if (status === current) {
      clear.mutate(plate.id)
      return
    }
    record.mutate({
      plateId: plate.id,
      input: { status, note: plate.feedback?.note ?? null },
    })
  }

  function handleSaveNote() {
    if (!current) return
    record.mutate({
      plateId: plate.id,
      input: { status: current, note: noteDraft.trim() || null },
    })
    setNoteOpen(false)
  }

  return (
    <div
      className="flex items-center gap-0.5 border-t border-border/60 pt-1"
      data-testid={`plate-feedback-${plate.id}`}
      aria-label={t("plate.feedback.label")}
    >
      {STATUSES.map(({ key, Icon, translationKey }) => {
        const active = current === key
        return (
          <Button
            key={key}
            type="button"
            variant={active ? "default" : "ghost"}
            size="icon"
            className={cn("h-6 w-6", active && "ring-1 ring-primary")}
            aria-label={t(translationKey)}
            aria-pressed={active}
            data-active={active ? "true" : undefined}
            onClick={() => handleClick(key)}
          >
            <Icon className="h-3 w-3" />
          </Button>
        )
      })}
      <Popover
        open={noteOpen}
        onOpenChange={(open) => {
          if (open) setNoteDraft(plate.feedback?.note ?? "")
          setNoteOpen(open)
        }}
      >
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant={plate.feedback?.note ? "secondary" : "ghost"}
            size="icon"
            className="ml-auto h-6 w-6"
            aria-label={t("plate.feedback.add_note")}
            disabled={!current}
          >
            <StickyNote className="h-3 w-3" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-64">
          <Textarea
            value={noteDraft}
            onChange={(e) => setNoteDraft(e.target.value)}
            placeholder={t("plate.feedback.note_placeholder")}
            className="min-h-20 text-sm"
            aria-label={t("plate.feedback.add_note")}
          />
          <Button
            type="button"
            size="sm"
            onClick={handleSaveNote}
            className="self-end"
          >
            {t("plate.feedback.save_note")}
          </Button>
        </PopoverContent>
      </Popover>
    </div>
  )
}
