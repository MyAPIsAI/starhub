type Text = (zh: string, en: string, ar: string) => string;

export function gatewayPolicyGuide(text: Text) {
  return {
    quota: text(
      "账户 API 配额与密钥剩余额度分别校验；当前额度不会按日/月自动重置。只有 unlimited_quota=true 才跳过密钥额度检查，账户 API 配额仍须足够。",
      "Account API quota and remaining key quota are checked separately. Quotas do not reset daily or monthly. Only unlimited_quota=true skips the key quota check; account API quota is still required.",
      "تُفحص حصة الحساب وحصة المفتاح المتبقية بشكل منفصل ولا تتجدد يوميًا أو شهريًا. يتجاوز unlimited_quota=true فحص حصة المفتاح فقط؛ وتبقى حصة الحساب مطلوبة.",
    ),
    wallet: text(
      "支付钱包与 API 配额分别记账。当前充值流程使用模拟支付，真实支付网关及可信回调尚未接入；模拟到账不会增加 API 配额。打开收银台或返回页面也不代表支付成功。",
      "The payment wallet and API quota use separate accounting. Recharge currently uses simulated payment; a real payment gateway and trusted callback are not connected. Simulated wallet credit does not increase API quota. Opening a cashier or returning to the page does not confirm payment.",
      "تُسجّل محفظة الدفع وحصة الواجهة بشكل منفصل. الشحن الحالي محاكاة؛ لم تُربط بوابة دفع فعلية وإشعارات موثوقة بعد. رصيد المحاكاة لا يزيد حصة الواجهة، وفتح صفحة الدفع أو العودة منها لا يؤكد الدفع.",
    ),
    defaults: text(
      "下表是当前代码默认值，不是本次部署的实时配置。管理员可修改限流配置，也可覆盖单个模型的限额；以服务返回的限制为准。",
      "These are current code defaults, not live settings for this deployment. Administrators can change rate settings and individual model limits. The service's enforced limits take precedence.",
      "هذه قيم الشيفرة الافتراضية وليست الإعدادات الحية لهذا النشر. يمكن للمسؤول تعديل الحدود العامة وحدود النماذج الفردية؛ والحدود التي تفرضها الخدمة هي المعتمدة.",
    ),
    rateHelp: text(
      "超出速率限制返回 429（rate_limit_exceeded），请遵循 Retry-After 后再试。配额不足返回 403（insufficient_quota 或 token_quota_exhausted）。预扣按本次模型、分组与请求预算估算，未使用差额在结算时退还；失败状态不保证没有已产生的费用。",
      "Rate limits return 429 (rate_limit_exceeded); respect Retry-After before retrying. Insufficient quota returns 403 (insufficient_quota or token_quota_exhausted). Reservations use the model, group and request budget; unused amounts are refunded at settlement. Failed status does not guarantee zero incurred cost.",
      "تُرجع حدود المعدل 429؛ اتبع Retry-After قبل المحاولة. نقص الحصة يُرجع 403. يعتمد الحجز على النموذج والمجموعة وميزانية الطلب، ويُرد غير المستخدم عند التسوية. حالة الفشل لا تعني انعدام التكلفة المتحققة.",
    ),
    tokenFields: [
      [
        "group",
        text(
          "服务分组，决定可用渠道与计费倍率；auto 按 auto_groups 配置的顺序尝试。",
          "Service group selects eligible channels and billing ratios; auto tries auto_groups in configured order.",
          "تحدد مجموعة الخدمة القنوات ومضاعفات الفوترة؛ ويجرب auto مجموعات auto_groups بترتيب الإعداد.",
        ),
      ],
      [
        "auto_groups",
        text(
          "auto 分组的候选分组标识，英文逗号分隔；不能为空。",
          "Comma-separated candidate group IDs for auto; must not be empty.",
          "معرّفات المجموعات المرشحة لـ auto مفصولة بفواصل إنجليزية؛ لا يجوز أن تكون فارغة.",
        ),
      ],
      [
        "model_limits",
        text(
          "授权模型标识，英文逗号分隔。空值不额外限制模型，仍受分组、渠道和能力检查；仅列智能入口不能授权其实际模型。",
          "Comma-separated authorized model IDs. Empty adds no model restriction, while group, channel and capability checks still apply. Listing only a smart alias does not authorize concrete models.",
          "معرّفات النماذج المصرح بها بفواصل إنجليزية. الفراغ لا يضيف تقييدًا، لكن فحوص المجموعة والقناة والقدرات تبقى سارية. الاسم الذكي وحده لا يصرح بالنماذج الفعلية.",
        ),
      ],
      [
        "allow_ips",
        text(
          "来源 IP 或 CIDR 白名单，英文逗号分隔；空值不限制来源。",
          "Comma-separated source IP or CIDR allowlist; empty does not restrict source addresses.",
          "قائمة عناوين IP أو CIDR المسموحة بفواصل إنجليزية؛ الفراغ لا يقيد المصدر.",
        ),
      ],
      [
        "remain_quota",
        text(
          "密钥剩余额度；unlimited_quota=false 时，0 表示已用尽。",
          "Remaining key quota; with unlimited_quota=false, zero means exhausted.",
          "حصة المفتاح المتبقية؛ عندما يكون unlimited_quota=false يعني الصفر نفادها.",
        ),
      ],
      [
        "unlimited_quota",
        text(
          "true 跳过密钥剩余额度检查，不跳过账户 API 配额、权限或限流。",
          "True skips the key quota check, not account API quota, authorization or rate limits.",
          "تتجاوز true فحص حصة المفتاح فقط ولا تتجاوز حصة الحساب أو الصلاحيات أو حدود المعدل.",
        ),
      ],
    ],
    // Defaults mirror config/runtime.go and the middleware rate-limit constants.
    rateRows: [
      [
        text("IP 速率限制", "IP rate limit", "حد معدل IP"),
        "240 req/min",
        text(
          "每个来源 IP 的接口请求。",
          "API requests per source IP.",
          "طلبات الواجهة لكل عنوان IP.",
        ),
      ],
      [
        text("用户速率限制", "User rate limit", "حد معدل المستخدم"),
        "120 req/min",
        text(
          "同一用户的中继请求。",
          "Relay requests for the same user.",
          "طلبات الترحيل للمستخدم نفسه.",
        ),
      ],
      [
        text("密钥速率限制", "Key rate limit", "حد معدل المفتاح"),
        "60 req/min",
        text(
          "同一 API 密钥的中继请求。",
          "Relay requests for the same API key.",
          "طلبات الترحيل للمفتاح نفسه.",
        ),
      ],
      [
        text("模型速率限制", "Model rate limit", "حد معدل النموذج"),
        "200 req/min",
        text(
          "共享模型限流桶；可配置单模型覆盖。",
          "Shared model rate bucket; individual overrides may apply.",
          "حد مشترك للنموذج مع إمكانية إعداد حدود فردية.",
        ),
      ],
    ],
  };
}
