// Auth API — calls the Go backend /api/user/* endpoints.
//
// The backend session cookie (HttpOnly, SameSite=Lax, 7-day) is set
// automatically by the browser; no token storage is needed on the
// client. credentials:"same-origin" in http.ts ensures the cookie is
// sent with every request.
//
// Password and second-factor steps share an HttpOnly login challenge cookie.

import { apiGet, apiPost, type ApiRequestOptions } from "./http";

export interface AuthUser {
  id: number;
  username: string;
  email: string;
  role: number;
  status: number;
  quota: number;
  used_quota: number;
  group: string;
  aff_code: string;
  inviter_id: number;
  created_at: number;
  admin_password_change_required?: boolean;
  admin_security_required?: boolean;
}

interface LoginResponse {
  session: string;
  user: AuthUser;
  "2fa_required"?: false;
}

interface FactorChallenge {
  "2fa_required": true;
  username: string;
}

/** Backend accounts have no phone field; omit it to keep saved profiles valid. */
export function authUserProfile(user: AuthUser) {
  return { id: String(user.id), username: user.username };
}

export function loginUser(
  username: string,
  password: string,
): Promise<LoginResponse | FactorChallenge> {
  return apiPost<LoginResponse | FactorChallenge>("/api/user/login", {
    username,
    password,
  });
}

export function verifyLoginFactor(
  username: string,
  code: string,
): Promise<LoginResponse> {
  return apiPost<LoginResponse>("/api/user/login/2fa", { username, code });
}

export function registerUser(
  username: string,
  password: string,
  email: string,
): Promise<AuthUser> {
  return apiPost<AuthUser>("/api/user/register", { username, password, email });
}

export function getSelf(
  options?: ApiRequestOptions,
  signal?: AbortSignal,
): Promise<AuthUser> {
  return apiGet<AuthUser>("/api/user/self", signal, options);
}

export function logoutUser(): Promise<null> {
  return apiPost<null>("/api/user/logout");
}
