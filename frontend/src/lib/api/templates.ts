import { apiFetch } from "./client"

export interface TemplateComponent {
  id: number
  template_id: number
  food_id: number
  portions: number
  sort_order: number
}

export interface Template {
  id: number
  name: string
  components: TemplateComponent[]
  created_at: string
}

export interface TemplateComponentInput {
  food_id: number
  portions: number
}

export interface CreateTemplateInput {
  name: string
  from_plate_id?: number
  components?: TemplateComponentInput[]
}

export interface UpdateTemplateInput {
  name: string
}

export interface ApplyTemplateInput {
  start_date: string // YYYY-MM-DD
  slot_id: number
}

export interface CreateTemplateFromRangeInput {
  name: string
  from: string // YYYY-MM-DD
  to: string // YYYY-MM-DD
}

interface TemplateListResponse {
  items: Template[]
}

export async function getTemplates(): Promise<Template[]> {
  const res = await apiFetch<TemplateListResponse>("/templates")
  return res.items
}

export function getTemplate(id: number): Promise<Template> {
  return apiFetch(`/templates/${id}`)
}

export function createTemplate(input: CreateTemplateInput): Promise<Template> {
  return apiFetch(`/templates`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function updateTemplate(
  id: number,
  input: UpdateTemplateInput
): Promise<Template> {
  return apiFetch(`/templates/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function deleteTemplate(id: number): Promise<void> {
  return apiFetch(`/templates/${id}`, { method: "DELETE" })
}

export function applyTemplate(
  id: number,
  input: ApplyTemplateInput
): Promise<void> {
  return apiFetch(`/templates/${id}/apply`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function createTemplateFromRange(
  input: CreateTemplateFromRangeInput
): Promise<Template> {
  return apiFetch(`/templates`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}
