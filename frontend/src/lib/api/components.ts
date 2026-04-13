import { apiFetch } from "./client"

export interface ComponentIngredient {
  id: number
  component_id: number
  ingredient_id: number
  amount: number
  unit: string
  grams: number
  sort_order: number
}

export interface Instruction {
  id: number
  component_id: number
  step_number: number
  text: string
}

export interface Component {
  id: number
  name: string
  role: string
  variant_group_id: number | null
  reference_portions: number
  prep_minutes: number | null
  cook_minutes: number | null
  image_path: string | null
  notes: string | null
  last_cooked_at: string | null
  cook_count: number
  ingredients: ComponentIngredient[]
  instructions: Instruction[]
  tags: string[]
  created_at: string
  updated_at: string
}

export interface ComponentListResponse {
  items: Component[]
  total: number
}

export interface ComponentIngredientInput {
  ingredient_id: number
  amount: number
  unit: string
  grams: number
  sort_order: number
}

export interface InstructionInput {
  step_number: number
  text: string
}

export interface ComponentInput {
  name: string
  role: string
  reference_portions: number
  prep_minutes?: number
  cook_minutes?: number
  notes?: string | null
  ingredients?: ComponentIngredientInput[]
  instructions?: InstructionInput[]
  tags?: string[]
}

export interface ComponentListParams {
  search?: string
  role?: string
  tag?: string
  limit?: number
  offset?: number
  sort?: string
  order?: string
}

export interface ComponentNutrition {
  kcal: number
  protein: number
  fat: number
  carbs: number
  fiber: number
  sodium: number
}

export function listComponents(
  params?: ComponentListParams
): Promise<ComponentListResponse> {
  const query = new URLSearchParams()
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        query.set(key, String(value))
      }
    })
  }
  const qs = query.toString()
  return apiFetch(`/components${qs ? `?${qs}` : ""}`)
}

export function getComponent(id: number): Promise<Component> {
  return apiFetch(`/components/${id}`)
}

export function createComponent(input: ComponentInput): Promise<Component> {
  return apiFetch("/components", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function updateComponent(
  id: number,
  input: ComponentInput
): Promise<Component> {
  return apiFetch(`/components/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deleteComponent(id: number): Promise<void> {
  return apiFetch(`/components/${id}`, {
    method: "DELETE",
  })
}

export function getComponentNutrition(id: number): Promise<ComponentNutrition> {
  return apiFetch(`/components/${id}/nutrition`)
}

export interface VariantListResponse {
  items: Component[]
}

export function createVariant(id: number): Promise<Component> {
  return apiFetch(`/components/${id}/variant`, { method: "POST" })
}

export function listVariants(id: number): Promise<VariantListResponse> {
  return apiFetch(`/components/${id}/variants`)
}
