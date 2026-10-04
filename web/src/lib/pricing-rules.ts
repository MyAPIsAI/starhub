import type {
  ModelPricing,
  PricingAttributes,
  ConditionalUnitPrice,
  PricingRule,
  PricingTimeWindow,
} from "../config/models";

export const USAGE_FIELDS = [
  "input_tokens",
  "output_tokens",
  "cache_read_tokens",
  "cache_write_tokens",
  "images",
  "seconds",
  "requests",
] as const;
export type MeteredUsage = Partial<
  Record<(typeof USAGE_FIELDS)[number], number>
>;
type Rational = { n: bigint; d: bigint };
const gcd = (a: bigint, b: bigint): bigint =>
  b ? gcd(b, a % b) : a < 0n ? -a : a;
function rational(n: bigint, d = 1n): Rational {
  if (!d) throw new Error("除数不能为零");
  if (d < 0n) {
    n = -n;
    d = -d;
  }
  const factor = gcd(n, d);
  return { n: n / factor, d: d / factor };
}
const add = (a: Rational, b: Rational) =>
  rational(a.n * b.d + b.n * a.d, a.d * b.d);
const multiply = (a: Rational, b: Rational) => rational(a.n * b.n, a.d * b.d);
const divide = (a: Rational, b: Rational) => rational(a.n * b.d, a.d * b.n);
const compare = (a: Rational, b: Rational) => a.n * b.d - b.n * a.d;
const ceil = (value: Rational) =>
  rational(value.n / value.d + (value.n > 0n && value.n % value.d ? 1n : 0n));
function decimal(value: string): Rational {
  if (!/^\d+(\.\d{1,12})?$/.test(value))
    throw new Error("金额或用量须为普通十进制数字，最多 12 位小数");
  const [whole, fraction = ""] = value.split(".");
  return rational(BigInt(whole + fraction), 10n ** BigInt(fraction.length));
}
const isDecimal = (value: unknown, positive = false) =>
  typeof value === "string" &&
  /^(0|[1-9]\d*)(\.\d{1,6})?$/.test(value) &&
  Number(value) <= 1_000_000 &&
  (positive ? Number(value) > 0 : true);

type Node =
  | { kind: "number"; value: string }
  | { kind: "variable"; name: keyof MeteredUsage }
  | { kind: "binary"; op: string; left: Node; right: Node }
  | { kind: "function"; name: string; args: Node[] };
