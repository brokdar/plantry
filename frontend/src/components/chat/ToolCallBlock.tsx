import { Check, Loader2, Wrench, X } from "lucide-react"
import { useTranslation } from "react-i18next"

import type { ToolCallState } from "@/lib/stores/chat-stream"
import { cn } from "@/lib/utils"

interface ToolCallBlockProps {
  tool: ToolCallState
}

export function ToolCallBlock({ tool }: ToolCallBlockProps) {
  const { t } = useTranslation()
  const statusLabel = t(`chat.tool.status.${tool.status}`, {
    defaultValue: tool.status,
  })

  return (
    <div
      data-testid="chat-tool-call"
      data-state={tool.status}
      className={cn(
        "mt-2 flex items-start gap-2 rounded-md border bg-muted/40 px-3 py-2 text-sm",
        tool.status === "error" && "border-destructive/40 bg-destructive/10",
        tool.status === "ok" && "border-primary/30"
      )}
    >
      <div className="mt-0.5 flex h-5 w-5 items-center justify-center">
        {tool.status === "running" && (
          <Loader2 className="h-4 w-4 animate-spin text-primary" />
        )}
        {tool.status === "pending" && (
          <Wrench className="h-4 w-4 text-muted-foreground" />
        )}
        {tool.status === "ok" && <Check className="h-4 w-4 text-primary" />}
        {tool.status === "error" && <X className="h-4 w-4 text-destructive" />}
      </div>
      <div className="flex-1">
        <div className="flex items-center justify-between gap-2">
          <code className="font-mono text-xs">{tool.name}</code>
          <span className="text-xs text-muted-foreground">
            {statusLabel}
            {typeof tool.durationMs === "number" &&
              ` · ${Math.round(tool.durationMs)} ms`}
          </span>
        </div>
        {tool.argsJson && (
          <details className="mt-1">
            <summary className="cursor-pointer text-xs text-muted-foreground">
              {t("chat.tool.args")}
            </summary>
            <pre className="mt-1 overflow-x-auto rounded bg-muted p-2 text-xs">
              {tool.argsJson}
            </pre>
          </details>
        )}
      </div>
    </div>
  )
}
