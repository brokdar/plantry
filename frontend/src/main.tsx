import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { QueryClientProvider } from "@tanstack/react-query"
import { RouterProvider } from "@tanstack/react-router"

import "./index.css"
import i18n from "./lib/i18n"
import { configureZodI18n } from "./lib/i18n/zod"

configureZodI18n(i18n)

import { ErrorBoundary } from "@/components/ErrorBoundary"
import { ThemeProvider } from "@/components/theme-provider"
import { queryClient } from "@/lib/query-client"
import { router } from "./router"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <ThemeProvider defaultTheme="light">
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ThemeProvider>
    </ErrorBoundary>
  </StrictMode>
)
