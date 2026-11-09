import { message as translatedMessage } from "../../i18n/messages";
import { placeholders } from "../../config/placeholders";

export function geminiNativeGuide(
  text: (zh: string, en: string, ar: string) => string,
  apiBase: string = placeholders.apiBase,
) {
  return {
    example: `curl --fail --silent --show-error '${apiBase.replace(/\/v1\/?$/, "")}/v1beta/models/YOUR_AUTHORIZED_GEMINI_MODEL:generateContent' \\
  -H "x-goog-api-key: $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "contents": [{"role": "user", "parts": [{"text": "Say hello"}]}],
    "generationConfig": {"maxOutputTokens": 64}
  }'`,
    lead: text(
      ...translatedMessage(
        "native_endpoint_post_v1beta_models_model_gene_c38d671fa4",
      ),
    ),
    response: text(
      ...translatedMessage(
        "read_text_from_candidates_0_content_parts_tho_672f11a6ed",
      ),
    ),
    streaming: text(
      ...translatedMessage(
        "for_streaming_use_streamgeneratecontent_alt_s_9a8c3b9dc8",
      ),
    ),
    boundary: text(
      ...translatedMessage(
        "locally_verified_scope_one_candidate_text_out_f8b48c1930",
      ),
    ),
  };
}
