import { message as translatedMessage } from "../../i18n/messages";
import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";

export type AnnouncementLanguage = "zh" | "en" | "ar";
export type AnnouncementStatus =
  "investigating" | "identified" | "monitoring" | "resolved";
export type AnnouncementText = Record<AnnouncementLanguage, string>;
export interface Announcement {
  id: number;
  version: number;
  title: AnnouncementText;
  content: AnnouncementText;
  vendors: string[];
  models: string[];
  status: AnnouncementStatus;
  created_at: number;
  updated_at: number;
  resolved_at: number;
}
export interface AnnouncementList {
  items: Announcement[];
  total: number;
}
export const announcementStatuses: AnnouncementStatus[] = [
  "investigating",
  "identified",
  "monitoring",
  "resolved",
];
export function announcementStatus(
  status: AnnouncementStatus,
  t: (zh: string, en: string, ar: string) => string,
) {
  return {
    investigating: t(...translatedMessage("investigating_0c5447395e")),
    identified: t(...translatedMessage("identified_56d6138ce1")),
    monitoring: t(...translatedMessage("monitoring_81d8512cf9")),
    resolved: t(...translatedMessage("resolved_6f9b1bbcb3")),
  }[status];
}
export function usePublishedAnnouncements() {
  return useQuery({
    queryKey: ["published-announcements"],
    queryFn: ({ signal }) =>
      apiGet<AnnouncementList>("/api/status/announcements", signal, {
        handleUnauthorizedLocally: true,
      }),
    refetchInterval: 30_000,
    staleTime: 0,
    retry: 1,
  });
}
