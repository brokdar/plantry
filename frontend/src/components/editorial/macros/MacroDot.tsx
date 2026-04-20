import { cn } from "@/lib/utils"
import { MACRO_DOT_CLASS, type MacroKind } from "./tokens"

interface MacroDotProps {
  kind: MacroKind
  size?: "xs" | "sm" | "md"
  className?: string
}

const SIZE: Record<NonNullable<MacroDotProps["size"]>, string> = {
  xs: "size-1.5",
  sm: "size-2",
  md: "size-2.5",
}

export function MacroDot({ kind, size = "sm", className }: MacroDotProps) {
  return (
    <span
      aria-hidden
      className={cn(
        "inline-block shrink-0 rounded-full",
        SIZE[size],
        MACRO_DOT_CLASS[kind],
        className
      )}
    />
  )
}
