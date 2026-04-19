import { createFileRoute, useNavigate } from "@tanstack/react-router"

import { PickerPage } from "@/components/picker/PickerPage"

export const Route = createFileRoute("/planner/$weekId/$day/$slotId/pick")({
  component: PickerRoute,
})

function PickerRoute() {
  const { weekId, day, slotId } = Route.useParams()
  const navigate = useNavigate()
  return (
    <PickerPage
      weekId={Number(weekId)}
      day={Number(day)}
      slotId={Number(slotId)}
      onBack={() => void navigate({ to: "/" })}
    />
  )
}
