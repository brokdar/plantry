import { useQuery, useMutation } from "@tanstack/react-query"
import {
  listComponents,
  getComponent,
  createComponent,
  updateComponent,
  deleteComponent,
  getComponentNutrition,
  createVariant,
  listVariants,
  type ComponentListParams,
  type ComponentInput,
} from "@/lib/api/components"
import { queryClient } from "@/lib/query-client"
import { componentKeys } from "./keys"

export function useComponents(params?: ComponentListParams) {
  return useQuery({
    queryKey: componentKeys.list(params ?? {}),
    queryFn: () => listComponents(params),
  })
}

export function useComponent(id: number) {
  return useQuery({
    queryKey: componentKeys.detail(id),
    queryFn: () => getComponent(id),
    enabled: id > 0,
  })
}

export function useComponentNutrition(id: number) {
  return useQuery({
    queryKey: componentKeys.nutrition(id),
    queryFn: () => getComponentNutrition(id),
    enabled: id > 0,
  })
}

export function useCreateComponent() {
  return useMutation({
    mutationFn: (input: ComponentInput) => createComponent(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: componentKeys.lists() })
    },
  })
}

export function useUpdateComponent() {
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: ComponentInput }) =>
      updateComponent(id, data),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: componentKeys.lists() })
      void queryClient.invalidateQueries({
        queryKey: componentKeys.detail(variables.id),
      })
    },
  })
}

export function useDeleteComponent() {
  return useMutation({
    mutationFn: (id: number) => deleteComponent(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: componentKeys.lists() })
    },
  })
}

export function useVariants(id: number) {
  return useQuery({
    queryKey: componentKeys.variants(id),
    queryFn: () => listVariants(id),
    enabled: id > 0,
  })
}

export function useCreateVariant() {
  return useMutation({
    mutationFn: (id: number) => createVariant(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({
        queryKey: componentKeys.variants(id),
      })
      void queryClient.invalidateQueries({
        queryKey: componentKeys.detail(id),
      })
      void queryClient.invalidateQueries({ queryKey: componentKeys.lists() })
    },
  })
}
