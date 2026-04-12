const BASE = "/api"

export class ApiError extends Error {
  status: number
  messageKey: string

  constructor(status: number, messageKey: string) {
    super(messageKey)
    this.name = "ApiError"
    this.status = status
    this.messageKey = messageKey
  }
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit
): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({
      message_key: "error.server",
      status: res.status,
    }))
    throw new ApiError(body.status, body.message_key)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}
