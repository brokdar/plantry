import { useState, useDeferredValue } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useComponents, useDeleteComponent } from "@/lib/queries/components"
import { COMPONENT_ROLES } from "@/lib/schemas/component"

const PAGE_SIZE = 20

export function ComponentList() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [roleFilter, setRoleFilter] = useState("")
  const [offset, setOffset] = useState(0)
  const [deleteId, setDeleteId] = useState<number | null>(null)

  const { data, isLoading } = useComponents({
    search: deferredSearch || undefined,
    role: roleFilter || undefined,
    limit: PAGE_SIZE,
    offset,
  })

  const deleteMutation = useDeleteComponent()

  function handleDelete() {
    if (deleteId === null) return
    deleteMutation.mutate(deleteId, {
      onSuccess: () => setDeleteId(null),
    })
  }

  const total = data?.total ?? 0
  const items = data?.items ?? []
  const from = total > 0 ? offset + 1 : 0
  const to = Math.min(offset + PAGE_SIZE, total)

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("component.title")}
        </h1>
        <Button asChild>
          <Link to="/components/new">{t("component.create")}</Link>
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Input
          placeholder={t("component.search_placeholder")}
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setOffset(0)
          }}
          className="max-w-sm"
        />
        <Select
          value={roleFilter}
          onValueChange={(v) => {
            setRoleFilter(v === "all" ? "" : v)
            setOffset(0)
          }}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder={t("component.all_roles")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("component.all_roles")}</SelectItem>
            {COMPONENT_ROLES.map((role) => (
              <SelectItem key={role} value={role}>
                {t(`component.role_${role}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <p className="py-12 text-center text-muted-foreground">
          {deferredSearch || roleFilter
            ? t("component.no_results")
            : t("component.empty_state")}
        </p>
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("component.name")}</TableHead>
                <TableHead>{t("component.role")}</TableHead>
                <TableHead className="text-right">
                  {t("component.reference_portions")}
                </TableHead>
                <TableHead>{t("component.tags")}</TableHead>
                <TableHead className="w-[60px]">
                  <span className="sr-only">{t("common.actions")}</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow
                  key={item.id}
                  className="cursor-pointer"
                  onClick={() =>
                    navigate({
                      to: "/components/$id",
                      params: { id: String(item.id) },
                    })
                  }
                >
                  <TableCell className="font-medium">{item.name}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">
                      {t(`component.role_${item.role}`)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    {item.reference_portions}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {item.tags.map((tag) => (
                        <Badge key={tag} variant="outline">
                          {tag}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={(e) => {
                        e.stopPropagation()
                        setDeleteId(item.id)
                      }}
                    >
                      <Trash2 className="size-4" />
                      <span className="sr-only">{t("common.delete")}</span>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          <div className="flex items-center justify-between">
            <p className="text-sm text-muted-foreground">
              {t("common.showing", { from, to, total })}
            </p>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={offset === 0}
                onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
              >
                {t("common.previous")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={offset + PAGE_SIZE >= total}
                onClick={() => setOffset((o) => o + PAGE_SIZE)}
              >
                {t("common.next")}
              </Button>
            </div>
          </div>
        </>
      )}

      <Dialog open={deleteId !== null} onOpenChange={() => setDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("component.delete_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("component.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteId(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteMutation.isPending}
            >
              {t("common.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
