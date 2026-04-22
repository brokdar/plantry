import { Check, Loader2, TriangleAlert } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { SettingsCard } from "@/components/editorial/SettingsCard"
import { SettingRow } from "@/components/settings/SettingRow"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { listAIModels, type SettingItem } from "@/lib/api/settings"
import {
  useClearSetting,
  useSetSetting,
  useSettings,
  useSystemInfo,
} from "@/lib/queries/settings"
import { ApiError } from "@/lib/api/client"
import { cn } from "@/lib/utils"

const KEY_PROVIDER = "ai.provider"
const KEY_API_KEY = "ai.api_key"
const KEY_MODEL = "ai.model"
const KEY_RATE_LIMIT = "ai.rate_limit_per_min"
const KEY_FAKE_SCRIPT = "ai.fake_script"
const KEY_FDC = "fdc.api_key"

type ModelOption = { id: string; display_name?: string }

export function AITab() {
  const { t } = useTranslation()
  const { data, isLoading } = useSettings()
  const { data: system } = useSystemInfo()
  const items = useMemo(() => indexByKey(data?.items ?? []), [data])

  if (isLoading || !data) {
    return (
      <SettingsCard title={t("settings_page.tabs.ai")}>
        <p className="text-sm text-on-surface-variant">{t("common.loading")}</p>
      </SettingsCard>
    )
  }

  const cipherAvailable =
    data.cipher_available && system?.cipher_available !== false

  return (
    <div className="space-y-6">
      <SettingsCard title={t("settings_page.ai.provider_section_title")}>
        {!cipherAvailable && <SecretKeyBanner />}
        <AIWizard items={items} cipherAvailable={cipherAvailable} />
      </SettingsCard>

      <SettingsCard title={t("settings_page.ai.advanced_title")}>
        <RateLimitRow
          key={`rate-${items[KEY_RATE_LIMIT]?.source ?? "none"}-${items[KEY_RATE_LIMIT]?.value ?? ""}`}
          item={items[KEY_RATE_LIMIT]}
        />
        <FakeScriptRow
          key={`fake-${items[KEY_FAKE_SCRIPT]?.source ?? "none"}-${items[KEY_FAKE_SCRIPT]?.value ?? ""}`}
          item={items[KEY_FAKE_SCRIPT]}
        />
      </SettingsCard>

      <SettingsCard title={t("settings_page.fdc.title")}>
        <FDCKeyRow item={items[KEY_FDC]} cipherAvailable={cipherAvailable} />
      </SettingsCard>
    </div>
  )
}

function SecretKeyBanner() {
  const { t } = useTranslation()
  return (
    <div
      className="rounded-xl border border-tertiary/40 bg-tertiary/5 p-4 text-sm"
      data-testid="secret-key-banner"
    >
      <div className="flex gap-3">
        <TriangleAlert className="mt-0.5 size-4 shrink-0 text-tertiary" />
        <div>
          <p className="font-medium text-on-surface">
            {t("settings_page.ai.secret_key_banner_title")}
          </p>
          <p className="mt-1 leading-relaxed text-on-surface-variant">
            {t("settings_page.ai.secret_key_banner_body")}
          </p>
        </div>
      </div>
    </div>
  )
}

function indexByKey(items: SettingItem[]): Record<string, SettingItem> {
  return Object.fromEntries(items.map((it) => [it.key, it]))
}

type WizardProps = {
  items: Record<string, SettingItem>
  cipherAvailable: boolean
}

