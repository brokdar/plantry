import { cn } from "@/lib/utils"

import { FilterChip } from "./FilterChip"

export type FilterChipOption = {
  value: string
  label: React.ReactNode
  ariaLabel?: string
  testId?: string
}

type BaseProps = {
  className?: string
  ariaLabel?: string
  testId?: string
}

type SingleSelectProps = BaseProps & {
  options: readonly FilterChipOption[]
  value: string | null
  onValueChange: (value: string | null) => void
  multi?: false
  allowDeselect?: boolean
  children?: never
}

type MultiSelectProps = BaseProps & {
  options: readonly FilterChipOption[]
  values: readonly string[]
  onValuesChange: (values: string[]) => void
  multi: true
  children?: never
}

type ChildrenProps = BaseProps & {
  children: React.ReactNode
  options?: never
  value?: never
  onValueChange?: never
  values?: never
  onValuesChange?: never
  multi?: never
}

export type FilterChipGroupProps =
  | SingleSelectProps
  | MultiSelectProps
  | ChildrenProps

export function FilterChipGroup(props: FilterChipGroupProps) {
  const { className, ariaLabel, testId } = props

  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={cn("flex flex-wrap gap-2", className)}
      data-testid={testId ?? "filter-chip-group"}
    >
      {renderChips(props)}
    </div>
  )
}

function renderChips(props: FilterChipGroupProps): React.ReactNode {
  if ("children" in props && props.children !== undefined) {
    return props.children
  }

  if ("multi" in props && props.multi) {
    const { options, values, onValuesChange } = props
    return options.map((option) => {
      const selected = values.includes(option.value)
      return (
        <FilterChip
          key={option.value}
          selected={selected}
          ariaLabel={option.ariaLabel}
          testId={option.testId}
          onClick={() => {
            const next = selected
              ? values.filter((v) => v !== option.value)
              : [...values, option.value]
            onValuesChange(next)
          }}
        >
          {option.label}
        </FilterChip>
      )
    })
  }

  if ("options" in props && props.options !== undefined) {
    const { options, value, onValueChange, allowDeselect = true } = props
    return options.map((option) => {
      const selected = value === option.value
      return (
        <FilterChip
          key={option.value}
          selected={selected}
          ariaLabel={option.ariaLabel}
          testId={option.testId}
          onClick={() => {
            if (selected) {
              if (allowDeselect) onValueChange(null)
              return
            }
            onValueChange(option.value)
          }}
        >
          {option.label}
        </FilterChip>
      )
    })
  }

  return null
}
