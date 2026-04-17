import { Plus, Sparkles } from "lucide-react"
import { useMemo } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { useAISettings, useChatStream, useConversation } from "@/lib/queries/ai"
import {
  chatStreamStore,
  useChatStream as useChatStreamStore,
} from "@/lib/stores/chat-stream"
import { type ChatMode, useChatUI } from "@/lib/stores/chat-ui"

import { ChatComposer } from "./ChatComposer"
import { ChatMessage, messageToText } from "./ChatMessage"
import { ToolCallBlock } from "./ToolCallBlock"

interface ChatPanelProps {
  weekId: number
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ChatPanel({ weekId, open, onOpenChange }: ChatPanelProps) {
  const { t } = useTranslation()
  const { data: settings } = useAISettings()
  const activeConversationId = useChatUI((s) => s.activeConversationId)
  const setActiveConversation = useChatUI((s) => s.setActiveConversation)
  const { data: conversation } = useConversation(activeConversationId)
  const chatStream = useChatStream()
  const stream = useChatStreamStore()

  const persistedTurns = useMemo(
    () => (conversation?.messages ?? []).filter((m) => m.role !== "tool"),
    [conversation]
  )

  async function handleSubmit(text: string, mode: ChatMode) {
    try {
      await chatStream.mutateAsync({
        conversationId: activeConversationId ?? undefined,
        weekId,
        mode: mode || undefined,
        message: text,
      })
      // The `conversation_ready` event captured by useChatStream already
      // stored the server-assigned id in the chat-ui store, so subsequent
      // submits will reuse this conversation.
    } catch {
      // Errors surfaced as `error` events are already reflected in the
      // transcript; swallow here to keep the panel usable.
    }
  }

  function handleNewConversation() {
    if (chatStream.isStreaming) chatStream.abort()
    setActiveConversation(null)
    chatStreamStore.reset()
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex w-full flex-col sm:max-w-md">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <Sparkles className="h-4 w-4" />
            {t("chat.title")}
          </SheetTitle>
          <SheetDescription>
            {settings?.enabled
              ? t("chat.description", { model: settings.model })
              : t("chat.disabled")}
          </SheetDescription>
          {settings?.enabled && activeConversationId !== null && (
            <div className="pt-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleNewConversation}
                data-testid="chat-new-conversation"
              >
                <Plus className="mr-1.5 h-4 w-4" />
                {t("chat.new_conversation")}
              </Button>
            </div>
          )}
        </SheetHeader>

        <div
          className="flex-1 space-y-3 overflow-y-auto py-4"
          data-testid="chat-history"
        >
          {!settings?.enabled && (
            <p className="text-sm text-muted-foreground">
              {t("chat.provider_missing")}
            </p>
          )}

          {persistedTurns.map((m) => (
            <ChatMessage
              key={m.id}
              role={
                m.role === "assistant"
                  ? "assistant"
                  : m.role === "user"
                    ? "user"
                    : "error"
              }
              text={messageToText(m)}
            />
          ))}

          {(chatStream.isStreaming ||
            stream.text ||
            stream.toolCalls.length > 0) && (
            <div data-testid="chat-streaming">
              {stream.text && (
                <ChatMessage role="assistant" text={stream.text} />
              )}
              {stream.toolCalls.map((tc) => (
                <ToolCallBlock key={tc.id} tool={tc} />
              ))}
            </div>
          )}
        </div>

        <ChatComposer
          streaming={chatStream.isStreaming}
          onSubmit={handleSubmit}
          onAbort={chatStream.abort}
          disabled={!settings?.enabled}
        />
      </SheetContent>
    </Sheet>
  )
}
