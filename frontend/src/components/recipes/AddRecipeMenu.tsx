import { Link } from "@tanstack/react-router"
import { ChevronDown, Link2, PencilLine, Plus } from "lucide-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

/** The primary Add action of the library (UX §5.8): import from a link
 *  (the sheet at `?add=url`) or create manually. Discover is Phase 5. */
export function AddRecipeMenu() {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          className="min-h-11 md:pointer-fine:min-h-8"
          data-testid="add-recipe"
        >
          <Plus data-icon="inline-start" strokeWidth={1.5} aria-hidden />
          {t("recipes.add.label")}
          <ChevronDown data-icon="inline-end" strokeWidth={1.5} aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem asChild>
          <Link
            to="/recipes"
            search={(prev) => ({ ...prev, add: "url" as const })}
            data-testid="add-recipe-import"
          >
            <Link2 strokeWidth={1.5} aria-hidden />
            {t("recipes.add.import")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/recipes/new" data-testid="add-recipe-create">
            <PencilLine strokeWidth={1.5} aria-hidden />
            {t("recipes.add.create")}
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
