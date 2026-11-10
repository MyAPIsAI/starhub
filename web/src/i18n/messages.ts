import english from "./messages.en.json";
import chinese from "./messages.zh.json";
import arabic from "./messages.ar.json";

export function localizedMessage(
  key: keyof typeof english,
  language: string,
  values: Record<string, unknown> = {},
): string {
  const template =
    language === "zh"
      ? chinese[key]
      : language === "ar"
        ? arabic[key]
        : english[key];
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) =>
    String(values[name] ?? ""),
  );
}

export function message(
  key: keyof typeof english,
  values: Record<string, unknown> = {},
): [string, string, string] {
  return [
    localizedMessage(key, "zh", values),
    localizedMessage(key, "en", values),
    localizedMessage(key, "ar", values),
  ];
}
// The administration interface intentionally supports only Chinese and English.
export function messagePair(
  key: keyof typeof english,
  values: Record<string, unknown> = {},
): [string, string] {
  const [zh, en] = message(key, values);
  return [zh, en];
}
