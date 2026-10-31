import { useEffect, useState } from "react"
import { Toaster as SonnerToaster, type ToasterProps } from "sonner"

type Theme = "light" | "dark"

function getInitialTheme(): Theme {
  if (typeof document === "undefined") return "light"
  return document.documentElement.classList.contains("dark") ? "dark" : "light"
}

export function Toaster(props: ToasterProps) {
  const [theme, setTheme] = useState<Theme>(getInitialTheme)

  useEffect(() => {
    const observer = new MutationObserver(() => {
      setTheme(
        document.documentElement.classList.contains("dark") ? "dark" : "light"
      )
    })
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    })
    return () => observer.disconnect()
  }, [])

  return (
    <SonnerToaster
      theme={theme}
      position="top-right"
      richColors
      closeButton
      toastOptions={{
        classNames: {
          toast:
            "rounded-xl border border-line/20 bg-line text-ink shadow-sheet",
          title: "font-semibold text-ink",
          description: "text-ink-muted",
          actionButton: "bg-primary text-on-accent",
          cancelButton: "bg-line text-ink",
          closeButton: "bg-line text-ink-muted hover:text-ink",
        },
      }}
      {...props}
    />
  )
}
