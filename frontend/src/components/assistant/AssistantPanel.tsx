import { Plus } from "lucide-react"
import { useEffect, useMemo, useRef } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import type { Plan } from "@/lib/api/plan"
import { buildTranscript } from "@/lib/assistant/transcript"
import type { Change, ConversationChange } from "@/lib/domain/assistantEvents"
import {
  useAssistantStream,
  useConversation,
  useUndoChange,
} from "@/lib/queries/ai"
import {
  assistantStream,
  useAssistantStreamState,
} from "@/lib/stores/assistant-stream"

import { Composer } from "./Composer"
import { ConversationHistory } from "./ConversationHistory"
import { LiveRegion } from "./LiveRegion"
import { MessageList } from "./MessageList"
import { PanelShell } from "./PanelShell"
import { StatusLine } from "./StatusLine"
import { SuggestionChips } from "./SuggestionChips"
import { UnlockCard } from "./UnlockCard"

interface AssistantPanelProps {
  open: boolean
  /** Below 768 px the panel is a bottom sheet. */
  phone: boolean
  /** An AI provider is configured. */
  enabled: boolean
  /** The settings are still loading: the provider is not known yet. */
  loading?: boolean
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
  loading,
  plan,
  onClose,
}: AssistantPanelProps) {
  const { t } = useTranslation()
  const chatStream = useAssistantStream()
  const stream = useAssistantStreamState()
  const activeConversationId = stream.conversationId
  const { data: conversation } = useConversation(activeConversationId)
  const undo = useUndoChange()
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

  const transcript = useMemo(
    () =>
      buildTranscript(
        conversation?.messages ?? [],
        conversation?.changes ?? []
      ),
    [conversation]
  )
  // Changes of the turn in flight until the refetched conversation holds them.
  const liveChanges = useMemo(() => {
    const held = new Set((conversation?.changes ?? []).map((c) => c.id))
    return stream.changes.filter((c) => !held.has(c.id))
  }, [conversation, stream.changes])
  const empty =
    transcript.turns.length === 0 &&
    transcript.rest.length === 0 &&
    liveChanges.length === 0 &&
    !showStreamText &&
    !chatStream.isStreaming

  function lineState(c: Change) {
    return (
      stream.lineStates[c.id] ??
      ((c as ConversationChange).undone_at ? ("undone" as const) : null)
    )
  }

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
        {!enabled && !loading && (
          <p className="text-small text-ink-muted">
            {t("assistant.provider_missing")}
          </p>
        )}
        <MessageList
          transcript={transcript}
          liveChanges={liveChanges}
          streamText={showStreamText ? stream.text : ""}
          streaming={chatStream.isStreaming}
          lineState={lineState}
          onUndo={(id) => undo.mutate(id)}
        />
        {stream.unlockRequest && (
          <UnlockCard
            request={stream.unlockRequest}
            slots={plan.slots}
            disabled={chatStream.isStreaming}
            onConfirm={(text) => void handleSubmit(text)}
          />
        )}
        {enabled && empty && <SuggestionChips plan={plan} onPick={pick} />}
      </div>
      <LiveRegion change={stream.changes.at(-1) ?? null} />
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
