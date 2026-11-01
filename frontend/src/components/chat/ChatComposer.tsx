import { Send, StopCircle } from "lucide-react"
import { useRef } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  assistantStream,
  useAssistantStreamState,
} from "@/lib/stores/assistant-stream"

interface ChatComposerProps {
  streaming: boolean
  onSubmit: (text: string) => void
  onAbort: () => void
  disabled?: boolean
}

export function ChatComposer({
  streaming,
  onSubmit,
  onAbort,
  disabled,
}: ChatComposerProps) {
  const { t } = useTranslation()
  const draft = useAssistantStreamState().draft
  const setDraft = assistantStream.setDraft
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  function send() {
    const trimmed = draft.trim()
    if (!trimmed || streaming || disabled) return
    onSubmit(trimmed)
    setDraft("")
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault()
      send()
    }
  }

  return (
    <form
      className="flex flex-col gap-2 border-t bg-background p-3"
      onSubmit={(e) => {
        e.preventDefault()
        send()
      }}
    >
      <div className="flex items-end gap-2">
        <textarea
          ref={textareaRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
          rows={2}
          disabled={disabled}
          placeholder={t("chat.composer.placeholder")}
          aria-label={t("chat.composer.aria_label")}
          data-testid="chat-composer-input"
          className="max-h-40 min-h-[2.5rem] flex-1 resize-none rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
        />
        {streaming ? (
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={onAbort}
            aria-label={t("chat.composer.abort")}
            data-testid="chat-composer-abort"
          >
            <StopCircle className="h-4 w-4" />
          </Button>
        ) : (
          <Button
            type="submit"
            size="icon"
            disabled={disabled || draft.trim() === ""}
            aria-label={t("chat.composer.send")}
            data-testid="chat-composer-submit"
          >
            <Send className="h-4 w-4" />
          </Button>
        )}
      </div>
    </form>
  )
}
