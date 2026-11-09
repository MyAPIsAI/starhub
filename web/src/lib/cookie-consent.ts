import { create } from "zustand";
import type { LegalPolicy } from "./api/legal";
export const consentKey = "myapis-cookie-consent-v1";
type Receipt = { region_id: string; version: string; choice: "necessary" };
let memory: Receipt | null = null;
export function hasNecessaryConsent(policy: LegalPolicy): boolean {
  let receipt = memory;
  try {
    const raw = localStorage.getItem(consentKey);
    try {
      receipt = raw ? JSON.parse(raw) : null;
    } catch {
      receipt = null;
    }
  } catch {
    /* Storage can be unavailable; optional storage remains disabled. */
  }
  return (
    receipt?.choice === "necessary" &&
    receipt.region_id === policy.region_id &&
    receipt.version === policy.consent_version
  );
}
export function saveNecessaryConsent(policy: LegalPolicy) {
  memory = {
    region_id: policy.region_id,
    version: policy.consent_version,
    choice: "necessary",
  };
  try {
    localStorage.setItem(consentKey, JSON.stringify(memory));
    return true;
  } catch {
    return false;
  }
}
export const useCookieSettings = create<{
  open: boolean;
  show: () => void;
  close: () => void;
}>((set) => ({
  open: false,
  show: () => set({ open: true }),
  close: () => set({ open: false }),
}));
