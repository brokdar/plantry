import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import { defineConfig } from "vitest/config"

export default defineConfig({
  plugins: [
    tanstackRouter({
      target: "react",
      autoCodeSplitting: true,
      routeFileIgnorePattern: "\\.test\\.",
    }),
    react(),
    tailwindcss(),
  ],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
      "/images": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return
          if (/[\\/](react|react-dom|scheduler)[\\/]/.test(id))
            return "vendor-react"
          if (/[\\/]@tanstack[\\/]/.test(id)) return "vendor-tanstack"
          if (/[\\/]@?radix-ui[\\/]/.test(id)) return "vendor-radix"
          if (/[\\/](i18next|react-i18next)[\\/]/.test(id)) return "vendor-i18n"
          if (
            /[\\/](react-markdown|remark-|micromark|mdast-|hast-|unist-|unified|vfile|bail|trough|devlop|estree-|ccount|character-|decode-named|html-void|space-separated|comma-separated|property-information|web-namespaces|zwitch)[\\/]/.test(
              id
            )
          )
            return "vendor-markdown"
          if (/[\\/](cmdk|sonner)[\\/]/.test(id)) return "vendor-overlays"
          if (/[\\/](barcode-detector|@zxing)[\\/]/.test(id))
            return "vendor-barcode"
          if (/[\\/]react-easy-crop[\\/]/.test(id)) return "vendor-crop"
          if (/[\\/]@dnd-kit[\\/]/.test(id)) return "vendor-dnd"
          if (/[\\/](react-hook-form|@hookform|zod)[\\/]/.test(id))
            return "vendor-forms"
          if (/[\\/]date-fns[\\/]/.test(id)) return "vendor-date-fns"
        },
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: true,
    exclude: ["**/node_modules/**", "**/e2e/**", "**/dist/**"],
  },
})