/** Small arithmetic grammar, deliberately not JavaScript. No property access or code execution. */
function parseExpression(source: string): Node {
  if (typeof source !== "string" || !source.trim() || source.length > 512)
    throw new Error("表达式须为 1–512 个字符");
  const tokens: string[] = [];
  const pattern = /\s*(\d+(?:\.\d{1,12})?|[a-z_]+|[+\-*/(),])/gy;
  let cursor = 0;
  while (cursor < source.length) {
    if (!source.slice(cursor).trim()) break;
    pattern.lastIndex = cursor;
    const match = pattern.exec(source);
    if (!match) throw new Error("表达式包含不允许的字符或操作");
    tokens.push(match[1]);
    cursor = pattern.lastIndex;
  }
  let position = 0,
    depth = 0,
    nodes = 0;
  const take = () => tokens[position++];
  const primary = (): Node => {
    if (++depth > 32 || ++nodes > 256) throw new Error("表达式过于复杂");
    const token = take();
    let node: Node;
    if (token === "+" || token === "-")
      node = {
        kind: "binary",
        op: token,
        left: { kind: "number", value: "0" },
        right: primary(),
      };
    else if (token === "(") {
      node = sum();
      if (take() !== ")") throw new Error("表达式括号不匹配");
    } else if (token && /^\d/.test(token)) {
      decimal(token);
      node = { kind: "number", value: token };
    } else if (USAGE_FIELDS.includes(token as keyof MeteredUsage))
      node = { kind: "variable", name: token as keyof MeteredUsage };
    else if (["min", "max", "ceil"].includes(token)) {
      if (take() !== "(") throw new Error("函数必须使用括号");
      const args = [sum()];
      while (tokens[position] === ",") {
        take();
        args.push(sum());
        if (args.length > 8) throw new Error("函数参数过多");
      }
      if (
        take() !== ")" ||
        (token === "ceil" ? args.length !== 1 : args.length < 2)
      )
        throw new Error("函数参数数量或括号无效");
      node = { kind: "function", name: token, args };
    } else throw new Error(`表达式变量或函数不允许：${token ?? "空"}`);
    depth--;
    return node;
  };
  const product = (): Node => {
    let node = primary();
    while (["*", "/"].includes(tokens[position]))
      node = { kind: "binary", op: take(), left: node, right: primary() };
    return node;
  };
  const sum = (): Node => {
    let node = product();
    while (["+", "-"].includes(tokens[position]))
      node = { kind: "binary", op: take(), left: node, right: product() };
    return node;
  };
  const tree = sum();
  if (position !== tokens.length) throw new Error("表达式存在多余内容");
  return tree;
}
function usageValue(usage: MeteredUsage, field: keyof MeteredUsage): Rational {
  const value = usage[field];
  if (value === undefined) throw new Error(`缺少计费用量：${field}`);
  if (
    !Number.isFinite(value) ||
    value < 0 ||
    value > 1_000_000_000 ||
    (field !== "seconds" && !Number.isSafeInteger(value))
  )
    throw new Error(`无效计费用量：${field}`);
  return decimal(String(value));
}
function evaluate(node: Node, usage: MeteredUsage): Rational {
  if (node.kind === "number") return decimal(node.value);
  if (node.kind === "variable") return usageValue(usage, node.name);
  if (node.kind === "function") {
    const args = node.args.map((arg) => evaluate(arg, usage));
    if (node.name === "ceil") return ceil(args[0]);
    return args.reduce((best, item) =>
      (
        node.name === "min"
          ? compare(item, best) < 0n
          : compare(item, best) > 0n
      )
        ? item
        : best,
    );
  }
  const a = evaluate(node.left, usage),
    b = evaluate(node.right, usage);
  if (node.op === "+") return add(a, b);
  if (node.op === "-") return add(a, rational(-b.n, b.d));
  if (node.op === "*") return multiply(a, b);
  return divide(a, b);
}
function minuteOfDay(value: string, end = false): number {
  if (end && value === "24:00") return 1440;
  if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(value)) return -1;
  const [hour, minute] = value.split(":").map(Number);
  return hour * 60 + minute;
}
function timeWindowError(
  windows: PricingTimeWindow[] | undefined,
): string | null {
  if (windows === undefined) return null;
  if (!Array.isArray(windows) || windows.length > 16)
    return "分时时段最多 16 个";
  const occupied = new Set<number>(),
    ids = new Set<string>();
  let timezone: string | undefined;
  for (const window of windows) {
    if (
      !window ||
      typeof window.id !== "string" ||
      !window.id ||
      ids.has(window.id) ||
      typeof window.label !== "string" ||
      !window.label.trim()
    )
      return "分时时段名称和唯一标识不能为空";
    ids.add(window.id);
    try {
      new Intl.DateTimeFormat("en", { timeZone: window.timezone }).format();
    } catch {
      return "分时时区无效";
    }
    if (!window.timezone || (timezone && timezone !== window.timezone))
      return "同一模型的分时时段须使用同一时区，以准确检查重叠";
    timezone = window.timezone;
    if (
      !Array.isArray(window.days) ||
      !window.days.length ||
      window.days.some((day) => !Number.isInteger(day) || day < 0 || day > 6) ||
      new Set(window.days).size !== window.days.length
    )
      return "请选择有效且不重复的星期";
    if (!isDecimal(window.multiplier, true) || Number(window.multiplier) > 100)
      return "分时系数须大于 0 且不超过 100";
    if (
      window.tokenPrices !== undefined &&
      (!window.tokenPrices ||
        !Object.keys(window.tokenPrices).length ||
        window.multiplier !== "1" ||
        Object.entries(window.tokenPrices).some(
          ([key, price]) =>
            ![
              "input_per_million_usd",
              "output_per_million_usd",
              "cache_read_per_million_usd",
              "cache_write_per_million_usd",
            ].includes(key) || !isDecimal(price),
        ))
    )
      return "分时独立词元价格须有效，且不能再次叠加分时系数";
    const start = minuteOfDay(window.start),
      end = minuteOfDay(window.end, true);
    if (start < 0 || end < 0 || start === end)
      return "分时时段须有有效且不同的起止时间";
    const duration = end > start ? end - start : 1440 - start + end;
    for (const day of window.days)
      for (let offset = 0; offset < duration; offset++) {
        const minute = (day * 1440 + start + offset) % 10080;
        if (occupied.has(minute))
          return "分时时段重叠；同一时刻只能命中一条规则";
        occupied.add(minute);
      }
  }
  return null;
}
export function pricingRuleError(value: unknown): string | null {
  if (!value || typeof value !== "object") return "缺少计价规则";
  const rule = value as PricingRule;
  if (rule.mode === "tokens") {
    /* token bucket rates live in entry.pricing */
  } else if (rule.mode === "unit") {
    if (
      !["request", "image", "second"].includes(rule.unit) ||
      !isDecimal(rule.priceUsd) ||
      !isDecimal(rule.minimum) ||
      !isDecimal(rule.ceilTo)
    )
      return "单位价格、最低计费量及取整步长须为有效非负数字";
    if (
      rule.unit !== "second" &&
      (!Number.isInteger(Number(rule.minimum)) ||
        !Number.isInteger(Number(rule.ceilTo)))
    )
      return "按次或按张计费的最低量、步长须为整数";

    if (rule.conditionRates !== undefined) {
      if (
        !Array.isArray(rule.conditionRates) ||
        rule.conditionRates.length > 24
      )
        return "条件价最多 24 条";
      const ids = new Set<string>();
      for (const rate of rule.conditionRates) {
        if (
          !rate?.id ||
          ids.has(rate.id) ||
          !rate.label?.trim() ||
          !isDecimal(rate.priceUsd) ||
          !rate.when ||
          !Object.keys(rate.when).length
        )
          return "条件价须有唯一标识、名称、有效价格和至少一个条件";
        ids.add(rate.id);
        if (
          Object.entries(rate.when).some(
            ([key, value]) =>
              !["resolution", "quality", "audio"].includes(key) ||
              (key === "audio"
                ? typeof value !== "boolean"
                : typeof value !== "string" ||
                  !value.trim() ||
                  value.length > 50),
          )
        )
          return "条件仅支持分辨率、质量和是否包含音频";
      }
      for (let i = 0; i < rule.conditionRates.length; i++)
        for (let j = i + 1; j < rule.conditionRates.length; j++) {
          const a = rule.conditionRates[i].when,
            b = rule.conditionRates[j].when;
          if (
            (["resolution", "quality", "audio"] as const).every(
              (key) =>
                a[key] === undefined ||
                b[key] === undefined ||
                a[key] === b[key],
            )
          )
            return "条件价存在重叠，请细化条件，确保一次请求只命中一条";
        }
    }
  } else if (rule.mode === "expression") {
    try {
      parseExpression(rule.expression);
    } catch (error) {
      return (error as Error).message;
    }
  } else return "未知计价方式";
  if (
    rule.mode !== "tokens" &&
    rule.timeWindows?.some((window) => window.tokenPrices)
  )
    return "独立词元时段价格只适用于按词元计费，请先移除此时段或改用系数";
  return timeWindowError(rule.timeWindows);
}
export const validatePricingRule = (rule: unknown): rule is PricingRule =>
  pricingRuleError(rule) === null;
