import { z } from "zod"

export const presetNameSchema = z.string().trim().min(1)
