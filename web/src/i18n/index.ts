import { computed, ref, watch } from "vue";
import { createI18n } from "vue-i18n";
import {
  dateEnUS,
  dateZhCN,
  enUS,
  zhCN,
  type NDateLocale,
  type NLocale,
} from "naive-ui";
import zhCNMessages from "./locales/zh-CN";
import enUSMessages from "./locales/en-US";

export type AppLocale = "zh-CN" | "en-US";

const STORAGE_KEY = "tdload-locale";

function isChineseLanguage(lang: string): boolean {
  return lang.toLowerCase().startsWith("zh");
}

export function detectLocale(): AppLocale {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === "zh-CN" || saved === "en-US") return saved;
  } catch {
    /* ignore */
  }
  const candidates = [
    ...(typeof navigator !== "undefined" ? navigator.languages || [] : []),
    typeof navigator !== "undefined" ? navigator.language : "",
  ].filter(Boolean);
  if (candidates.some((l) => isChineseLanguage(l))) return "zh-CN";
  return "en-US";
}

function applyDocumentLang(locale: AppLocale) {
  if (typeof document !== "undefined") {
    document.documentElement.lang = locale === "zh-CN" ? "zh-CN" : "en";
  }
}

const initial = detectLocale();
applyDocumentLang(initial);

export const i18n = createI18n({
  legacy: false,
  locale: initial,
  fallbackLocale: "en-US",
  messages: {
    "zh-CN": zhCNMessages,
    "en-US": enUSMessages,
  },
});

export const currentLocale = ref<AppLocale>(initial);

watch(
  currentLocale,
  (locale) => {
    i18n.global.locale.value = locale;
    applyDocumentLang(locale);
    try {
      localStorage.setItem(STORAGE_KEY, locale);
    } catch {
      /* ignore */
    }
  },
  { immediate: false },
);

export function setLocale(locale: AppLocale) {
  currentLocale.value = locale;
}

export function toggleLocale() {
  setLocale(currentLocale.value === "zh-CN" ? "en-US" : "zh-CN");
}

export const naiveLocale = computed<NLocale>(() =>
  currentLocale.value === "zh-CN" ? zhCN : enUS,
);

export const naiveDateLocale = computed<NDateLocale>(() =>
  currentLocale.value === "zh-CN" ? dateZhCN : dateEnUS,
);

export function dateLocaleTag(): string {
  return currentLocale.value === "zh-CN" ? "zh-CN" : "en-US";
}

export function formatFromNow(iso: string): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "";
  const d = Date.now() - t;
  const { t: translate } = i18n.global;
  if (d < 60_000) return translate("relativeTime.justNow");
  if (d < 3_600_000) return translate("relativeTime.minutesAgo", { n: Math.floor(d / 60_000) });
  if (d < 86_400_000) return translate("relativeTime.hoursAgo", { n: Math.floor(d / 3_600_000) });
  if (d < 7 * 86_400_000) return translate("relativeTime.daysAgo", { n: Math.floor(d / 86_400_000) });
  return new Date(t).toLocaleDateString(dateLocaleTag());
}
