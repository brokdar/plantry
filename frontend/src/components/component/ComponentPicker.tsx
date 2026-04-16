import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { Component } from "@/lib/api/components"
import { useComponents } from "@/lib/queries/components"

interface ComponentPickerProps {
  defaultRole?: string
  onPick: (c: Component) => void
}

const ROLES = [
  "main",
  "side_starch",
  "side_veg",
  "side_protein",
  "sauce",
  "drink",
  "dessert",
  "standalone",
] as const

export function ComponentPicker({ defaultRole, onPick }: ComponentPickerProps) {
  const { t } = useTranslation()
  const [role, setRole] = useState<string>(defaultRole ?? "")
  const [search, setSearch] = useState("")

  const query = useComponents({
    role: role || undefined,
    search: search || undefined,
    limit: 50,
  })

  const items = query.data?.items ?? []

  return (
    <div className="flex flex-col gap-3">
      <div className="flex gap-2">
        <Input
          placeholder={t("component.search_placeholder")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label={t("component.search_placeholder")}
        />
        <Select
          value={role || "__all__"}
          onValueChange={(v) => setRole(v === "__all__" ? "" : v)}
        >
          <SelectTrigger
            className="w-44"
            aria-label={t("plate.filter_by_role")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t("component.all_roles")}</SelectItem>
            {ROLES.map((r) => (
              <SelectItem key={r} value={r}>
                {t(`component.role_${r}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="max-h-80 overflow-auto" role="list">
        {query.isLoading ? (
          <p className="p-4 text-sm text-muted-foreground">
            {t("common.loading")}
          </p>
        ) : items.length === 0 ? (
          <p className="p-4 text-sm text-muted-foreground">
            {t("component.no_results")}
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {items.map((c) => (
              <li key={c.id} role="listitem">
                <Button
                  variant="ghost"
                  className="w-full justify-start"
                  onClick={() => onPick(c)}
                >
                  <span className="font-medium">{c.name}</span>
                  <span className="ml-2 text-xs text-muted-foreground">
                    {t(`component.role_${c.role}`)}
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
