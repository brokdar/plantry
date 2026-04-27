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
import { useConversations, useDeleteConversation } from "@/lib/queries/ai"
import { useChatUI } from "@/lib/stores/chat-ui"

export function ConversationHistory() {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const { data } = useConversations()
  const deleteMutation = useDeleteConversation()
  const activeId = useChatUI((s) => s.activeConversationId)
  const setActive = useChatUI((s) => s.setActiveConversation)

  const items = data?.items ?? []

  function handleSelect(id: number) {
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
          data-testid="chat-history-trigger"
        >
          <History className="mr-1.5 h-4 w-4" />
          {t("chat.history.open")}
          {items.length > 0 && (
            <span className="ml-1.5 rounded-full bg-muted px-1.5 text-xs">
              {items.length}
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80">
        <PopoverTitle>{t("chat.history.title")}</PopoverTitle>
        {items.length === 0 ? (
          <p className="text-muted-foreground">{t("chat.history.empty")}</p>
        ) : (
          <ul className="flex max-h-72 flex-col gap-1 overflow-y-auto">
            {items.map((c) => {
              const active = c.id === activeId
              const label =
                c.title && c.title.trim().length > 0
                  ? c.title
                  : formatter.format(new Date(c.created_at))
              return (
                <li
                  key={c.id}
                  className="flex items-center gap-1"
                  data-testid={`chat-history-item-${c.id}`}
                >
                  <Button
                    type="button"
                    variant={active ? "secondary" : "ghost"}
                    size="sm"
                    className="flex-1 justify-start truncate"
                    aria-label={t("chat.history.select")}
                    onClick={() => handleSelect(c.id)}
                  >
                    {label}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 shrink-0 text-muted-foreground"
                    aria-label={t("chat.history.delete")}
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
