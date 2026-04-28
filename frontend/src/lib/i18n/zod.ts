import { z } from "zod"
import type { i18n as I18nInstance } from "i18next"

type Origin = "string" | "number" | "bigint" | "array" | "set" | "date" | "file"

function translateOrigin(
  t: I18nInstance["t"],
  origin: Origin | string
): string {
  return t(`validation.origin.${origin}`, {
    defaultValue: origin,
  })
}

export function configureZodI18n(i18n: I18nInstance) {
  const t = i18n.t.bind(i18n)

  function customError(issue: z.core.$ZodRawIssue): string | undefined {
    switch (issue.code) {
      case "too_small": {
        const origin = (issue as { origin?: Origin }).origin ?? "string"
        const minimum = (issue as { minimum?: number | bigint }).minimum
        return t(`validation.too_small.${origin}`, {
          defaultValue: t("validation.too_small.default", {
            minimum: String(minimum),
            defaultValue: `Must be at least ${String(minimum)}`,
          }),
          minimum: String(minimum),
        })
      }
      case "too_big": {
        const origin = (issue as { origin?: Origin }).origin ?? "string"
        const maximum = (issue as { maximum?: number | bigint }).maximum
        return t(`validation.too_big.${origin}`, {
          defaultValue: t("validation.too_big.default", {
            maximum: String(maximum),
            defaultValue: `Must be at most ${String(maximum)}`,
          }),
          maximum: String(maximum),
        })
      }
      case "invalid_type": {
        const expected = (issue as { expected?: string }).expected
        if (
          expected === "string" ||
          expected === "number" ||
          expected === "boolean"
        ) {
          return t(`validation.required`, {
            defaultValue: "This field is required",
          })
        }
        return t("validation.invalid_type", {
          expected: translateOrigin(t, expected ?? "value"),
          defaultValue: `Invalid value — expected ${expected}`,
        })
      }
      case "invalid_value": {
        return t("validation.invalid_value", {
          defaultValue: "Please choose a valid option",
        })
      }
      case "invalid_format": {
        const format = (issue as { format?: string }).format ?? "value"
        return t(`validation.invalid_format.${format}`, {
          defaultValue: t("validation.invalid_format.default", {
            format,
            defaultValue: `Invalid ${format}`,
          }),
        })
      }
      case "not_multiple_of": {
        const divisor = (issue as { divisor?: number }).divisor
        return t("validation.not_multiple_of", {
          divisor: String(divisor ?? ""),
          defaultValue: `Must be a multiple of ${String(divisor ?? "")}`,
        })
      }
      case "unrecognized_keys": {
        return t("validation.unrecognized_keys", {
          defaultValue: "Unexpected fields",
        })
      }
      case "invalid_union":
      case "invalid_key":
      case "invalid_element": {
        return t("validation.invalid_value", {
          defaultValue: "Invalid value",
        })
      }
      case "custom": {
        const msg = (issue as { message?: string }).message
        if (msg) return t(msg, { defaultValue: msg })
        return undefined
      }
      default:
        return undefined
    }
  }

  z.config({ customError })

  i18n.on("languageChanged", () => {
    z.config({ customError })
  })
}
