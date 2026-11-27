import { ArrowDown, ArrowUp, Trash2 } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"

interface StepsEditorProps {
  steps: string[]
  onChange: (steps: string[]) => void
}

/** The review's cooking steps (REV-2): edit, add, delete and reorder. */
export function StepsEditor({ steps, onChange }: StepsEditorProps) {
  const { t } = useTranslation()

  function move(i: number, by: -1 | 1) {
    const next = [...steps]
    ;[next[i], next[i + by]] = [next[i + by], next[i]]
    onChange(next)
  }

  return (
    <ol className="space-y-2">
      {steps.map((text, i) => (
        <li key={i} className="flex items-start gap-2">
          <span className="mt-2 w-5 shrink-0 text-right text-sm text-ink-muted tabular-nums">
            {i + 1}.
          </span>
          <Textarea
            aria-label={t("import.steps.step_n", { n: i + 1 })}
            value={text}
            rows={2}
            className="min-w-0 flex-1"
            onChange={(e) =>
              onChange(steps.map((s, j) => (j === i ? e.target.value : s)))
            }
          />
          <div className="flex shrink-0 flex-col">
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("import.steps.move_up", { n: i + 1 })}
              disabled={i === 0}
              onClick={() => move(i, -1)}
            >
              <ArrowUp />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("import.steps.move_down", { n: i + 1 })}
              disabled={i === steps.length - 1}
              onClick={() => move(i, 1)}
            >
              <ArrowDown />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("import.steps.delete", { n: i + 1 })}
              onClick={() => onChange(steps.filter((_, j) => j !== i))}
            >
              <Trash2 />
            </Button>
          </div>
        </li>
      ))}
      <li>
        <Button
          variant="outline"
          size="sm"
          onClick={() => onChange([...steps, ""])}
        >
          {t("import.steps.add")}
        </Button>
      </li>
    </ol>
  )
}
