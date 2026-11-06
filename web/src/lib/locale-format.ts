export function regionalLocale(language: string): string {
  return language.startsWith("ar")
    ? "ar-AE-u-nu-latn"
    : language.startsWith("zh")
      ? "zh-CN-u-nu-latn"
      : "en-US-u-nu-latn";
}
