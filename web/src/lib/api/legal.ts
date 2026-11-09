import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
export const legalKinds = ["terms", "privacy", "cookies", "refund"] as const;
export type LegalKind = (typeof legalKinds)[number];
export interface LegalDocument {
  kind: LegalKind;
  language: string;
  version: string;
  content: string;
  effective_at: number;
  timezone: string;
  now: number;
  next_effective_at: number;
}
export interface LegalPolicy {
  region_id: string;
  timezone: string;
  cookie_consent: boolean;
  cookie_consent_mode: "banner" | "dialog";
  consent_version: string;
}
export function useLegalPolicy() {
  return useQuery({
    queryKey: ["public-legal-policy"],
    queryFn: () => apiGet<LegalPolicy>("/api/legal-policy"),
    staleTime: 0,
    refetchInterval: 60000,
    refetchOnWindowFocus: true,
  });
}
export function usePublishedLegal(kind: string, language: string) {
  return useQuery({
    queryKey: ["published-legal", kind, language],
    queryFn: () =>
      apiGet<LegalDocument>(
        `/api/legal/${encodeURIComponent(kind)}?language=${language}`,
      ),
    enabled: legalKinds.includes(kind as LegalKind),
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchInterval: (q) => {
      const doc = q.state.data;
      return doc && doc.next_effective_at > doc.now
        ? Math.max(
            500,
            Math.min(30000, (doc.next_effective_at - doc.now) * 1000),
          )
        : 30000;
    },
  });
}
