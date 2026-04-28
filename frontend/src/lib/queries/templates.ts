import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  applyTemplate,
  createTemplate,
  createTemplateFromRange,
  deleteTemplate,
  getTemplate,
  getTemplates,
  updateTemplate,
  type ApplyTemplateInput,
  type CreateTemplateFromRangeInput,
  type CreateTemplateInput,
  type UpdateTemplateInput,
} from "@/lib/api/templates"

import { plateKeys, templateKeys } from "./keys"

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

// useApplyTemplate accepts {templateId, input} and invalidates all plate caches
// so the planner grid refetches after applying the template.
export function useApplyTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      templateId,
      input,
    }: {
      templateId: number
      input: ApplyTemplateInput
    }) => applyTemplate(templateId, input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: plateKeys.all })
    },
  })
}

export function useCreateTemplateFromRange() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateTemplateFromRangeInput) =>
      createTemplateFromRange(input),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: templateKeys.lists() })
    },
  })
}
