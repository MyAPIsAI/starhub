import { message as translatedMessage } from "../i18n/messages";
export interface StoredBillingDetail {
  version: number;
  mode: string;
  priced_at: string;
  group_multiplier: string;
  formula?: string;
  parts: {
    name: string;
    quantity: string;
    unit: string;
    unit_price_usd: string;
  }[];
}

export function callTypeLabel(
  value: string | undefined,
  language: string,
): string {
  const names: Record<string, [string, string, string]> = {
    chat: translatedMessage("chat_94cc5639f0"),
    responses: translatedMessage("responses_41fa1e671c"),
    embeddings: translatedMessage("embeddings_387a044c6b"),
    rerank: translatedMessage("reranking_16b3ce50be"),
    images: translatedMessage("images_2490b318b5"),
    audio: translatedMessage("audio_42f61562b3"),
    video: translatedMessage("video_f038b3ffa9"),
  };
  const labels = names[value ?? ""];
  return labels
    ? labels[language.startsWith("zh") ? 0 : language.startsWith("ar") ? 2 : 1]
    : value || "—";
}
export function readBillingDetail(
  value?: string,
): StoredBillingDetail | undefined {
  if (!value) return undefined;
  try {
    const result = JSON.parse(value);
    return result.version === 1 && Array.isArray(result.parts)
      ? result
      : undefined;
  } catch {
    return undefined;
  }
}
