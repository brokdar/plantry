import { Send, StopCircle } from "lucide-react"
import { useRef } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { type ChatMode, useChatUI } from "@/lib/stores/chat-ui"

interface ChatComposerProps {
  streaming: boolean
  onSubmit: (text: string, mode: ChatMode) => void
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
  const draft = useChatUI((s) => s.draftMessage)
  const setDraft = useChatUI((s) => s.setDraft)
  const mode = useChatUI((s) => s.mode)
  const setMode = useChatUI((s) => s.setMode)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  function send() {
    const trimmed = draft.trim()
    if (!trimmed || streaming || disabled) return
    onSubmit(trimmed, mode)
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
      <div className="flex items-center justify-between gap-2">
        <label htmlFor="chat-mode" className="text-xs text-muted-foreground">
          {t("chat.composer.mode_label")}
        </label>
        <select
          id="chat-mode"
          value={mode}
          onChange={(e) => setMode(e.target.value as ChatMode)}
          disabled={disabled || streaming}
          aria-label={t("chat.composer.mode_label")}
          data-testid="chat-composer-mode"
          className="h-7 rounded-md border border-input bg-background px-2 text-xs"
        >
          <option value="">{t("chat.composer.mode.default")}</option>
          <option value="fill_empty">
            {t("chat.composer.mode.fill_empty")}
          </option>
          <option value="replace_all">
            {t("chat.composer.mode.replace_all")}
          </option>
        </select>
      </div>
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
