import { apiGet, apiPost } from "./http";
import { usePreferences } from "../preferences";

export type StaffStage = "change_password" | "enroll" | "verify" | "ready";
export interface StaffAuthState {
  stage: StaffStage;
  id: number;
  username: string;
  role: number;
  account_kind: "staff";
}
export interface StaffAccount {
  platform_admin: boolean;
  id: number;
  username: string;
  role: number;
  status: number;
  version: number;
  must_change_password: boolean;
  two_factor_bound: boolean;
}
export const staffAuth = {
  state: () =>
    apiGet<StaffAuthState>("/api/admin/auth/state", undefined, {
      handleUnauthorizedLocally: true,
    }),
  login: (username: string, password: string) =>
    apiPost<StaffAuthState>(
      "/api/admin/auth/login",
      { username, password },
      { handleUnauthorizedLocally: true },
    ),
  password: (current_password: string, new_password: string) =>
    apiPost<StaffAuthState>(
      "/api/admin/auth/password",
      { current_password, new_password },
      { handleUnauthorizedLocally: true },
    ),
  enroll: () =>
    apiPost<{ secret: string; otpauth_uri: string }>(
      "/api/admin/auth/enroll",
      undefined,
      { handleUnauthorizedLocally: true },
    ),
  verify: (code: string) =>
    apiPost<StaffAuthState>(
      "/api/admin/auth/verify",
      { code },
      { handleUnauthorizedLocally: true },
    ),
  logout: () =>
    apiPost<null>("/api/admin/auth/logout", undefined, {
      handleUnauthorizedLocally: true,
    }),
};
export function useStaffText() {
  const language = usePreferences((s) => s.language);
  return (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
}
export function staffRoleName(
  role: number,
  text: (zh: string, en: string, ar: string) => string,
): string {
  return role === 100
    ? text("超级管理员", "Super administrator", "المسؤول الأعلى")
    : role === 10
      ? text("运营", "Operations", "العمليات")
      : role === 5
        ? text("财务", "Finance", "المالية")
        : role === 20
          ? text("技术", "Technical", "التقنية")
          : text("未知角色", "Unknown role", "دور غير معروف");
}
