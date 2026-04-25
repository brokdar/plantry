import { ApiError, apiFetch } from "./client"

export async function uploadFoodImage(
  id: number,
  file: Blob
): Promise<{ image_path: string }> {
  const formData = new FormData()
  formData.append("image", file, "image.jpg")
  const res = await fetch(`/api/foods/${id}/image`, {
    method: "POST",
    body: formData,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new ApiError(res.status, body.message_key || "error.server")
  }
  return res.json()
}

export function deleteFoodImage(id: number): Promise<void> {
  return apiFetch(`/foods/${id}/image`, { method: "DELETE" })
}

export async function fetchImageFromUrl(url: string): Promise<Blob> {
  const res = await fetch("/api/image/fetch-url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new ApiError(
      res.status,
      body.message_key || "error.image.url_fetch_failed"
    )
  }
  return res.blob()
}
