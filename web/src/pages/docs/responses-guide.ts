import { placeholders } from "../../config/placeholders";

type Translate = (zh: string, en: string, ar: string) => string;

export function responsesGuide(text: Translate) {
  const request = `curl --fail --silent --show-error ${placeholders.apiBase}/responses \\
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
base_url = "${placeholders.apiBase}"
env_key = "MYAPIS_API_KEY"
wire_api = "responses"`;
  return {
    request,
    config,
    lead: text(
      "POST /v1/responses 接收原生 input，使用 Bearer 密钥。将示例地址和模型占位符替换为你的网关地址及已授权、渠道可用的 Responses（响应接口）模型；执行请求会产生实际费用。",
      "POST /v1/responses accepts native input with Bearer authentication. Replace the example address and model placeholder with your gateway and an authorized Responses model on an available channel. Executing requests incurs usage charges.",
      "يستقبل POST /v1/responses حقل input الأصلي مع مصادقة Bearer. استبدل عنوان المثال ومعرّف النموذج بعنوان بوابتك ونموذج Responses مصرح به على قناة متاحة. تنفيذ الطلبات يترتب عليه رسوم استخدام.",
    ),
    response: text(
      "非流式结果位于 output 数组：读取 message 项的 content 中 type 为 output_text 的 text。usage.input_tokens/output_tokens 是总量，缓存和推理明细包含在总量内；status 可为 completed、incomplete、failed 或 cancelled。",
      "For JSON responses, read text from output message content blocks with type output_text. usage.input_tokens/output_tokens are totals that include cache and reasoning subsets. status may be completed, incomplete, failed, or cancelled.",
      "في استجابات JSON اقرأ النص من كتل content ذات النوع output_text في رسائل output. تتضمن إجماليات usage.input_tokens/output_tokens حصص التخزين المؤقت والاستدلال. قد تكون status هي completed أو incomplete أو failed أو cancelled.",
    ),
    streaming: text(
      "设置 stream: true 后读取 SSE（服务器发送事件）。response.output_text.delta 的 delta 为文本增量；response.completed、response.incomplete、response.failed 表示终态，error 表示错误。连接中断不代表成功；不要按 Chat Completions 的 choices 或 [DONE] 解析。失败或中断仍可能产生已用量费用。",
      "Set stream: true for SSE. Read text deltas from response.output_text.delta.delta; response.completed, response.incomplete, and response.failed are terminal events, while error reports an error. A broken connection is not success. Use semantic events rather than Chat Completions choices or [DONE]. Failed or interrupted requests may still incur usage charges.",
      "اضبط stream: true لاستخدام SSE. اقرأ زيادات النص من response.output_text.delta.delta؛ أحداث response.completed وresponse.incomplete وresponse.failed نهائية، ويشير error إلى خطأ. انقطاع الاتصال لا يعني النجاح. استخدم الأحداث الدلالية بدل choices أو [DONE]. قد تُحاسب الطلبات الفاشلة أو المنقطعة حسب الاستخدام.",
    ),
    boundary: text(
      "当前支持前台 HTTP/JSON 和 SSE，以及 function/custom 工具的完整历史。previous_response_id、conversation、background: true、文件引用、托管工具、检索/取消/压缩及 Responses WebSocket 尚未开放。续轮请保留首条用户消息、工具调用及结果；智能路由要求 responses 和所需能力元数据，过期绑定或删减的工具历史会被拒绝。",
      "Foreground HTTP/JSON and SSE support complete function/custom tool histories. previous_response_id, conversation, background: true, file references, hosted tools, retrieval/cancel/compaction endpoints, and Responses WebSocket are unavailable. Preserve the first user message, tool calls, and results on continuation. Smart routing requires responses and applicable capability metadata; expired bindings or trimmed tool histories are rejected.",
      "تدعم HTTP/JSON وSSE الأماميتان السجل الكامل لأدوات function/custom. لا تتوفر previous_response_id أو conversation أو background: true أو مراجع الملفات أو الأدوات المستضافة أو نقاط الاسترجاع والإلغاء والضغط أو Responses WebSocket. احتفظ بأول رسالة مستخدم واستدعاءات الأدوات ونتائجها عند المتابعة. يتطلب التوجيه الذكي بيانات responses والقدرات اللازمة؛ تُرفض الروابط المنتهية والسجلات المقتطعة.",
    ),
    codex: text(
      "Codex（代码客户端）配置参考：把以下内容合并到自己的 config.toml，在启动客户端的环境中设置 MYAPIS_API_KEY。base_url 保留 /v1；model 必须替换为你获准使用的实际模型。配置字段遵循官方说明；真实 Codex 端到端验收尚未完成，客户端若请求压缩、后台或其他未开放能力会失败。",
      "Codex configuration reference: merge this into your config.toml and set MYAPIS_API_KEY in the environment that launches the client. Keep /v1 in base_url and replace model with your authorized actual model. These fields follow official documentation. Real Codex end-to-end verification is pending; client requests for compaction, background, or other unavailable capabilities will fail.",
      "مرجع إعداد Codex: ادمج هذا في config.toml واضبط MYAPIS_API_KEY في بيئة تشغيل العميل. احتفظ بـ /v1 في base_url واستبدل model بنموذجك الفعلي المصرح به. تتبع الحقول الوثائق الرسمية. لم يكتمل التحقق الشامل باستخدام Codex الفعلي؛ ستفشل طلبات الضغط أو الخلفية أو القدرات غير المتاحة.",
    ),
  };
}
