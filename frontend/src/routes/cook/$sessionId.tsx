import { createFileRoute } from "@tanstack/react-router"

import { CookSessionPage } from "@/components/cook/CookPages"

export const Route = createFileRoute("/cook/$sessionId")({
  component: CookSessionRoute,
})

function CookSessionRoute() {
  const { sessionId } = Route.useParams()
  return <CookSessionPage sessionId={Number(sessionId)} />
}
