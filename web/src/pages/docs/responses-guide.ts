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
  const retrieve = `: "\${RESPONSE_ID:?Set RESPONSE_ID to a response created with this API key}"
curl --fail --silent --show-error "${placeholders.apiBase}/responses/$RESPONSE_ID" \\
  -H "Authorization: Bearer $MYAPIS_API_KEY"`;
  return {
    request,
    config,
    retrieve,
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
    retrieval: text(
      "创建时设 store: true，或保留上游默认存储选项，可用同一 API 密钥通过 GET /v1/responses/{id} 原生检索已记录归属的前台终态或后台响应，返回 JSON。网关保存归属索引，内容及实际保留时间由上游负责；索引用于检索的有效期最多 30 天。当前模型、分组、原渠道及原凭证仍须有效；池内其他凭证不会替代原账户。检索不重复扣生成配额，配额耗尽时也可读取已有响应。",
      "Create with store: true, or retain the upstream storage default, then use the same API key to GET /v1/responses/{id} for a foreground terminal or background response with a recorded ownership binding. Retrieval returns native JSON. The gateway keeps an ownership index; content storage and actual retention belong to the upstream. Index visibility is at most 30 days. Current model/group access and the original channel/account must remain available; another pool credential is never substituted. Retrieval does not recharge generation quota and remains available when quota is exhausted.",
      "أنشئ باستخدام store: true أو اترك إعداد التخزين الافتراضي للمزود، ثم استخدم مفتاح API نفسه مع GET /v1/responses/{id} لاسترجاع استجابة نهائية ذات ارتباط ملكية مسجل بصيغة JSON الأصلية. تحفظ البوابة فهرس الملكية، ويحدد المزود تخزين المحتوى ومدة الاحتفاظ الفعلية. صلاحية الفهرس للاسترجاع لا تتجاوز 30 يومًا. يجب استمرار صلاحيات النموذج والمجموعة وتوفر القناة والحساب الأصليين؛ لا يُستبدل الحساب بمفتاح آخر في المجموعة. لا يخصم الاسترجاع حصة التوليد مجددًا ويعمل عند نفادها.",
    ),
    indexStatus: text(
      "归属记录状态位于 X-StarHub-Response-Reference：recorded 表示归属已记录，not_stored 表示请求或上游关闭了存储，unavailable 表示未确认可用记录。JSON 在响应头提供状态；SSE 在流结束后的尾部字段提供状态，请读完整流后再检索。未记录归属的旧响应、未知或歧义 ID 不会转向上游试查；recorded 不保证上游仍保留内容。",
      "X-StarHub-Response-Reference reports recorded, not_stored, or unavailable ownership-index status. JSON uses a response header; SSE uses a trailer after the stream finishes, so consume the complete stream before retrieving. Old unobserved responses and unknown or ambiguous IDs are not probed upstream. A recorded binding does not guarantee that the upstream still retains the content.",
      "يعرض X-StarHub-Response-Reference حالة فهرس الملكية: recorded أو not_stored أو unavailable. تظهر في ترويسة JSON، وفي ترويسة ختامية بعد اكتمال SSE؛ اقرأ التدفق كاملًا قبل الاسترجاع. لا تُستعلم من المزود الاستجابات القديمة غير المسجلة أو المعرفات المجهولة أو الملتبسة. تسجيل الارتباط لا يضمن استمرار احتفاظ المزود بالمحتوى.",
    ),
    boundary: text(
      "已支持前台 JSON/SSE、同密钥 previous_response_id 续轮、网关会话引用、后台 JSON、取消、删除和压缩。后台须显式设置 background:true、store:true、stream:false；账单由持久任务恢复并幂等结算，查询和取消不再预扣生成额度。网关会话仅保存归属及上次响应引用，创建时只接受 metadata，不提供原生 items 存储接口。文件、托管工具、流式检索、后台流式和 WebSocket 仍未开放。",
      "Foreground JSON/SSE, owned previous_response_id continuation, gateway conversation references, background JSON, cancellation, deletion and compaction are available. Background requires explicit background:true, store:true and stream:false. Durable tasks recover settlement without charging retrieval or cancellation again. Gateway conversations store ownership and the last response reference; creation accepts metadata only, without native items storage. Files, hosted tools, streaming retrieval, background streaming and WebSocket remain unavailable.",
      "تتوفر JSON/SSE الأمامية والمتابعة بمفتاح المالك ومراجع محادثات البوابة وJSON الخلفية والإلغاء والحذف والضغط. تتطلب الخلفية background:true وstore:true وstream:false صراحةً. تستعيد المهام الدائمة التسوية دون حجز جديد للاسترجاع أو الإلغاء. تحفظ محادثات البوابة الملكية ومرجع الاستجابة الأخيرة وتقبل metadata فقط عند الإنشاء دون تخزين items الأصلي. لا تتوفر الملفات والأدوات المستضافة والاسترجاع المتدفق وتدفق الخلفية وWebSocket.",
    ),
    codex: text(
      "Codex（代码客户端）配置参考：把以下内容合并到自己的 config.toml，在启动客户端的环境中设置 MYAPIS_API_KEY。base_url 保留 /v1；model 必须替换为你获准使用的实际模型。配置字段遵循官方说明；本地已验证原生工具续轮、压缩与后台生命周期；真实 Codex 端到端验收按用户要求搁置。配置不保证目标客户端兼容，文件、托管工具和 WebSocket 等未开放能力仍会失败。",
      "Codex configuration reference: merge this into your config.toml and set MYAPIS_API_KEY in the environment that launches the client. Keep /v1 in base_url and replace model with your authorized actual model. These fields follow official documentation. Native tool continuation, compaction and background lifecycle have local verification. Real Codex acceptance is deferred at the user’s request. Configuration is not proof of client compatibility; files, hosted tools and WebSocket remain unavailable.",
      "مرجع إعداد Codex: ادمج هذا في config.toml واضبط MYAPIS_API_KEY في بيئة تشغيل العميل. احتفظ بـ /v1 في base_url واستبدل model بنموذجك الفعلي المصرح به. تتبع الحقول الوثائق الرسمية. تم التحقق محليًا من متابعة الأدوات والضغط ودورة الخلفية. التحقق باستخدام Codex الفعلي مؤجل بطلب المستخدم؛ الإعداد لا يثبت توافق العميل ولا تتوفر الملفات والأدوات المستضافة وWebSocket.",
    ),
  };
}
