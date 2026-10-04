import type { PricingQuoteSnapshot } from "../lib/model-pricing";
import { usePreferences } from "../lib/preferences";

export function PricingSnapshot({ snapshot }: { snapshot: PricingQuoteSnapshot }) {
  const language = usePreferences((state) => state.language);
  const text = (zh: string, en: string, ar: string) => language === "zh" ? zh : language === "ar" ? ar : en;
  const { rule, usage, pricing } = snapshot;
  const labels = {
    input_tokens: text("普通输入词元", "Uncached input tokens", "رموز الإدخال"),
    output_tokens: text("输出词元", "Output tokens", "رموز الإخراج"),
    cache_read_tokens: text("缓存读取词元", "Cache read tokens", "رموز قراءة التخزين"),
    cache_write_tokens: text("缓存写入词元", "Cache write tokens", "رموز كتابة التخزين"),
    images: text("图片张数", "Images", "الصور"),
    seconds: text("时长（秒）", "Duration (seconds)", "المدة بالثواني"),
    requests: text("请求次数", "Requests", "الطلبات"),
  };
  const unit = rule.mode === "unit" ? { request: text("次", "request", "طلب"), image: text("张", "image", "صورة"), second: text("秒", "second", "ثانية") }[rule.unit] : "";
  return <>
    <div><dt>{text("计费用量快照", "Metered usage snapshot", "لقطة الاستخدام")}</dt><dd>{Object.entries(usage).map(([field, amount]) => <div key={field}>{labels[field as keyof typeof labels]}：{amount?.toLocaleString()}</div>)}</dd></div>
    <div><dt>{text("调用时计价规则", "Pricing rule at request", "قاعدة التسعير وقت الطلب")}</dt><dd>
      {rule.mode === "tokens" ? <>
        {text("按词元 · 美元 / 百万词元基准价", "Per token · base USD / 1M tokens", "لكل رمز · السعر الأساسي لكل مليون رمز")}
        <small className="request-rate-note">{text("普通输入 / 输出", "Input / output", "إدخال / إخراج")}：${pricing.input_per_million_usd} / ${pricing.output_per_million_usd}</small>
        <small className="request-rate-note">{text("缓存读取 / 写入", "Cache read / write", "قراءة / كتابة التخزين")}：{pricing.cache_read_per_million_usd === undefined ? "—" : `$${pricing.cache_read_per_million_usd}`} / {pricing.cache_write_per_million_usd === undefined ? "—" : `$${pricing.cache_write_per_million_usd}`}</small>
      </> : rule.mode === "unit" ? <>
        ${snapshot.matchedCondition?.priceUsd ?? rule.priceUsd} / {unit}
        {snapshot.matchedCondition && <small className="request-rate-note">{text("条件价", "Conditional price", "السعر المشروط")}：{snapshot.matchedCondition.label} · {JSON.stringify(snapshot.attributes)}</small>}
        <small className="request-rate-note">{text("最低计费量 / 取整步长", "Minimum / rounding step", "الحد الأدنى / خطوة التقريب")}：{rule.minimum} / {rule.ceilTo}</small>
        <small className="request-rate-note">{text("结算数量", "Billed quantity", "الكمية المفوترة")}：{snapshot.billedQuantity} {unit}</small>
      </> : <><span>{text("表达式基准金额（美元）", "Expression base amount (USD)", "المبلغ الأساسي بالتعبير")}</span><code style={{display:"block",overflowWrap:"anywhere"}} dir="ltr">{rule.expression}</code></>}
    </dd></div>
    <div><dt>{text("本次结算系数", "Settlement coefficients", "معاملات التسوية")}</dt><dd>
      {text("分组", "Group", "المجموعة")} {snapshot.groupMultiplier}× · {text("时段", "Time window", "الفترة")} {snapshot.timeMultiplier}×
      <small className="request-rate-note">{text("基准金额 × 分组倍率 × 时段系数，各计算一次。", "Base amount × group × time coefficient, each applied once.", "المبلغ الأساسي × معامل المجموعة × الفترة، مرة واحدة لكل منها.")}</small>
    </dd></div>
    <div><dt>{text("计价时刻与时区", "Pricing time and timezone", "وقت التسعير والمنطقة")}</dt><dd>
      {new Date(snapshot.quotedAt).toLocaleString()}
      <small className="request-rate-note">{snapshot.matchedWindow ? `${snapshot.matchedWindow.label} · ${snapshot.matchedWindow.timezone} · ${snapshot.matchedWindow.start}–${snapshot.matchedWindow.end}` : text("未命中分时时段，使用常规价", "No window matched; standard rate", "لم تطابق فترة؛ السعر العادي")}</small>
    </dd></div>
    <div><dt>{text("价格来源", "Price source", "مصدر السعر")}</dt><dd>
      {snapshot.source === "sample" ? text("演示样例，非核实官价", "Demo sample, not verified official pricing", "عينة عرض وليست سعرًا رسميًا موثقًا") : text("管理员录入的官方参考价，未自动核实", "Official reference entered by admin; not independently verified", "مرجع رسمي أدخله المسؤول دون تحقق آلي")}
      <small className="request-rate-note">{snapshot.version}</small>
    </dd></div>
  </>;
}