function AIWizard({ items, cipherAvailable }: WizardProps) {
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

  const setSetting = useSetSetting()
  const clearSetting = useClearSetting()

  // Sync local wizard state with upstream data whenever the settings query
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

  async function handleProviderChange(next: string) {
    setProvider(next)
    setModels([])
    setValidated(false)
    setValidationError(null)
    if (next === "") {
      await clearSetting.mutateAsync(KEY_PROVIDER)
    } else {
      await setSetting.mutateAsync({ key: KEY_PROVIDER, value: next })
    }
  }

  async function saveKey() {
    if (!apiKeyDraft) return
    try {
      await setSetting.mutateAsync({ key: KEY_API_KEY, value: apiKeyDraft })
      toast.success(t("settings_page.ai.api_key_saved"))
      setApiKeyDraft("")
    } catch (err) {
      const key = err instanceof ApiError ? err.messageKey : "error.server"
      toast.error(t(key))
    }
  }

  async function saveModel(next: string) {
    setModel(next)
    try {
      await setSetting.mutateAsync({ key: KEY_MODEL, value: next })
      toast.success(t("settings_page.ai.model_saved"))
    } catch (err) {
      const key = err instanceof ApiError ? err.messageKey : "error.server"
      toast.error(t(key))
    }
  }

  return (
    <div className="space-y-6">
      <Step
        index={1}
        title={t("settings_page.ai.wizard.step1_title")}
        description={t("settings_page.ai.wizard.step1_description")}
        active={!step1Complete}
        complete={step1Complete}
      >
        <SettingRow
          label={t("settings_page.ai.provider_label")}
          item={providerItem}
          onClearOverride={
            providerItem?.source === "db"
              ? () => clearSetting.mutate(KEY_PROVIDER)
              : undefined
          }
        >
          <Select
            value={provider || "none"}
            onValueChange={(v) => handleProviderChange(v === "none" ? "" : v)}
          >
            <SelectTrigger className="w-full" data-testid="ai-provider-select">
              <SelectValue
                placeholder={t("settings_page.ai.provider_placeholder")}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">
                {t("settings_page.ai.provider_none")}
              </SelectItem>
              <SelectItem value="openai">OpenAI</SelectItem>
              <SelectItem value="anthropic">Anthropic</SelectItem>
              <SelectItem value="fake">
                {t("settings_page.ai.provider_fake")}
              </SelectItem>
            </SelectContent>
          </Select>
        </SettingRow>
      </Step>

      <Step
        index={2}
        title={t("settings_page.ai.wizard.step2_title")}
        description={t("settings_page.ai.wizard.step2_description")}
        active={step1Complete && !step2Complete}
        complete={step2Complete}
        disabled={!step1Complete}
      >
        <SettingRow
          label={t("settings_page.ai.api_key_label")}
          description={
            hasStoredKey
              ? t("settings_page.ai.api_key_stored", {
                  preview: keyItem?.masked_preview ?? "",
                })
              : t("settings_page.ai.api_key_description")
          }
          item={keyItem}
          onClearOverride={
            keyItem?.source === "db"
              ? () => clearSetting.mutate(KEY_API_KEY)
              : undefined
          }
        >
          <div className="flex flex-col gap-3 sm:flex-row">
            <Input
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
              disabled={!step1Complete || !cipherAvailable}
              data-testid="ai-api-key-input"
            />
            <Button
              type="button"
              variant="outline"
              onClick={validate}
              disabled={!canValidate || validating}
              data-testid="ai-test-connection"
            >
              {validating && (
                <Loader2 className="mr-2 size-4 animate-spin" aria-hidden />
              )}
              {t("settings_page.ai.test_connection")}
            </Button>
            {apiKeyDraft && cipherAvailable && (
              <Button
                type="button"
                variant="default"
                onClick={saveKey}
                disabled={setSetting.isPending}
                data-testid="ai-save-api-key"
              >
                {t("common.save")}
              </Button>
            )}
          </div>
          {validationError && (
            <p className="mt-2 text-xs text-destructive">
              {t(validationError)}
            </p>
          )}
          {validated && (
            <p className="mt-2 inline-flex items-center gap-1 text-xs text-primary">
              <Check className="size-3" aria-hidden />
              {t("settings_page.ai.validated")}
            </p>
          )}
        </SettingRow>
      </Step>

      <Step
        index={3}
        title={t("settings_page.ai.wizard.step3_title")}
        description={t("settings_page.ai.wizard.step3_description")}
        active={step2Complete}
        complete={!!model && step2Complete}
        disabled={!step2Complete}
      >
        <SettingRow
          label={t("settings_page.ai.model_label")}
          item={modelItem}
          onClearOverride={
            modelItem?.source === "db"
              ? () => clearSetting.mutate(KEY_MODEL)
              : undefined
          }
        >
          <Select
            value={model || ""}
            onValueChange={saveModel}
            disabled={!step2Complete || models.length === 0}
          >
            <SelectTrigger className="w-full" data-testid="ai-model-select">
              <SelectValue
                placeholder={t("settings_page.ai.model_placeholder")}
              />
            </SelectTrigger>
            <SelectContent>
              {model && !models.some((m) => m.id === model) && (
                <SelectItem value={model}>{model}</SelectItem>
              )}
              {models.map((m) => (
                <SelectItem key={m.id} value={m.id}>
                  {m.display_name || m.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {models.length === 0 && step2Complete && (
            <p className="mt-1 text-xs text-on-surface-variant">
              {t("settings_page.ai.click_test_to_load_models")}
            </p>
          )}
        </SettingRow>
      </Step>
    </div>
  )
}

type StepProps = {
  index: number
  title: string
  description: string
  active: boolean
  complete: boolean
  disabled?: boolean
  children: React.ReactNode
}

function Step({
  index,
  title,
  description,
  active,
  complete,
  disabled,
  children,
}: StepProps) {
  return (
    <section
      aria-disabled={disabled}
      className={cn(
        "grid gap-4 md:grid-cols-[auto_minmax(0,1fr)]",
        disabled && "opacity-50"
      )}
      data-testid={`ai-wizard-step-${index}`}
    >
      <div className="flex flex-col items-center gap-2 pt-1">
        <span
          className={cn(
            "flex size-8 items-center justify-center rounded-full font-heading text-sm font-bold",
            complete
              ? "bg-primary text-primary-foreground"
              : active
                ? "border-2 border-primary text-primary"
                : "border border-outline text-on-surface-variant"
          )}
        >
          {complete ? <Check className="size-4" aria-hidden /> : index}
        </span>
      </div>
      <div className="space-y-4">
        <div>
          <h4 className="font-heading text-base font-semibold text-on-surface">
            {title}
          </h4>
          <p className="text-xs leading-relaxed text-on-surface-variant">
            {description}
          </p>
        </div>
        <fieldset disabled={disabled} className="space-y-3">
          {children}
        </fieldset>
      </div>
    </section>
  )
}

function RateLimitRow({ item }: { item?: SettingItem }) {
  const { t } = useTranslation()
  const setSetting = useSetSetting()
  const clearSetting = useClearSetting()
  // The parent component re-mounts this row (via `key`) whenever the upstream
  // value or source changes, so local state is initialized fresh on each
  // server update — no effect-driven sync needed.
  const [value, setValue] = useState(item?.value ?? "10")

  return (
    <SettingRow
      label={t("settings_page.ai.rate_limit_label")}
      description={t("settings_page.ai.rate_limit_description")}
      item={item}
      onClearOverride={
        item?.source === "db"
          ? () => clearSetting.mutate(KEY_RATE_LIMIT)
          : undefined
      }
    >
      <div className="flex gap-3">
        <Input
          type="number"
          min={0}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="max-w-24"
          data-testid="ai-rate-limit-input"
        />
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            setSetting.mutate(
              { key: KEY_RATE_LIMIT, value },
              { onSuccess: () => toast.success(t("common.saved")) }
            )
          }
        >
          {t("common.save")}
        </Button>
      </div>
    </SettingRow>
  )
}

function FakeScriptRow({ item }: { item?: SettingItem }) {
  const { t } = useTranslation()
  const setSetting = useSetSetting()
  const clearSetting = useClearSetting()
  // Parent re-mounts via `key` prop when upstream changes — see RateLimitRow.
  const [value, setValue] = useState(item?.value ?? "")

  return (
    <SettingRow
      label={t("settings_page.ai.fake_script_label")}
      description={t("settings_page.ai.fake_script_description")}
      item={item}
      onClearOverride={
        item?.source === "db"
          ? () => clearSetting.mutate(KEY_FAKE_SCRIPT)
          : undefined
      }
    >
      <div className="flex gap-3">
        <Input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="/path/to/fake.jsonl"
          data-testid="ai-fake-script-input"
        />
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            setSetting.mutate(
              { key: KEY_FAKE_SCRIPT, value },
              { onSuccess: () => toast.success(t("common.saved")) }
            )
          }
        >
          {t("common.save")}
        </Button>
      </div>
    </SettingRow>
  )
}

function FDCKeyRow({
  item,
  cipherAvailable,
}: {
  item?: SettingItem
  cipherAvailable: boolean
}) {
  const { t } = useTranslation()
  const setSetting = useSetSetting()
  const clearSetting = useClearSetting()
  const [draft, setDraft] = useState("")

  const hasStored = !!item && (item.source === "db" || item.source === "env")

  return (
    <SettingRow
      label={t("settings_page.fdc.api_key_label")}
      description={
        hasStored
          ? t("settings_page.fdc.api_key_stored", {
              preview: item?.masked_preview ?? "",
            })
          : t("settings_page.fdc.api_key_description")
      }
      item={item}
      onClearOverride={
        item?.source === "db" ? () => clearSetting.mutate(KEY_FDC) : undefined
      }
    >
      <div className="flex gap-3">
        <Input
          type="password"
          autoComplete="off"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={
            hasStored
              ? t("settings_page.fdc.api_key_placeholder_replace")
              : t("settings_page.fdc.api_key_placeholder")
          }
          disabled={!cipherAvailable}
          data-testid="fdc-key-input"
        />
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            if (!draft) return
            setSetting.mutate(
              { key: KEY_FDC, value: draft },
              {
                onSuccess: () => {
                  toast.success(t("common.saved"))
                  setDraft("")
                },
              }
            )
          }}
          disabled={!cipherAvailable || !draft}
        >
          {t("common.save")}
        </Button>
      </div>
    </SettingRow>
  )
}
