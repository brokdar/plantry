import { useState, useDeferredValue } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
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
import { useIngredients, useDeleteIngredient } from "@/lib/queries/ingredients"

const PAGE_SIZE = 20

export function IngredientList() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [search, setSearch] = useState("")
  const deferredSearch = useDeferredValue(search)
  const [offset, setOffset] = useState(0)
  const [deleteId, setDeleteId] = useState<number | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const { data, isLoading } = useIngredients({
    search: deferredSearch || undefined,
    limit: PAGE_SIZE,
    offset,
  })

  const deleteMutation = useDeleteIngredient()

  function handleDelete() {
    if (deleteId === null) return
    deleteMutation.mutate(deleteId, {
      onSuccess: () => setDeleteId(null),
      onError: (err: unknown) => {
        const key = err instanceof Error ? err.message : "error.server"
        setDeleteError(t(key))
      },
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
          {t("ingredient.title")}
        </h1>
        <Button asChild>
          <Link to="/ingredients/new">{t("ingredient.create")}</Link>
        </Button>
      </div>

      <Input
        placeholder={t("ingredient.search_placeholder")}
        value={search}
        onChange={(e) => {
          setSearch(e.target.value)
          setOffset(0)
        }}
        className="max-w-sm"
      />

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <p className="py-12 text-center text-muted-foreground">
          {deferredSearch
            ? t("ingredient.no_results")
            : t("ingredient.empty_state")}
        </p>
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("ingredient.name")}</TableHead>
                <TableHead className="text-right">
                  {t("ingredient.kcal")}
                </TableHead>
                <TableHead className="text-right">
                  {t("ingredient.protein")}
                </TableHead>
                <TableHead className="text-right">
                  {t("ingredient.fat")}
                </TableHead>
                <TableHead className="text-right">
                  {t("ingredient.carbs")}
                </TableHead>
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
                      to: "/ingredients/$id/edit",
                      params: { id: String(item.id) },
                    })
                  }
                >
                  <TableCell className="font-medium">{item.name}</TableCell>
                  <TableCell className="text-right">{item.kcal_100g}</TableCell>
                  <TableCell className="text-right">
                    {item.protein_100g}
                  </TableCell>
                  <TableCell className="text-right">{item.fat_100g}</TableCell>
                  <TableCell className="text-right">
                    {item.carbs_100g}
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

      <Dialog
        open={deleteId !== null}
        onOpenChange={() => {
          setDeleteId(null)
          setDeleteError(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("ingredient.delete_confirm_title")}</DialogTitle>
            <DialogDescription>
              {t("ingredient.delete_confirm_body")}
            </DialogDescription>
          </DialogHeader>
          {deleteError && (
            <p className="px-1 text-sm text-destructive">{deleteError}</p>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setDeleteId(null)
                setDeleteError(null)
              }}
            >
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
