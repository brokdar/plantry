import { clsx, type ClassValue } from "clsx"
import { extendTailwindMerge } from "tailwind-merge"

// The Cookbook type scale (`--text-*` in index.css @theme). Without this,
// tailwind-merge reads `text-small` etc. as colours and drops either the size
// or the colour when both appear in one cn() call.
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      "font-size": [{ text: ["caption", "small", "body", "h3", "h2", "h1"] }],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
