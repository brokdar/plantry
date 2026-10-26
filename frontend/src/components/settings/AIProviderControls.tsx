import { Check, Loader2 } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { ApiError } from "@/lib/api/client"
import { listAIModels } from "@/lib/api/settings"
import {
  useClearSetting,
  useSetSetting,
  useSettingItems,
} from "@/lib/queries/settings"
import { cn } from "@/lib/utils"

import { useSavedNote } from "./setting-field"
import { SettingRow } from "./SettingRow"

const KEY_PROVIDER = "ai.provider"
const KEY_API_KEY = "ai.api_key"
const KEY_MODEL = "ai.model"

type ModelOption = { id: string; display_name?: string }

/**
 * The assistant's provider → key → model steps. Each choice saves at once
 * (the key on its Save button). Reused by Settings → Assistant and the setup
 * wizard's assistant step.
 */
export function AIProviderControls({
  variant,
}: {
  /** "wizard" (the setup wizard's narrow column, under its h1): each label
   *  above its control, h2 step titles, and none of the Settings-only details
   *  (source badges, clear override, the test provider, key-storage
   *  internals). By default rows go side by side on wide screens and the step
   *  titles are h4 under the Settings group's h3. */
  variant?: "wizard"
} = {}) {
  const { data: items } = useSettingItems()
  if (!items) return null
  return <ProviderSteps items={items} wizard={variant === "wizard"} />
}

/** The inline result of a successful key check (plan D17). Anthropic keys
 *  aren't really checked until Phase 5, so that copy only says it's saved. */
function validatedKey(provider: string): string {
  return provider === "anthropic"
    ? "settings_page.ai.validated_saved"
    : "settings_page.ai.validated_works"
}

