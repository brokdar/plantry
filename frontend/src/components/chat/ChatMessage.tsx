import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"

import type { ChatEvent, ConversationMessage } from "@/lib/domain/chatEvents"
import { cn } from "@/lib/utils"

interface ChatMessageProps {
  role: "user" | "assistant" | "tool" | "error"
  text: string
}

export function ChatMessage({ role, text }: ChatMessageProps) {
  const isUser = role === "user"
  return (
    <div
      data-testid={`chat-message-${role}`}
      className={cn("flex", isUser ? "justify-end" : "justify-start")}
    >
      <div
        className={cn(
          "max-w-[85%] rounded-lg px-3 py-2 text-sm",
          isUser && "bg-primary text-primary-foreground",
          role === "assistant" && "bg-muted",
          role === "error" && "bg-destructive/15 text-destructive",
          role === "tool" && "bg-muted text-xs text-muted-foreground"
        )}
      >
        {isUser ? (
          <p className="break-words whitespace-pre-wrap">{text}</p>
        ) : (
          <div className="prose prose-sm dark:prose-invert max-w-none break-words">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              // react-markdown sanitizes by default; skipHtml prevents raw HTML.
              skipHtml
            >
              {text || " "}
            </ReactMarkdown>
          </div>
        )}
      </div>
    </div>
  )
}

// Extract a plain-text representation from persisted ConversationMessage
// content (an array of provider-neutral content blocks). Tool results are
// rendered as a compact one-liner.
export function messageToText(m: ConversationMessage): string {
  const blocks = (Array.isArray(m.content) ? m.content : []) as Array<{
    type?: string
    text?: string
    tool_use_name?: string
    tool_result_id?: string
  }>
  return blocks
    .map((b) => {
      if (b.type === "text") return b.text ?? ""
      if (b.type === "tool_use") return `(called ${b.tool_use_name})`
      if (b.type === "tool_result") return `(result for ${b.tool_result_id})`
      return ""
    })
    .filter(Boolean)
    .join("\n\n")
}

// Compatibility placeholder export; ChatEvent may be used by history renderers
// to interleave live deltas with persisted turns.
export type { ChatEvent }
