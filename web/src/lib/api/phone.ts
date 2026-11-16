import { apiGet, apiPost } from "./http";

export interface PhoneBinding {
  user_id: number;
  bound: boolean;
  masked_phone: string;
}

export interface BindPhoneInput {
  expected_user_id: number;
  current_password: string;
  phone: string;
  code?: string;
}

export function getPhoneBinding(owner: number, signal?: AbortSignal) {
  return apiGet<PhoneBinding>(
    `/api/user/phone?expected_user_id=${owner}`,
    signal,
  );
}

export function requestPhoneBindingCode(input: BindPhoneInput) {
  return apiPost<{ requested: boolean; retry_after: number }>(
    "/api/user/phone-verification/bind",
    input,
  );
}

export function bindPhone(input: BindPhoneInput) {
  return apiPost<{ bound: boolean; user_id: number }>(
    "/api/user/bind-phone",
    input,
  );
}
