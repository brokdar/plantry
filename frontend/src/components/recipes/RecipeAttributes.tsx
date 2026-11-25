import { useState } from "react"
import { useTranslation } from "react-i18next"
import type { TFunction } from "i18next"

import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import type {
  AttributeField,
  AttributePatch,
  AttributeSource,
  AttributeValues,
  ComposedFood,
  Taste,
} from "@/lib/api/foods"
import { useFoodAttributes, useSetFoodAttributes } from "@/lib/queries/foods"
import {
  ATTRIBUTE_OPTIONS,
  MAX_HEROES,
  RECIPE_FIELDS,
  TASTE_AXES,
  choiceValue,
  isChoiceField,
} from "@/lib/recipes/attributes"
import { cn } from "@/lib/utils"

const LEVELS = [0, 1, 2, 3]
const NO_TASTE: Taste = {
  salt: 0,
  acid: 0,
  sweet: 0,
  bitter: 0,
  umami: 0,
  heat: 0,
}

/** The options of a single-choice field; saucy and richness included. */
function optionsOf(field: AttributeField): readonly string[] {
  if (isChoiceField(field)) return ATTRIBUTE_OPTIONS[field]
  if (field === "saucy") return ["true", "false"]
  if (field === "richness") return LEVELS.map(String)
  return []
}

/** The option key as the API value. */
function patchValue(field: AttributeField, option: string): AttributePatch {
  if (field === "saucy") return { saucy: option === "true" }
  if (field === "richness") return { richness: Number(option) }
  return { [field]: option }
}

function optionLabel(t: TFunction, field: AttributeField, option: string) {
  switch (field) {
    case "cuisine":
      return t(`attributes.cuisine.${option}`)
    case "kind":
      return t(`attributes.kind.${option}`)
    case "texture":
      return t(`attributes.texture.${option}`)
    case "temperature":
      return t(`attributes.temperature.${option}`)
    case "saucy":
      return t(`attributes.saucy.${option}`)
    case "richness":
      return t(`attributes.richness.${option}`)
    case "colour":
      return t(`attributes.colour.${option}`)
    case "technique":
      return t(`attributes.technique.${option}`)
    case "format":
      return t(`attributes.format.${option}`)
    default:
      return option
  }
}

function valueText(
  t: TFunction,
  field: AttributeField,
  values: AttributeValues
): string | undefined {
  if (field === "heroes") return values.heroes?.join(", ")
  if (field === "taste") {
    if (!values.taste) return undefined
    const taste = values.taste
    const parts = TASTE_AXES.filter((a) => taste[a] > 0).map(
      (a) => `${t(`attributes.taste.${a}`)} ${taste[a]}`
    )
    return parts.length > 0 ? parts.join(" · ") : t("attributes.taste.none")
  }
  const v = choiceValue(field, values)
  return v === undefined ? undefined : optionLabel(t, field, v)
}

/** Recipe attribute chips (ATTR-5): every attribute, derived or set; each
 *  chip opens an editor, and a user value always wins until reset to
 *  automatic. */
export function RecipeAttributes({ recipe }: { recipe: ComposedFood }) {
  const { t } = useTranslation()
  const { data } = useFoodAttributes(recipe.id)
  const set = useSetFoodAttributes(recipe.id)
  if (!data) return null
  const ingredients = [
    ...new Set((recipe.children ?? []).map((c) => c.child_name)),
  ]

  return (
    <section
      aria-labelledby="recipe-attributes-title"
      className="flex flex-col gap-2"
      data-testid="recipe-attributes"
    >
      <h2
        id="recipe-attributes-title"
        className="text-small font-medium text-ink-muted"
      >
        {t("attributes.title")}
      </h2>
      <ul className="flex flex-wrap gap-2">
        {RECIPE_FIELDS.map((field) => (
          <li key={field}>
            <AttributeChip
              field={field}
              values={data.attributes}
              source={data.sources[field]}
              ingredients={ingredients}
              disabled={set.isPending}
              onSave={(patch) => set.mutate(patch)}
            />
          </li>
        ))}
      </ul>
    </section>
  )
}

type ChipProps = {
  field: AttributeField
  values: AttributeValues
  source?: AttributeSource
  ingredients: string[]
  disabled: boolean
  onSave: (patch: AttributePatch) => void
}

