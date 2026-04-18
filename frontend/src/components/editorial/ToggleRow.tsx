import { cn } from "@/lib/utils"

type ToggleRowProps = {
  title: string
  description?: string
  checked: boolean
  onChange: (next: boolean) => void
  testId?: string
}

export function ToggleRow({
  title,
  description,
  checked,
  onChange,
  testId,
}: ToggleRowProps) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <h4 className="text-sm font-semibold text-on-surface">{title}</h4>
        {description && (
          <p className="text-xs text-on-surface-variant">{description}</p>
        )}
      </div>
      <label
        className="relative inline-flex cursor-pointer items-center"
        data-testid={testId}
      >
        <input
          type="checkbox"
          className="peer sr-only"
          checked={checked}
          onChange={(e) => onChange(e.target.checked)}
        />
        <div
          className={cn(
            "h-6 w-11 rounded-full transition-colors",
            "bg-surface-container-highest peer-checked:bg-primary"
          )}
        />
        <div
          className={cn(
            "absolute top-1 left-1 size-4 rounded-full bg-outline transition-transform",
            "peer-checked:translate-x-5 peer-checked:bg-on-primary"
          )}
        />
      </label>
    </div>
  )
}
