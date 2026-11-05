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
    chat: ["文本对话", "Chat", "محادثة"],
    responses: ["响应接口", "Responses", "الاستجابات"],
    embeddings: ["向量嵌入", "Embeddings", "التضمينات"],
    rerank: ["重排序", "Reranking", "إعادة الترتيب"],
    images: ["图像", "Images", "الصور"],
    audio: ["音频", "Audio", "الصوت"],
    video: ["视频", "Video", "الفيديو"],
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
