import { History, Trash2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  useConversations,
  useConversationTitles,
  useDeleteConversation,
} from "@/lib/queries/ai"
import {
  assistantStream,
  useAssistantStreamState,
} from "@/lib/stores/assistant-stream"

export function ConversationHistory() {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const { data } = useConversations()
  const deleteMutation = useDeleteConversation()
  const activeId = useAssistantStreamState().conversationId
  const setActive = assistantStream.setConversation

  const items = data?.items ?? []
  const titles = useConversationTitles(items.slice(0, 20).map((c) => c.id))

  function handleSelect(id: number) {
    // Another conversation's unlock card, lines and failures do not carry over.
    if (id !== activeId) assistantStream.reset()
    setActive(id)
    setOpen(false)
  }

  function handleDelete(id: number) {
    deleteMutation.mutate(id, {
      onSuccess: () => {
        if (activeId === id) setActive(null)
      },
    })
  }

  const formatter = new Intl.DateTimeFormat(i18n.language, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  })

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          data-testid="assistant-history-trigger"
        >
          <History className="mr-1.5 h-4 w-4" />
          {t("assistant.history.open")}
          {items.length > 0 && (
            <span className="ml-1.5 rounded-full bg-muted px-1.5 text-xs">
              {items.length}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80">
        <PopoverTitle>{t("assistant.history.title")}</PopoverTitle>
        {items.length === 0 ? (
          <p className="text-muted-foreground">
            {t("assistant.history.empty")}
          </p>
        ) : (
          <ul className="flex max-h-72 flex-col gap-1 overflow-y-auto">
            {items.map((c) => {
              const active = c.id === activeId
              const time = formatter.format(new Date(c.created_at))
              const label =
                c.title && c.title.trim().length > 0
                  ? c.title
                  : (titles.get(c.id) ?? t("assistant.history.untitled"))
              return (
                <li
                  key={c.id}
                  className="flex items-center gap-1"
                  data-testid={`assistant-history-item-${c.id}`}
                >
                  <Button
                    type="button"
                    variant={active ? "secondary" : "ghost"}
                    size="sm"
                    className="h-auto min-h-11 flex-1 flex-col items-start gap-0 py-1.5 text-left md:pointer-fine:min-h-9"
                    aria-label={`${t("assistant.history.select")}: ${label}, ${time}`}
                    onClick={() => handleSelect(c.id)}
                  >
                    <span className="w-full truncate">{label}</span>
                    <span className="text-caption font-normal text-ink-muted">
                      {time}
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 shrink-0 text-muted-foreground"
                    aria-label={t("assistant.history.delete")}
                    onClick={() => handleDelete(c.id)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  )
}
