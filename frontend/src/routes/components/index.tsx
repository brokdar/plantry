import { createFileRoute } from "@tanstack/react-router"
import { ComponentList } from "@/components/components/ComponentList"

export const Route = createFileRoute("/components/")({
  component: ComponentsPage,
})

function ComponentsPage() {
  return <ComponentList />
}
