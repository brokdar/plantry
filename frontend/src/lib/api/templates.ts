import { apiFetch } from "./client"

export type TemplateScope = "slot" | "day" | "week"
export type ApplyConflict = "skip" | "overwrite"

/** A single entry in a template. Slot-scope templates have one entry with
 * day_offset=0 and slot_id=null. Day-scope entries carry slot_id. Week-scope
 * entries carry both day_offset (0-6) and slot_id. */
export interface TemplateEntry {
  id: number
  template_id: number
  food_id: number
  portions: number
  sort_order: number
  day_offset: number
  slot_id?: number | null
  note?: string | null
}

export interface Template {
  id: number
  name: string
  scope: TemplateScope
  /** Backend serializes entries under "components" for backward compat. */
  components: TemplateEntry[]
  created_at: string
}

/** Backward-compat alias. New code should use TemplateEntry. */
export type TemplateComponent = TemplateEntry

export interface TemplateEntryInput {
  food_id: number
  portions: number
  day_offset?: number
  slot_id?: number | null
  note?: string | null
}

export type TemplateComponentInput = TemplateEntryInput

export interface CreateTemplateInput {
  name: string
  scope?: TemplateScope
  from_plate_id?: number
  /** Build a week-scope template from existing plates in [from, to]. */
  from?: string
  to?: string
  /** Backend accepts either "components" (legacy) or "entries". */
  components?: TemplateEntryInput[]
  entries?: TemplateEntryInput[]
}

export interface UpdateTemplateInput {
  name: string
}

/** Apply payload — shape depends on the template's scope:
 *  - slot: { date, slot_id }
 *  - day:  { date, conflict? }
 *  - week: { start_date, conflict? }
 *
 *  Legacy { start_date, slot_id } still works for slot scope. */
export interface ApplyTemplateInput {
  date?: string
  slot_id?: number
  start_date?: string
  conflict?: ApplyConflict
}

export interface CreateTemplateFromRangeInput {
  name: string
  from: string
  to: string
}

export interface ApplyTemplateResultPlate {
  id: number
  date: string
  slot_id: number
}

export interface ApplyTemplateResultSkipped {
  date: string
  slot_id: number
}

export interface ApplyTemplateResult {
  plates: ApplyTemplateResultPlate[]
  skipped?: ApplyTemplateResultSkipped[]
}

interface TemplateListResponse {
  items: Template[]
}

export async function getTemplates(scope?: TemplateScope): Promise<Template[]> {
  const path = scope ? `/templates?scope=${scope}` : "/templates"
  const res = await apiFetch<TemplateListResponse>(path)
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
): Promise<ApplyTemplateResult> {
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