function ProviderSteps({
  items,
  wizard,
}: {
  items: NonNullable<ReturnType<typeof useSettingItems>["data"]>
  wizard: boolean
}) {
  const { t } = useTranslation()
  const providerItem = items[KEY_PROVIDER]
  const keyItem = items[KEY_API_KEY]
  const modelItem = items[KEY_MODEL]

  const [provider, setProvider] = useState(providerItem?.value ?? "")
  const [apiKeyDraft, setApiKeyDraft] = useState("")
  const [models, setModels] = useState<ModelOption[]>([])
  const [model, setModel] = useState(modelItem?.value ?? "")
  const [validating, setValidating] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)
  const [validated, setValidated] = useState(false)

  const rowClass = wizard ? "lg:grid-cols-1 lg:gap-3" : undefined
  const heading = wizard ? "h2" : "h4"
  /** Settings shows where a value comes from and lets an override go. */
  const source = (item: typeof providerItem, key: string) =>
    wizard
      ? {}
      : {
          item,
          onClearOverride:
            item?.source === "db" ? () => clearSetting.mutate(key) : undefined,
        }
  const providerNote = useSavedNote()
  const keyNote = useSavedNote()
  const modelNote = useSavedNote()

  const setSetting = useSetSetting()
  const clearSetting = useClearSetting()

  // Sync local state with upstream data whenever the settings query
  // refetches. Without this, clearing an override elsewhere doesn't
  // propagate back into the pre-filled inputs.
  useEffect(() => {
    setProvider(providerItem?.value ?? "")
  }, [providerItem?.value, providerItem?.source])

  useEffect(() => {
    setModel(modelItem?.value ?? "")
  }, [modelItem?.value, modelItem?.source])

  const hasStoredKey =
    !!keyItem && (keyItem.source === "db" || keyItem.source === "env")
  const step1Complete = !!provider
  const step2Complete = validated || (hasStoredKey && !apiKeyDraft)
  const canValidate = step1Complete && (apiKeyDraft.length > 0 || hasStoredKey)

  async function validate() {
    if (!provider) return
    setValidating(true)
    setValidationError(null)
    try {
      const res = await listAIModels(provider, apiKeyDraft || undefined)
      setModels(res.models)
      setValidated(true)
    } catch (err) {
      const key =
        err instanceof ApiError
          ? err.messageKey
          : "error.settings.invalid_api_key"
      setValidationError(key)
      setValidated(false)
      setModels([])
    } finally {
      setValidating(false)
    }
  }

  function handleProviderChange(next: string) {
    setProvider(next)
    setModels([])
    setValidated(false)
    setValidationError(null)
    const onSuccess = providerNote.markSaved
    if (next === "") clearSetting.mutate(KEY_PROVIDER, { onSuccess })
    else setSetting.mutate({ key: KEY_PROVIDER, value: next }, { onSuccess })
  }

  function saveKey() {
    if (!apiKeyDraft) return
    setSetting.mutate(
      { key: KEY_API_KEY, value: apiKeyDraft },
      {
        onSuccess: () => {
          setApiKeyDraft("")
          keyNote.markSaved()
        },
      }
    )
  }

  function saveModel(next: string) {
    setModel(next)
    setSetting.mutate(
      { key: KEY_MODEL, value: next },
      { onSuccess: modelNote.markSaved }
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <Step
        heading={heading}
        index={1}
        title={t("settings_page.ai.wizard.step1_title")}
        description={t("settings_page.ai.wizard.step1_description")}
        complete={step1Complete}
      >
        <SettingRow
          className={rowClass}
          label={t("settings_page.ai.provider_label")}
          {...source(providerItem, KEY_PROVIDER)}
          saved={providerNote.saved}
        >
          <Select
            value={provider || "none"}
            onValueChange={(v) => handleProviderChange(v === "none" ? "" : v)}
          >
            <SelectTrigger
              className="w-full pointer-coarse:h-11"
              data-testid="ai-provider-select"
              aria-label={t("settings_page.ai.provider_label")}
            >
              <SelectValue
                placeholder={t("settings_page.ai.provider_placeholder")}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="none">
                  {t("settings_page.ai.provider_none")}
                </SelectItem>
                <SelectItem value="openai">OpenAI</SelectItem>
                <SelectItem value="anthropic">Anthropic</SelectItem>
                {(!wizard || provider === "fake") && (
                  <SelectItem value="fake">
                    {t("settings_page.ai.provider_fake")}
                  </SelectItem>
                )}
              </SelectGroup>
            </SelectContent>
          </Select>
        </SettingRow>
      </Step>

      <Step
        heading={heading}
        index={2}
        title={t("settings_page.ai.wizard.step2_title")}
        description={t("settings_page.ai.wizard.step2_description")}
        complete={step2Complete}
        disabled={!step1Complete}
      >
        <SettingRow
          className={rowClass}
          label={t("settings_page.ai.api_key_label")}
          description={
            hasStoredKey
              ? t("settings_page.ai.api_key_stored", {
                  preview: keyItem?.masked_preview ?? "",
                })
              : t(
                  wizard
                    ? "setup.assistant.api_key_description"
                    : "settings_page.ai.api_key_description"
                )
          }
          {...source(keyItem, KEY_API_KEY)}
          saved={keyNote.saved}
          htmlFor="ai-api-key"
        >
          <div className="flex flex-col gap-3 sm:flex-row">
            <Input
              id="ai-api-key"
              type="password"
              autoComplete="off"
              placeholder={
                hasStoredKey
                  ? t("settings_page.ai.api_key_placeholder_replace")
                  : t("settings_page.ai.api_key_placeholder")
              }
              value={apiKeyDraft}
              onChange={(e) => {
                setApiKeyDraft(e.target.value)
                setValidated(false)
              }}
              disabled={!step1Complete}
              className="pointer-coarse:h-11"
              data-testid="ai-api-key-input"
            />
            <Button
              type="button"
              variant="outline"
              onClick={validate}
              disabled={!canValidate || validating}
              className="pointer-coarse:h-11"
              data-testid="ai-test-connection"
            >
              {validating && (
                <Loader2
                  data-icon="inline-start"
                  className="animate-spin"
                  aria-hidden
                />
              )}
              {t("settings_page.ai.test_connection")}
            </Button>
            {apiKeyDraft && (
              <Button
                type="button"
                onClick={saveKey}
                disabled={setSetting.isPending}
                className="pointer-coarse:h-11"
                data-testid="ai-save-api-key"
              >
                {t("common.save")}
              </Button>
            )}
          </div>
          {validationError && (
            <p role="alert" className="text-small text-danger">
              {t(validationError)}
            </p>
          )}
          {validated && (
            <p
              role="status"
              className="inline-flex items-center gap-1 text-small text-success"
            >
              <Check className="size-4" strokeWidth={1.5} aria-hidden />
              {t(validatedKey(provider), { count: models.length })}
            </p>
          )}
        </SettingRow>
      </Step>

      <Step
        heading={heading}
        index={3}
        title={t("settings_page.ai.wizard.step3_title")}
        description={t("settings_page.ai.wizard.step3_description")}
        complete={!!model && step2Complete}
        disabled={!step2Complete}
      >
        <SettingRow
          className={rowClass}
          label={t("settings_page.ai.model_label")}
          {...source(modelItem, KEY_MODEL)}
          saved={modelNote.saved}
        >
          <Select
            value={model || ""}
            onValueChange={saveModel}
            disabled={!step2Complete || models.length === 0}
          >
            <SelectTrigger
              className="w-full pointer-coarse:h-11"
              data-testid="ai-model-select"
              aria-label={t("settings_page.ai.model_label")}
            >
              <SelectValue
                placeholder={t("settings_page.ai.model_placeholder")}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {model && !models.some((m) => m.id === model) && (
                  <SelectItem value={model}>{model}</SelectItem>
                )}
                {models.map((m) => (
                  <SelectItem key={m.id} value={m.id}>
                    {m.display_name || m.id}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          {models.length === 0 && step2Complete && (
            <p className="text-small text-ink-muted">
              {t("settings_page.ai.click_test_to_load_models")}
            </p>
          )}
        </SettingRow>
      </Step>
    </div>
  )
}

function Step({
  heading: Heading,
  index,
  title,
  description,
  complete,
  disabled,
  children,
}: {
  heading: "h2" | "h4"
  index: number
  title: string
  description: string
  complete: boolean
  disabled?: boolean
  children: React.ReactNode
}) {
  return (
    <section
      aria-disabled={disabled}
      className={cn(
        "grid gap-3 md:grid-cols-[auto_minmax(0,1fr)] md:gap-4",
        disabled && "opacity-60"
      )}
      data-testid={`ai-wizard-step-${index}`}
    >
      <span
        aria-hidden
        className={cn(
          "flex size-7 items-center justify-center rounded-full text-small font-semibold tabular-nums",
          complete
            ? "bg-accent text-on-accent"
            : "border border-line text-ink-muted"
        )}
      >
        {complete ? <Check className="size-4" strokeWidth={1.5} /> : index}
      </span>
      <div className="flex flex-col gap-1">
        <Heading className="font-sans text-body font-semibold text-ink">
          {title}
        </Heading>
        <p className="text-small text-ink-muted">{description}</p>
        <fieldset disabled={disabled}>{children}</fieldset>
      </div>
    </section>
  )
}
