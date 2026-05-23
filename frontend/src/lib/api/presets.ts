import { apiFetch } from "./client"

export type ApplyConflict = "skip" | "overwrite"
export type PresetSort = "name" | "recent"

/** One food entry inside a PresetPlate. Mirrors the kind-aware quantity
 * model on plate components: composed foods carry `portions` (int); leaf
 * foods carry `amount + unit + grams + grams_source`. */
export interface PresetComponent {
  id: number
  food_id: number
  portions?: number | null
  amount?: number | null
  unit?: string | null
  grams?: number | null
  grams_source?: string | null
  note?: string | null
  sort_order: number
}

/** One plate inside a preset, bound to a time_slots row. */
export interface PresetPlate {
  id: number
  slot_id: number
  sort_order: number
  components: PresetComponent[]
}

export interface Preset {
  id: number
  name: string
  tags: string[]
  plates: PresetPlate[]
  created_at: string
  updated_at: string
  last_used_at?: string | null
}

export interface KnownTag {
  tag: string
  count: number
}

export interface PresetListResponse {
  items: Preset[]
  total: number
  known_tags: KnownTag[]
}

export interface ListPresetsParams {
  search?: string
  slot_ids?: number[]
  tags?: string[]
  sort?: PresetSort
  limit?: number
  offset?: number
}

export interface CreatePresetInput {
  name: string
  plate_ids: number[]
  tags?: string[]
}

/** Editor save payload. Any field may be omitted to mean "leave as-is". */
export interface UpdatePresetInput {
  name?: string
  tags?: string[]
  plates?: UpdatePresetPlateInput[]
}

export interface UpdatePresetPlateInput {
  slot_id: number
  components: UpdatePresetComponentInput[]
}

export interface UpdatePresetComponentInput {
  food_id: number
  portions?: number | null
  amount?: number | null
  unit?: string | null
  note?: string | null
}

export interface PatchPresetInput {
  name?: string
  add_tags?: string[]
  remove_tags?: string[]
}

/** Apply pipeline IO — see backend feature.md §6.5. */
export interface ApplyPresetInput {
  target_date: string // YYYY-MM-DD
  on_conflict?: ApplyConflict
  slot_ids_filter?: number[]
}

export interface ApplyResultPlate {
  id: number
  date: string
  slot_id: number
}

export interface ApplyResultReplaced {
  new_plate: ApplyResultPlate
  old_plate: ApplyResultPlate
}

export interface ApplyResultSkip {
  date?: string
  slot_id: number
}

export interface ApplySnapshotComponent {
  food_id: number
  portions?: number | null
  amount?: number | null
  unit?: string | null
  grams?: number | null
  grams_source?: string | null
  sort_order: number
}

export interface ApplySnapshotPlate {
  date: string
  slot_id: number
  note?: string | null
  skipped: boolean
  components: ApplySnapshotComponent[]
}

export interface ApplySnapshot {
  created_plate_ids: number[]
  replaced_plates: ApplySnapshotPlate[]
}

export interface ApplyResult {
  created: ApplyResultPlate[]
  replaced: ApplyResultReplaced[]
  skipped_occupied: ApplyResultSkip[]
  skipped_no_slot: ApplyResultSkip[]
  snapshot: ApplySnapshot
}

export interface CopyWeekInput {
  source_start: string
  target_start: string
  on_conflict?: ApplyConflict
}

function buildQuery(params: ListPresetsParams): string {
  const qs = new URLSearchParams()
  if (params.search) qs.set("search", params.search)
  for (const id of params.slot_ids ?? []) qs.append("slot_id", String(id))
  for (const tag of params.tags ?? []) qs.append("tag", tag)
  if (params.sort) qs.set("sort", params.sort)
  if (params.limit != null) qs.set("limit", String(params.limit))
  if (params.offset != null) qs.set("offset", String(params.offset))
  return qs.toString() ? `?${qs.toString()}` : ""
}

export async function getPresets(
  params: ListPresetsParams = {}
): Promise<PresetListResponse> {
  return apiFetch(`/presets${buildQuery(params)}`)
}

export function getPreset(id: number): Promise<Preset> {
  return apiFetch(`/presets/${id}`)
}

export function getKnownTags(): Promise<{ items: KnownTag[] }> {
  return apiFetch(`/presets/known-tags`)
}

export function createPreset(input: CreatePresetInput): Promise<Preset> {
  return apiFetch(`/presets`, { method: "POST", body: JSON.stringify(input) })
}

export function updatePreset(
  id: number,
  input: UpdatePresetInput
): Promise<Preset> {
  return apiFetch(`/presets/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
}

export function patchPreset(
  id: number,
  input: PatchPresetInput
): Promise<Preset> {
  return apiFetch(`/presets/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  })
}

export function deletePreset(id: number): Promise<void> {
  return apiFetch(`/presets/${id}`, { method: "DELETE" })
}

export function duplicatePreset(id: number): Promise<Preset> {
  return apiFetch(`/presets/${id}/duplicate`, { method: "POST" })
}

export function applyPreset(
  id: number,
  input: ApplyPresetInput
): Promise<ApplyResult> {
  return apiFetch(`/presets/${id}/apply`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function copyWeek(input: CopyWeekInput): Promise<ApplyResult> {
  return apiFetch(`/presets/copy-week`, {
    method: "POST",
    body: JSON.stringify(input),
  })
}

export function undoApply(snapshot: ApplySnapshot): Promise<void> {
  return apiFetch(`/presets/undo-apply`, {
    method: "POST",
    body: JSON.stringify({ snapshot }),
  })
}
