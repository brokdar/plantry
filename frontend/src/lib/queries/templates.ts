import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  applyTemplate,
  createTemplate,
  deleteTemplate,
  getTemplate,
  getTemplates,
  updateTemplate,
  type ApplyTemplateInput,
  type CreateTemplateInput,
  type UpdateTemplateInput,
} from "@/lib/api/templates"

import { templateKeys, weekKeys } from "./keys"

export function useTemplates() {
  return useQuery({
    queryKey: templateKeys.lists(),
    queryFn: getTemplates,
  })
}

export function useTemplate(id: number) {
  return useQuery({
    queryKey: templateKeys.detail(id),
    queryFn: () => getTemplate(id),
    enabled: id > 0,
  })
}

export function useCreateTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateTemplateInput) => createTemplate(input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: templateKeys.lists() })
    },
  })
}

export function useUpdateTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateTemplateInput }) =>
      updateTemplate(id, input),
    onSettled: (_data, _err, vars) => {
      void qc.invalidateQueries({ queryKey: templateKeys.lists() })
      void qc.invalidateQueries({ queryKey: templateKeys.detail(vars.id) })
    },
  })
}

export function useDeleteTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => deleteTemplate(id),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: templateKeys.lists() })
    },
  })
}

// useApplyTemplate invalidates all week-rooted caches so the planner grid,
// shopping list, and nutrition views all refetch after the plate changes.
export function useApplyTemplate(weekId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: ApplyTemplateInput }) =>
      applyTemplate(id, input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: weekKeys.all })
      void qc.invalidateQueries({ queryKey: weekKeys.shoppingList(weekId) })
      void qc.invalidateQueries({ queryKey: weekKeys.nutrition(weekId) })
    },
  })
}
