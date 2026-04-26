import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useEffect } from "react"

export const Route = createFileRoute("/archive/")({
  component: ArchiveRedirect,
})

function ArchiveRedirect() {
  const navigate = useNavigate()
  useEffect(() => {
    void navigate({
      to: "/calendar",
      search: { mode: "agenda" as const, edit: false, search: "" },
      replace: true,
    })
  }, [navigate])
  return null
}
