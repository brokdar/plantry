import { Plus } from "lucide-react"
import { useEffect, useMemo, useRef } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { Plan } from "@/lib/api/plan"
import { useAssistantStream, useConversation } from "@/lib/queries/ai"
import {
  assistantStream,
  useAssistantStreamState,
} from "@/lib/stores/assistant-stream"

import { Composer } from "./Composer"
import { ConversationHistory } from "./ConversationHistory"
import { MessageList } from "./MessageList"
import { PanelShell } from "./PanelShell"
import { StatusLine } from "./StatusLine"
import { SuggestionChips } from "./SuggestionChips"

interface AssistantPanelProps {
  open: boolean
  /** Below 768 px the panel is a bottom sheet. */
  phone: boolean
  /** An AI provider is configured. */
  enabled: boolean
  plan: Plan
  onClose: () => void
}

/**
 * The assistant on `/plan` (UX §5.6): transcript, status line, suggestion
 * chips for an empty conversation and the composer. Each turn sends the
 * window the board shows; conversations are not bound to windows (D19).
 */
export function AssistantPanel({
  open,
  phone,
  enabled,
  plan,
  onClose,
}: AssistantPanelProps) {
  const { t } = useTranslation()
  const chatStream = useAssistantStream()
  const stream = useAssistantStreamState()
  const activeConversationId = stream.conversationId
  const { data: conversation } = useConversation(activeConversationId)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // The aside has no focus trap to move focus in: put it in the composer.
  useEffect(() => {
    if (open && enabled && !phone) inputRef.current?.focus()
  }, [open, enabled, phone])

  // Once a stream completes, useAssistantStream has refetched the
  // conversation, so the final reply is already persisted. Keep the buffered
  // text only while streaming, or after a failed stream that may not be.
  const showStreamText =
    stream.text !== "" && (chatStream.isStreaming || chatStream.isError)

  const turns = useMemo(
    () => (conversation?.messages ?? []).filter((m) => m.role !== "tool"),
    [conversation]
  )
  const empty =
    turns.length === 0 &&
    !showStreamText &&
    stream.toolCalls.length === 0 &&
    !chatStream.isStreaming

  async function handleSubmit(text: string) {
    try {
      await chatStream.mutateAsync({
        conversationId: activeConversationId ?? undefined,
        start: plan.window.start,
        message: text,
      })
    } catch {
      // `error` events are already in the transcript; keep the panel usable.
    }
  }

  function handleNewConversation() {
    if (chatStream.isStreaming) chatStream.abort()
    assistantStream.setConversation(null)
    assistantStream.reset()
  }

  function pick(text: string) {
    assistantStream.setDraft(text)
    inputRef.current?.focus()
  }

  return (
    <PanelShell
      phone={phone}
      open={open}
      onClose={onClose}
      actions={
        enabled && (
          <div className="flex flex-wrap items-center gap-2">
            <ConversationHistory />
            {activeConversationId !== null && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleNewConversation}
                data-testid="assistant-new-conversation"
              >
                <Plus className="mr-1.5 size-4" aria-hidden="true" />
                {t("assistant.new_conversation")}
              </Button>
            )}
          </div>
        )
      }
    >
      <div
        className="flex-1 space-y-3 overflow-y-auto px-4 py-3"
        data-testid="assistant-transcript"
      >
        {!enabled && (
          <p className="text-small text-ink-muted">
            {t("assistant.provider_missing")}
          </p>
        )}
        <MessageList
          turns={turns}
          streamText={showStreamText ? stream.text : ""}
          toolCalls={stream.toolCalls}
          streaming={chatStream.isStreaming}
        />
        {enabled && empty && <SuggestionChips plan={plan} onPick={pick} />}
      </div>
      <StatusLine streaming={chatStream.isStreaming} status={stream.status} />
      <Composer
        streaming={chatStream.isStreaming}
        onSubmit={handleSubmit}
        onAbort={chatStream.abort}
        disabled={!enabled}
        inputRef={inputRef}
      />
    </PanelShell>
  )
}
