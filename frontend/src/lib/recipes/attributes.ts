import type { AttributeField, AttributeValues, Taste } from "@/lib/api/foods"

/** The attribute vocabularies (backend `flavor` package). */
export const ATTRIBUTE_OPTIONS = {
  cuisine: [
    "german",
    "italian",
    "french",
    "mediterranean",
    "indian",
    "chinese",
    "japanese",
    "thai",
    "mexican",
    "middle_eastern",
    "neutral",
  ],
  kind: [
    "stew",
    "curry",
    "pasta",
    "roast",
    "pan",
    "bake",
    "salad",
    "soup",
    "bowl",
    "grain",
    "veg",
    "egg",
    "breakfast",
    "sauce",
    "pickle",
    "bread",
    "fish",
    "wrap",
  ],
  texture: ["soft", "creamy", "chewy", "crisp", "crunchy"],
  temperature: ["hot", "warm", "cold"],
  colour: [
    "green",
    "red",
    "orange",
    "beige",
    "brown",
    "white",
    "purple",
    "mixed",
  ],
  technique: [
    "braise",
    "roast",
    "fry",
    "saute",
    "grill",
    "boil",
    "bake",
    "raw",
    "steam",
    "simmer",
  ],
  format: ["plate", "bowl", "one_pot", "casserole", "wrap", "soup", "salad"],
} as const

export type ChoiceField = keyof typeof ATTRIBUTE_OPTIONS

/** Recipe attributes in display order; a leaf carries the last three. */
export const RECIPE_FIELDS: AttributeField[] = [
  "cuisine",
  "kind",
  "heroes",
  "texture",
  "temperature",
  "saucy",
  "richness",
  "colour",
  "taste",
  "technique",
  "format",
]

export const TASTE_AXES: (keyof Taste)[] = [
  "salt",
  "acid",
  "sweet",
  "bitter",
  "umami",
  "heat",
]

export const MAX_HEROES = 3

export function isChoiceField(field: AttributeField): field is ChoiceField {
  return field in ATTRIBUTE_OPTIONS
}

/** The value of a single-choice field as an option key (saucy and richness
 *  included), undefined when unknown. */
export function choiceValue(
  field: AttributeField,
  values: AttributeValues
): string | undefined {
  const v = values[field]
  if (v === undefined || Array.isArray(v) || typeof v === "object") {
    return undefined
  }
  return String(v)
}
