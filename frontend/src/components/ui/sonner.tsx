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
            "editorial-shadow rounded-xl border border-outline-variant/20 bg-surface-container-highest text-on-surface",
          title: "font-heading font-bold text-on-surface",
          description: "text-on-surface-variant",
          actionButton: "bg-primary text-on-primary",
          cancelButton: "bg-surface-container-high text-on-surface",
          closeButton:
            "bg-surface-container-high text-on-surface-variant hover:bg-surface-container-highest",
        },
      }}
      {...props}
    />
  )
}
