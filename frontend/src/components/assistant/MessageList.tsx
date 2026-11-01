import { TriangleAlert } from "lucide-react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { useTranslation } from "react-i18next"

import type { ToolFailure, Transcript } from "@/lib/assistant/transcript"
import type { Change } from "@/lib/domain/assistantEvents"
import type { ChangeLineState } from "@/lib/stores/assistant-stream"
import { cn } from "@/lib/utils"

import { ChangeLine } from "./ChangeLine"

interface MessageProps {
  role: "user" | "assistant" | "error"
  text: string
}

function Message({ role, text }: MessageProps) {
  const isUser = role === "user"
  return (
    <div
      data-testid={`assistant-message-${role}`}
      className={cn("flex", isUser ? "justify-end" : "justify-start")}
    >
      <div
        className={cn(
          "max-w-[85%] rounded-lg px-3 py-2 text-sm",
          isUser && "bg-primary text-primary-foreground",
          role === "assistant" && "bg-muted",
          role === "error" && "bg-destructive/15 text-destructive"
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

/** One line for a tool call that failed: nothing changed. */
function FailureLine() {
  const { t } = useTranslation()
  return (
    <div
      data-testid="assistant-failure"
      role="status"
      className="flex items-center gap-2 rounded-cell border border-line bg-panel px-3 py-2 text-small text-danger"
    >
      <TriangleAlert className="size-4 shrink-0" aria-hidden="true" />
      {t("assistant.failed")}
    </div>
  )
}

interface MessageListProps {
  transcript: Transcript
  /** Changes applied by the turn in flight that the conversation does not
   *  hold yet. */
  liveChanges: Change[]
  /** Failed tool calls of the turn in flight, likewise. */
  liveFailures: ToolFailure[]
  /** The reply being streamed, shown until it is persisted. */
  streamText: string
  streaming: boolean
  lineState: (change: Change) => ChangeLineState | null
  onUndo: (id: number) => void
}

/** The transcript: persisted turns with their change lines, then the
 *  in-flight changes and reply. */
export function MessageList({
  transcript,
  liveChanges,
  liveFailures,
  streamText,
  streaming,
  lineState,
  onUndo,
}: MessageListProps) {
  const failures = (list: ToolFailure[]) =>
    list.map((f) => <FailureLine key={f.id} />)
  const lines = (changes: Change[]) =>
    changes.map((c) => (
      <ChangeLine key={c.id} change={c} state={lineState(c)} onUndo={onUndo} />
    ))
  return (
    <>
      {transcript.turns.map(
        ({ message: m, text, changes, failures: fails }) => (
          <div key={m.id} className="space-y-2">
            {text !== "" && (
              <Message
                role={
                  m.role === "assistant"
                    ? "assistant"
                    : m.role === "user"
                      ? "user"
                      : "error"
                }
                text={text}
              />
            )}
            {lines(changes)}
            {failures(fails)}
          </div>
        )
      )}
      {transcript.rest.length > 0 && (
        <div className="space-y-2">{lines(transcript.rest)}</div>
      )}
      {(streaming ||
        streamText !== "" ||
        liveChanges.length > 0 ||
        liveFailures.length > 0) && (
        <div data-testid="assistant-streaming" className="space-y-2">
          {lines(liveChanges)}
          {failures(liveFailures)}
          {streamText !== "" && <Message role="assistant" text={streamText} />}
        </div>
      )}
    </>
  )
}
