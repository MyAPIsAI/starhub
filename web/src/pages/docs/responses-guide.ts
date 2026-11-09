import { message as translatedMessage } from "../../i18n/messages";
import { placeholders } from "../../config/placeholders";

type Translate = (zh: string, en: string, ar: string) => string;

export function responsesGuide(
  text: Translate,
  apiBase: string = placeholders.apiBase,
) {
  const request = `curl --fail --silent --show-error ${apiBase}/responses \\
  -H "Authorization: Bearer $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "YOUR_AUTHORIZED_RESPONSES_MODEL",
    "input": "Say hello",
    "max_output_tokens": 64,
    "store": false,
    "stream": false
  }'`;
  const config = `model = "YOUR_AUTHORIZED_RESPONSES_MODEL"
model_provider = "myapis"

[model_providers.myapis]
name = "MyAPIs"
base_url = "${apiBase}"
env_key = "MYAPIS_API_KEY"
wire_api = "responses"`;
  const retrieve = `: "\${RESPONSE_ID:?Set RESPONSE_ID to a response created with this API key}"
curl --fail --silent --show-error "${apiBase}/responses/$RESPONSE_ID" \\
  -H "Authorization: Bearer $MYAPIS_API_KEY"`;
  return {
    request,
    config,
    retrieve,
    lead: text(
      ...translatedMessage(
        "post_v1_responses_accepts_native_input_with_b_464559e923",
      ),
    ),
    response: text(
      ...translatedMessage(
        "for_json_responses_read_text_from_output_mess_d3dd76027f",
      ),
    ),
    streaming: text(
      ...translatedMessage(
        "set_stream_true_for_sse_read_text_deltas_from_4213d62394",
      ),
    ),
    retrieval: text(
      ...translatedMessage(
        "create_with_store_true_or_retain_the_upstream_6a90d4921d",
      ),
    ),
    indexStatus: text(
      ...translatedMessage(
        "x_starhub_response_reference_reports_recorded_8bb25d77ee",
      ),
    ),
    boundary: text(
      ...translatedMessage(
        "foreground_json_sse_owned_previous_response_i_6f9efa5b10",
      ),
    ),
    codex: text(
      ...translatedMessage(
        "codex_configuration_reference_merge_this_into_8209741bb5",
      ),
    ),
  };
}
