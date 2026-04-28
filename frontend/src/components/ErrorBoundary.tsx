import { Component, type ErrorInfo, type ReactNode } from "react"
import i18n from "@/lib/i18n"

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[ErrorBoundary]", error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      return (
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            height: "100dvh",
            gap: "1rem",
            fontFamily: "sans-serif",
            padding: "2rem",
            textAlign: "center",
            color: "#191c1c",
          }}
        >
          <p style={{ fontSize: "2.5rem", fontWeight: 700, opacity: 0.2 }}>!</p>
          <h1 style={{ fontSize: "1.25rem", fontWeight: 700 }}>
            {i18n.t("error.server")}
          </h1>
          <p style={{ fontSize: "0.875rem", color: "#424842" }}>
            {i18n.t("error.page_error_body")}
          </p>
          <button
            onClick={() => window.location.reload()}
            style={{
              padding: "0.5rem 1.25rem",
              background: "#4a654d",
              color: "#fff",
              border: "none",
              borderRadius: "0.5rem",
              cursor: "pointer",
              fontSize: "0.875rem",
            }}
          >
            {i18n.t("error.reload")}
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