function AttributeChip({
  field,
  values,
  source,
  ingredients,
  disabled,
  onSave,
}: ChipProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const text = valueText(t, field, values)
  const fieldLabel = t(`attributes.field.${field}`)
  const byUser = source === "user"

  function save(patch: AttributePatch) {
    setOpen(false)
    onSave(patch)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          disabled={disabled}
          className={cn(
            "inline-flex min-h-11 items-center gap-1.5 rounded-tag border border-line bg-panel px-3 text-small hover:bg-panel-sunken focus-visible:ring-2 focus-visible:ring-accent focus-visible:outline-none disabled:opacity-60 md:pointer-fine:min-h-8",
            !text && "border-dashed"
          )}
          data-testid={`attr-chip-${field}`}
          data-source={source ?? "none"}
        >
          <span className="text-ink-muted">{fieldLabel}</span>
          <span className={cn(text ? "text-ink" : "text-ink-muted")}>
            {text ?? "–"}
          </span>
          {byUser && (
            <>
              <span aria-hidden className="size-1.5 rounded-full bg-accent" />
              <span className="sr-only">{t("attributes.by_you")}</span>
            </>
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto max-w-80">
        <p className="text-small font-medium">{fieldLabel}</p>
        {field === "heroes" ? (
          <HeroesEditor
            initial={values.heroes ?? []}
            ingredients={ingredients}
            onSave={(heroes) => save({ heroes })}
          />
        ) : field === "taste" ? (
          <TasteEditor
            initial={values.taste ?? NO_TASTE}
            onSave={(taste) => save({ taste })}
          />
        ) : (
          <div className="flex flex-wrap gap-1.5" role="group">
            {optionsOf(field).map((option) => (
              <Button
                key={option}
                type="button"
                size="sm"
                variant={
                  choiceValue(field, values) === option ? "default" : "outline"
                }
                aria-pressed={choiceValue(field, values) === option}
                onClick={() => save(patchValue(field, option))}
                data-testid={`attr-option-${option}`}
              >
                {optionLabel(t, field, option)}
              </Button>
            ))}
          </div>
        )}
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="self-start"
          disabled={!byUser}
          onClick={() => save({ [field]: null })}
          data-testid="attr-reset"
        >
          {t("attributes.automatic")}
        </Button>
      </PopoverContent>
    </Popover>
  )
}

function HeroesEditor({
  initial,
  ingredients,
  onSave,
}: {
  initial: string[]
  ingredients: string[]
  onSave: (heroes: string[]) => void
}) {
  const { t } = useTranslation()
  const [chosen, setChosen] = useState(initial)
  const toggle = (name: string) =>
    setChosen((c) =>
      c.includes(name) ? c.filter((n) => n !== name) : [...c, name]
    )

  return (
    <>
      <p className="text-caption text-ink-muted">
        {t("attributes.heroes_hint", { count: MAX_HEROES })}
      </p>
      <ul className="flex max-h-60 flex-col gap-1 overflow-y-auto">
        {ingredients.map((name) => {
          const checked = chosen.includes(name)
          return (
            <li key={name}>
              <label className="flex min-h-9 items-center gap-2">
                <input
                  type="checkbox"
                  className="size-4 accent-(--accent)"
                  checked={checked}
                  disabled={!checked && chosen.length >= MAX_HEROES}
                  onChange={() => toggle(name)}
                />
                {name}
              </label>
            </li>
          )
        })}
      </ul>
      <Button
        type="button"
        size="sm"
        className="self-start"
        disabled={chosen.length === 0}
        onClick={() => onSave(chosen)}
        data-testid="attr-save"
      >
        {t("common.save")}
      </Button>
    </>
  )
}

function TasteEditor({
  initial,
  onSave,
}: {
  initial: Taste
  onSave: (taste: Taste) => void
}) {
  const { t } = useTranslation()
  const [taste, setTaste] = useState(initial)

  return (
    <>
      <div className="flex flex-col gap-1.5">
        {TASTE_AXES.map((axis) => (
          <div
            key={axis}
            role="group"
            aria-label={t(`attributes.taste.${axis}`)}
            className="flex items-center justify-between gap-3"
          >
            <span className="text-small">{t(`attributes.taste.${axis}`)}</span>
            <span className="flex gap-1">
              {LEVELS.map((level) => (
                <Button
                  key={level}
                  type="button"
                  size="sm"
                  variant={taste[axis] === level ? "default" : "outline"}
                  aria-pressed={taste[axis] === level}
                  className="w-9 tabular-nums"
                  onClick={() => setTaste((s) => ({ ...s, [axis]: level }))}
                >
                  {level}
                </Button>
              ))}
            </span>
          </div>
        ))}
      </div>
      <Button
        type="button"
        size="sm"
        className="self-start"
        onClick={() => onSave(taste)}
        data-testid="attr-save"
      >
        {t("common.save")}
      </Button>
    </>
  )
}
