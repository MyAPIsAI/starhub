import english from "./messages.en.json";
import chinese from "./messages.zh.json";
import arabic from "./messages.ar.json";

export function localizedMessage(
  key: keyof typeof english,
  language: string,
): string {
  return language === "zh"
    ? chinese[key]
    : language === "ar"
      ? arabic[key]
      : english[key];
}

export function message(
  key: keyof typeof english,
  values: Record<string, unknown> = {},
): [string, string, string] {
  const render = (text: string) =>
    text.replace(/\{\{(\w+)\}\}/g, (_, name: string) =>
      String(values[name] ?? ""),
    );
  return [render(chinese[key]), render(english[key]), render(arabic[key])];
}
// The administration interface intentionally supports only Chinese and English.
export function messagePair(
  key: keyof typeof english,
  values: Record<string, unknown> = {},
): [string, string] {
  const [zh, en] = message(key, values);
  return [zh, en];
}
