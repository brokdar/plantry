import { describe, expect, test, vi, beforeEach } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithRouter } from "@/test/render"
import { ImageUpload } from "./ImageUpload"

vi.mock("@/lib/api/images", () => ({
  uploadImage: vi.fn(),
  deleteImage: vi.fn(),
}))

import { uploadImage, deleteImage } from "@/lib/api/images"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("ImageUpload", () => {
  test("shows upload button when no image", async () => {
    const onImageChange = vi.fn()
    renderWithRouter(
      <ImageUpload
        ingredientId={1}
        currentImagePath={null}
        onImageChange={onImageChange}
      />
    )

    expect(await screen.findByText("Upload image")).toBeInTheDocument()
  })

  test("shows image when currentImagePath is provided", async () => {
    const onImageChange = vi.fn()
    renderWithRouter(
      <ImageUpload
        ingredientId={1}
        currentImagePath="chicken.jpg"
        onImageChange={onImageChange}
      />
    )

    await screen.findByText("Remove image")
    const img = screen.getByAltText("")
    expect(img).toBeTruthy()
    expect(img).toHaveAttribute("src", "/images/chicken.jpg")

    // Should also show remove button
    expect(screen.getByText("Remove image")).toBeInTheDocument()
  })

  test("file input triggers upload", async () => {
    const user = userEvent.setup()
    const onImageChange = vi.fn()
    vi.mocked(uploadImage).mockResolvedValue({ image_path: "new-image.jpg" })

    renderWithRouter(
      <ImageUpload
        ingredientId={1}
        currentImagePath={null}
        onImageChange={onImageChange}
      />
    )

    await screen.findByText("Upload image")

    const file = new File(["image content"], "test.jpg", {
      type: "image/jpeg",
    })
    const input = document.querySelector(
      'input[type="file"]'
    ) as HTMLInputElement
    expect(input).toBeTruthy()

    await user.upload(input, file)

    expect(uploadImage).toHaveBeenCalledWith(1, file)
    expect(onImageChange).toHaveBeenCalledWith("new-image.jpg")
  })

  test("delete button removes image", async () => {
    const user = userEvent.setup()
    const onImageChange = vi.fn()
    vi.mocked(deleteImage).mockResolvedValue(undefined)

    renderWithRouter(
      <ImageUpload
        ingredientId={1}
        currentImagePath="chicken.jpg"
        onImageChange={onImageChange}
      />
    )

    const removeButton = await screen.findByText("Remove image")
    await user.click(removeButton)

    expect(deleteImage).toHaveBeenCalledWith(1)
    expect(onImageChange).toHaveBeenCalledWith(null)
  })

  test("shows error message on upload failure", async () => {
    const user = userEvent.setup()
    vi.mocked(uploadImage).mockRejectedValue(new Error("Upload failed"))

    renderWithRouter(
      <ImageUpload
        ingredientId={1}
        currentImagePath={null}
        onImageChange={vi.fn()}
      />
    )

    await screen.findByText("Upload image")

    const file = new File(["content"], "test.jpg", { type: "image/jpeg" })
    const input = document.querySelector(
      'input[type="file"]'
    ) as HTMLInputElement
    expect(input).toBeTruthy()

    await user.upload(input, file)

    expect(
      await screen.findByText(/image operation failed/i)
    ).toBeInTheDocument()
  })

  test("shows error message on delete failure", async () => {
    const user = userEvent.setup()
    vi.mocked(deleteImage).mockRejectedValue(new Error("Delete failed"))

    renderWithRouter(
      <ImageUpload
        ingredientId={1}
        currentImagePath="chicken.jpg"
        onImageChange={vi.fn()}
      />
    )

    const removeButton = await screen.findByText("Remove image")
    await user.click(removeButton)

    expect(
      await screen.findByText(/image operation failed/i)
    ).toBeInTheDocument()
  })
})