export function matchedPricingWindow(
  windows: PricingTimeWindow[] | undefined,
  at: string,
): PricingTimeWindow | null {
  if (!Number.isFinite(Date.parse(at))) throw new Error("计费时间无效");
  for (const window of windows ?? []) {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: window.timezone,
      weekday: "short",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).formatToParts(new Date(at));
    const get = (type: string) =>
      parts.find((part) => part.type === type)!.value;
    const day = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].indexOf(
      get("weekday"),
    );
    const minute = Number(get("hour")) * 60 + Number(get("minute"));
    const start = minuteOfDay(window.start),
      end = minuteOfDay(window.end, true);
    const matches =
      end > start
        ? window.days.includes(day) && minute >= start && minute < end
        : (window.days.includes(day) && minute >= start) ||
          (window.days.includes((day + 6) % 7) && minute < end);
    if (matches) return window;
  }
  return null;
}
/** Exact rational arithmetic through all buckets and coefficients, then one nanodollar rounding. */
export function quotePricingRule(
  rule: PricingRule,
  pricing: ModelPricing,
  usage: MeteredUsage,
  groupMultiplier: string,
  at: string,
  attributes: PricingAttributes = {},
) {
  const invalid = pricingRuleError(rule);
  if (invalid) throw new Error(invalid);
  for (const field of USAGE_FIELDS)
    if (usage[field] !== undefined) usageValue(usage, field);
  const matchedWindow = matchedPricingWindow(rule.timeWindows, at);
  const effectivePricing = { ...pricing, ...matchedWindow?.tokenPrices };
  let base = rational(0n);
  let billedQuantity: string | undefined;
  let matchedCondition: ConditionalUnitPrice | null = null;
  if (rule.mode === "tokens") {
    const buckets: [keyof MeteredUsage, string | undefined, boolean][] = [
      ["input_tokens", effectivePricing.input_per_million_usd, true],
      ["output_tokens", effectivePricing.output_per_million_usd, true],
      ["cache_read_tokens", effectivePricing.cache_read_per_million_usd, false],
      [
        "cache_write_tokens",
        effectivePricing.cache_write_per_million_usd,
        false,
      ],
    ];
    for (const [field, price, required] of buckets) {
      const quantity = required
        ? usageValue(usage, field)
        : usageValue({ [field]: usage[field] ?? 0 }, field);
      if (quantity.n > 0n && price === undefined)
        throw new Error("Unconfigured cache price（未配置缓存单价）");
      if (price !== undefined)
        base = add(
          base,
          divide(multiply(quantity, decimal(price)), rational(1_000_000n)),
        );
    }
  } else if (rule.mode === "unit") {
    const field =
      rule.unit === "request"
        ? "requests"
        : rule.unit === "image"
          ? "images"
          : "seconds";
    let quantity = usageValue(usage, field);
    const minimum = decimal(rule.minimum),
      step = decimal(rule.ceilTo);
    if (compare(quantity, minimum) < 0n) quantity = minimum;
    if (step.n > 0n) quantity = multiply(ceil(divide(quantity, step)), step);
    billedQuantity =
      quantity.d === 1n
        ? quantity.n.toString()
        : `${Number(quantity.n) / Number(quantity.d)}`;
    const rates = rule.conditionRates ?? [];
    for (const dimension of ["resolution", "quality", "audio"] as const) {
      if (
        rates.some((rate) => rate.when[dimension] !== undefined) &&
        attributes[dimension] === undefined
      )
        throw new Error(
          `缺少条件计价参数：${{ resolution: "分辨率", quality: "质量", audio: "是否包含音频" }[dimension]}`,
        );
    }
    matchedCondition =
      rates.find((rate) =>
        Object.entries(rate.when).every(
          ([key, value]) =>
            attributes[key as keyof PricingAttributes] === value,
        ),
      ) ?? null;
    base = multiply(
      quantity,
      decimal(matchedCondition?.priceUsd ?? rule.priceUsd),
    );
  } else base = evaluate(parseExpression(rule.expression), usage);
  if (base.n < 0n) throw new Error("计费表达式结果不能为负数");
  const timeMultiplier = matchedWindow?.multiplier ?? "1";
  const amount = multiply(
    multiply(base, decimal(groupMultiplier)),
    decimal(timeMultiplier),
  );
  if (amount.n < 0n || compare(amount, rational(1_000_000_000_000n)) > 0n)
    throw new Error("计费金额超出有效范围");
  const amountNano =
    (amount.n * 1_000_000_000n * 2n + amount.d) / (amount.d * 2n);
  return {
    amountNano: amountNano.toString(),
    timeMultiplier,
    matchedWindow: matchedWindow ? structuredClone(matchedWindow) : null,
    billedQuantity,
    matchedCondition,
    effectivePricing,
  };
}

/** Explicit example dimensions for quotations, never implicit request defaults. */
export function examplePricingAttributes(
  rule: PricingRule,
): PricingAttributes | undefined {
  if (rule.mode !== "unit" || !rule.conditionRates?.length) return undefined;
  return Object.fromEntries(
    (["resolution", "quality", "audio"] as const).flatMap((dimension) => {
      const value = rule.conditionRates!.find(
        (rate) => rate.when[dimension] !== undefined,
      )?.when[dimension];
      return value === undefined ? [] : [[dimension, value]];
    }),
  );
}
