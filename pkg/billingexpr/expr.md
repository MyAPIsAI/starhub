# billingexpr 计费表达式

`pkg/billingexpr` 是 StarHub 的表达式计价引擎，用于「阶梯价 / 动态价」模型：
管理员为某个模型配置一条表达式，请求结算时以 usage 为变量求值，结果即该次
调用的 USD 价格。

## 设计约束（实现与调用方都必须遵守）

1. **纯函数、确定性**：表达式只能引用调用方传入的变量与白名单函数，
   禁止任何 IO、时间、随机数、网络或全局状态。同一输入永远得到同一结果，
   否则预扣与结算无法对账。
2. **长度上限**：单条表达式 ≤ 512 字符（`MaxExprLength`）。超长在
   `UpdateOption` 阶段直接拒绝，绝不进入运行时。
3. **深度上限**：解析递归深度 ≤ 64（`MaxExprDepth`），防止括号炸弹。
4. **步数上限**：单次求值 AST 节点访问 ≤ 10000（`MaxExprSteps`），
   防止超长表达式拖慢结算路径。
5. **函数白名单**：仅 `min` `max` `floor` `ceil` `round` `abs`。
   任何其他标识符（除变量与 `true`/`false`）都是解析或求值错误。
   禁止用户自定义函数、禁止属性访问、禁止字符串操作。
6. **只做数值运算**：所有值都是 float64，布尔值用 1/0 表示；
   比较运算符返回 1/0。除零 / 模零是求值错误。
7. **结果责任在调用方**：表达式结果（USD）允许为 0（免费模型），但
   负数 / NaN / Inf 由计价层 `common.QuotaFromFloat64` 饱和截断并打
   `quota_saturation` 审计标记，绝不会变成负配额「送钱」。
8. **先验证后启用**：表达式只能在 `setting.UpdatePricing`（选项热更新）
   时写入，写入前必须通过 `Validate`（完整解析一遍）；运行时 `Eval`
   遇到任何语法/约束错误直接返回错误，计价层回退到 token 计费并审计。

## 语法

```
expr    := ternary
ternary := or ("?" ternary ":" ternary)?          // 三元，阶梯价的基础
or      := and ("||" and)*
and     := cmp ("&&" cmp)*
cmp     := add (("<"|">"|"<="|">="|"=="|"!=") add)*
add     := mul (("+"|"-") mul)*
mul     := unary (("*"|"/"|"%") unary)*
unary   := ("-"|"!") unary | primary
primary := NUMBER | IDENT ("(" args ")")? | "(" expr ")"
```

- `NUMBER`：十进制字面量（可带小数）。
- `IDENT`：变量名（调用方给定）或白名单函数名，`true`/`false` 等价 1/0。

## 变量（由调用方按模式传入）

引擎本身不限制变量名（未声明变量求值时报错），扩展变量只需调用方
传入。以下变量由各计费路径提供：

### 通用变量（所有模式）

| 变量 | 含义 | 预扣代入 | 结算代入 |
| --- | --- | --- | --- |
| `prompt_tokens` | 上游 usage 的 prompt tokens | 估算值 | 实际 usage |
| `completion_tokens` | 上游 usage 的 completion tokens | 预算值 | 实际 usage |
| `total_tokens` | prompt + completion | 估算 + 预算 | 实际 usage |

### 图片变量（图片生成 / 编辑路径）

| 变量 | 含义 | 预扣代入 | 结算代入 |
| --- | --- | --- | --- |
| `count` | 交付图片张数 | 请求 `n` | 响应实际张数（守卫到 `[1, MaxImageN]`） |
| `size` | 长边像素（`1024`/`1792`/`1536`/`256`/`512`/...） | 请求 size | 请求 size（上游通常不回传） |
| `quality` | 枚举数值映射：`low=0.5 standard=1 medium=1 high=1.5 hd=2 auto=1` | 请求 quality | 请求 quality |

### 视频变量（视频任务路径）

| 变量 | 含义 | 预扣代入 | 结算代入 |
| --- | --- | --- | --- |
| `seconds` | 视频时长（秒） | 请求 `seconds` | 终态实际时长（若上游返回，否则请求值） |
| `n` | 视频路数 | 请求 `n` | 终态实际路数（若上游返回，否则请求值） |
| `task_status` | 终态数值：`success=1 cancelled=0.5 failed=0` | `1`（按成功预扣） | 终态映射 |

预扣阶段没有真实 usage，计价层用「prompt 估算值 / completion 预算值」
代入同一表达式求预扣额（宁多勿少，结算多退少补）。图片 / 视频路径的
预扣代入请求参数上限，结算代入实际交付值。

**图片路径约束**：配置了 `model_price_expr` 的图片模型，表达式结果即
完整的 USD 单次价格，**不再叠加** size/quality/count 作为 `OtherRatio`。
运营若需保留 size 倍率，在表达式里显式写出（如
`0.04 * count * (size == 1792 ? 2.025 : 1)`）。

**视频路径约束**：配置了 `model_price_expr` 的视频模型，终态折扣
（取消半价、失败全退）通过 `task_status` 变量交给表达式，适配器不再
硬编码 `Units *= 0.5`。

## 示例

阶梯价（前 1k completion 免费，1k–10k 半价，超出全价）：

```
completion_tokens <= 1000 ? 0
: completion_tokens <= 10000 ? 0.002 + 0.000001 * (completion_tokens - 1000)
: 0.008 + 0.000002 * (completion_tokens - 10000)
```

按量折扣（每 1k tokens 递减单价，min 兜底）：

```
max(0.001, 0.002 - 0.0000005 * total_tokens)
```
