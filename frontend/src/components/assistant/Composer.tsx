import { Send, StopCircle } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  assistantStream,
  useAssistantStreamState,
} from "@/lib/stores/assistant-stream"

interface ComposerProps {
  streaming: boolean
  onSubmit: (text: string) => void
  onAbort: () => void
  disabled?: boolean
  /** Lets the panel focus the field (a chip fills it, opening focuses it). */
  inputRef?: React.Ref<HTMLTextAreaElement>
}

export function Composer({
  streaming,
  onSubmit,
  onAbort,
  disabled,
  inputRef,
}: ComposerProps) {
  const { t } = useTranslation()
  const draft = useAssistantStreamState().draft
  const setDraft = assistantStream.setDraft

  function send() {
    const trimmed = draft.trim()
    if (!trimmed || streaming || disabled) return
    onSubmit(trimmed)
    setDraft("")
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    // Enter sends, Shift+Enter adds a line, Ctrl/Cmd+Enter sends too. Enter
    // that confirms an IME composition is not a send.
    if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return
    e.preventDefault()
    send()
  }

  return (
    <form
      className="flex flex-col gap-2 border-t border-line bg-panel-raised p-3"
      onSubmit={(e) => {
        e.preventDefault()
        send()
      }}
    >
      <div className="flex items-end gap-2">
        <textarea
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
          rows={2}
          disabled={disabled}
          placeholder={t("assistant.composer.placeholder")}
          aria-label={t("assistant.composer.aria_label")}
          data-testid="assistant-composer-input"
          className="max-h-40 min-h-[2.5rem] flex-1 resize-none rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
        />
        {streaming ? (
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={onAbort}
            aria-label={t("assistant.composer.abort")}
            data-testid="assistant-composer-abort"
          >
            <StopCircle className="h-4 w-4" />
          </Button>
        ) : (
          <Button
            type="submit"
            size="icon"
            disabled={disabled || draft.trim() === ""}
            aria-label={t("assistant.composer.send")}
            data-testid="assistant-composer-submit"
          >
            <Send className="h-4 w-4" />
          </Button>
        )}
      </div>
    </form>
  )
}
