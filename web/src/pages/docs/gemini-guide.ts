import { placeholders } from "../../config/placeholders";

export function geminiNativeGuide(
  text: (zh: string, en: string, ar: string) => string,
) {
  return {
    example: `curl --fail --silent --show-error '${placeholders.apiBase.replace(/\/v1\/?$/, "")}/v1beta/models/YOUR_AUTHORIZED_GEMINI_MODEL:generateContent' \\
  -H "x-goog-api-key: $MYAPIS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "contents": [{"role": "user", "parts": [{"text": "Say hello"}]}],
    "generationConfig": {"maxOutputTokens": 64}
  }'`,
    lead: text(
      "原生接口：POST /v1beta/models/{model}:generateContent。模型标识位于路径，正文使用 contents 与 parts；替换示例网关地址和已授权的实际模型。该入口接受 x-goog-api-key 或 Bearer 密钥，调用会产生实际费用。",
      "Native endpoint: POST /v1beta/models/{model}:generateContent. The path carries the model ID and the body uses contents and parts. Replace the example gateway and authorized actual model. This endpoint accepts x-goog-api-key or a Bearer key. Calls incur usage charges.",
      "النقطة الأصلية: POST /v1beta/models/{model}:generateContent. يحمل المسار معرّف النموذج ويستخدم الجسم contents وparts. استبدل بوابة المثال والنموذج الفعلي المصرح به. تقبل النقطة x-goog-api-key أو مفتاح Bearer. تترتب رسوم على الاستدعاءات.",
    ),
    response: text(
      "读取 candidates[0].content.parts 中的 text，thought: true 的文本为推理内容。finishReason 为 STOP 或 MAX_TOKENS 表示正常结束；检查 promptFeedback.blockReason 和其他结束原因。usageMetadata 的 promptTokenCount + toolUsePromptTokenCount 为输入，candidatesTokenCount + thoughtsTokenCount 为输出，缓存 token 已包含在 prompt 中，不再加 totalTokenCount。最终费用以调用日志为准。",
      "Read text from candidates[0].content.parts; thought: true identifies reasoning text. STOP or MAX_TOKENS in finishReason indicates normal completion. Check promptFeedback.blockReason and other finish reasons. In usageMetadata, input is promptTokenCount + toolUsePromptTokenCount and output is candidatesTokenCount + thoughtsTokenCount. Cached tokens are included in the prompt; do not add totalTokenCount again. Request logs determine the final bill.",
      "اقرأ text من candidates[0].content.parts؛ تشير thought: true إلى نص الاستدلال. تعني STOP أو MAX_TOKENS في finishReason نهاية طبيعية. تحقق من promptFeedback.blockReason وأسباب الانتهاء الأخرى. في usageMetadata يساوي الإدخال promptTokenCount + toolUsePromptTokenCount والإخراج candidatesTokenCount + thoughtsTokenCount. يتضمن prompt الرموز المخزنة مؤقتًا؛ لا تضف totalTokenCount مجددًا. تحدد سجلات الطلبات الفاتورة النهائية.",
    ),
    streaming: text(
      "流式调用使用 :streamGenerateContent?alt=sse，并用 curl -N 禁用本地缓冲。按 SSE（服务器发送事件）的 data JSON 读取 parts；检查终态 finishReason，读取最后一个有效 usageMetadata，不能把累计用量逐帧相加。连接在终态前断开属于失败，可能仍有已用量费用。",
      "For streaming, use :streamGenerateContent?alt=sse and curl -N to disable local buffering. Read parts from SSE data JSON. Check the terminal finishReason and retain the final valid usageMetadata instead of summing cumulative reports across frames. A connection broken before a terminal result is a failure and may still incur usage charges.",
      "للبث استخدم :streamGenerateContent?alt=sse وcurl -N لتعطيل التخزين المؤقت المحلي. اقرأ parts من JSON في data الخاص بـSSE. تحقق من finishReason النهائي واحتفظ بآخر usageMetadata صالح بدل جمع التقارير التراكمية بين الإطارات. انقطاع الاتصال قبل النتيجة النهائية فشل وقد يترتب عليه رسوم استخدام.",
    ),
    boundary: text(
      "本地已验证的范围：单候选文本输出、完整函数调用/结果历史及内联图片输入。cachedContent、continuationToken、文件引用、托管工具及其他生成模态尚未开放；续轮保留首条用户消息和完整工具历史。智能模型还要求 gemini 及相应能力元数据，真实供应商验收待完成。",
      "Locally verified scope: one candidate, text output, complete function call/result history, and inline image input. cachedContent, continuationToken, file references, hosted tools, and other output modalities are unavailable. Preserve the first user message and complete tool history on continuation. Smart models require gemini and applicable capability metadata. Real-provider verification remains pending.",
      "النطاق المتحقق محليًا: مرشح واحد وإخراج نصي وسجل كامل لاستدعاءات الدوال ونتائجها وصور إدخال مضمّنة. لا تتوفر cachedContent أو continuationToken أو مراجع الملفات أو الأدوات المستضافة أو أنماط الإخراج الأخرى. احتفظ بأول رسالة مستخدم وسجل الأدوات الكامل عند المتابعة. تتطلب النماذج الذكية بيانات gemini والقدرات اللازمة. لم يكتمل التحقق بالمزود الفعلي.",
    ),
  };
}
