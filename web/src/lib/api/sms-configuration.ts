import { apiGet, apiPut } from "./http";

export interface SMSConfiguration {
  schema: "sms_configuration_v1";
  user_id: number;
  version: number;
  enabled: boolean;
  access_key_id: string;
  secret_configured: boolean;
  sign_name: string;
  template_code: string;
  source: "database" | "environment_bridge";
  effective_enabled: boolean;
}

export async function getSMSConfiguration(owner: number, signal?: AbortSignal) {
  const result = await apiGet<unknown>(
    `/api/system/sms-config?expected_user_id=${owner}`,
    signal,
  );
  const value = result as Partial<SMSConfiguration> | null;
  if (
    !value ||
    value.schema !== "sms_configuration_v1" ||
    value.user_id !== owner ||
    !Number.isSafeInteger(value.version) ||
    value.version! < 0 ||
    typeof value.enabled !== "boolean" ||
    typeof value.secret_configured !== "boolean" ||
    typeof value.effective_enabled !== "boolean" ||
    typeof value.access_key_id !== "string" ||
    typeof value.sign_name !== "string" ||
    typeof value.template_code !== "string" ||
    !["database", "environment_bridge"].includes(value.source ?? "")
  )
    throw Error("SMS configuration identity or metadata could not be verified");
  return value as SMSConfiguration;
}

export interface SMSConfigurationInput {
  expected_user_id: number;
  expected_version: number;
  current_password: string;
  enabled: boolean;
  access_key_id: string;
  access_secret: string;
  clear_secret: boolean;
  sign_name: string;
  template_code: string;
}

export async function saveSMSConfiguration(input: SMSConfigurationInput) {
  const result = await apiPut<{
    user_id: number;
    version: number;
    saved: boolean;
  }>("/api/system/sms-config", input);
  if (
    result.user_id !== input.expected_user_id ||
    result.version !== input.expected_version + 1 ||
    result.saved !== true
  )
    throw Error("SMS configuration save was not confirmed");
}
