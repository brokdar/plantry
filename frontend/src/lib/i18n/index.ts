import i18n from "i18next"
import LanguageDetector from "i18next-browser-languagedetector"
import { initReactI18next } from "react-i18next"

import en from "./en.json"
import de from "./de.json"

// Registered before init so the initial language is applied too.
i18n.on("languageChanged", () => {
  document.documentElement.lang = i18n.resolvedLanguage ?? i18n.language
})

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: en },
      de: { translation: de },
    },
    fallbackLng: "en",
    interpolation: { escapeValue: false },
    react: { useSuspense: false },
  })

// `{{slot, lowercase}}`: English writes slot names in lower case mid-phrase
// ("Sun dinner"); German keeps its capitalised nouns by not using it.
i18n.services.formatter?.add("lowercase", (value, lng) =>
  String(value).toLocaleLowerCase(lng)
)

export default i18n
