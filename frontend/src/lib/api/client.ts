const BASE = "/api"

export class ApiError extends Error {
  status: number
  messageKey: string
  /** Set on a duplicate-name 409: the id of the food that already exists. */
  existingId?: number
  /** Interpolation values of the message, e.g. `{ host }` of an import error. */
  params?: Record<string, string>

  constructor(
    status: number,
    messageKey: string,
    existingId?: number,
    params?: Record<string, string>
  ) {
    super(messageKey)
    this.name = "ApiError"
    this.status = status
    this.messageKey = messageKey
    this.existingId = existingId
    this.params = params
  }
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit
): Promise<T> {
  let res: Response
  try {
    res = await fetch(`${BASE}${path}`, {
      ...init,
      headers: {
        ...(init?.body ? { "Content-Type": "application/json" } : {}),
        ...init?.headers,
      },
    })
  } catch (err) {
    // An abort is the caller's own doing; anything else is the network.
    if (err instanceof DOMException && err.name === "AbortError") throw err
    throw new ApiError(0, "error.network")
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({
      message_key: "error.server",
      status: res.status,
    }))
    throw new ApiError(
      body.status,
      body.message_key,
      body.existing_id,
      body.params
    )
  }
  if (res.status === 204) return undefined as T
  return res.json()
}
