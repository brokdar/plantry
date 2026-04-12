import { ApiError } from "./client"
import { apiFetch } from "./client"

export async function uploadImage(
  ingredientId: number,
  file: File
): Promise<{ image_path: string }> {
  const formData = new FormData()
  formData.append("image", file)
  const res = await fetch(`/api/ingredients/${ingredientId}/image`, {
    method: "POST",
    body: formData,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new ApiError(res.status, body.message_key || "error.server")
  }
  return res.json()
}

export function deleteImage(ingredientId: number): Promise<void> {
  return apiFetch(`/ingredients/${ingredientId}/image`, { method: "DELETE" })
}
