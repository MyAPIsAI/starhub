export const sections = [
  "overview",
  "tokens",
  "logs",
  "profile",
  "channels",
  "users",
  "options",
  "redemptions",
  "audit",
] as const;
export type Section = (typeof sections)[number];
export function canView(role: number, section: string) {
  return (
    sections.includes(section as Section) &&
    (["overview", "tokens", "logs", "profile"].includes(section) || role >= 10)
  );
}
export function canEdit(role: number, section: string) {
  return section === "users"
    ? role >= 100
    : ["channels", "options", "redemptions"].includes(section)
      ? role >= 10
      : section === "tokens";
}
export function safeRedirect(value: unknown): string {
  return typeof value === "string" && /^\/console(?:\/[a-z]+)?$/.test(value)
    ? value
    : "/console/overview";
}
