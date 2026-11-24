# 普通渠道兜底与保护恢复提交审查（2026-11-24）

- **审查对象**：本地提交 `60f729d`（`Add configurable ordinary fallback and guard protection recovery`）
- **上游提交**：`1610653`（`Add staged routing fallback and recover protection after successful calls`）
- **审查性质**：只分析，未修改提交内容
- **结论**：仍有 **2 个已复现的逻辑问题**，以及 **2 项运行边界风险**

---

## 一、提交范围

`60f729d` 共改动 18 个文件，主要内容：

- 在网关配置新增普通兜底开关 `ordinary_fallback` 与受保护兜底最小间隔 `fallback_interval_seconds`。
- 普通兜底允许突破模型流量规则（权重为 0 或未加入规则的渠道可被选中）。
- 新增 `upstream_fallback_slots` 表，对受保护资源的兜底调用做跨副本限频。
- Realtime 正常业务成功后清理本地路由连续失败计数。
- 恢复商业模式首份路由快照门禁及就绪检查。
- 补齐启动配置解析与示例中的上游保护、流量分配配置。

---

## 二、需要修复的逻辑问题

### 1. [高] 429 冷却可能被后到达的其他错误绕过

**位置**

- `internal/upstream/state.go:161`
- `internal/upstream/state.go:301`

**问题**

兜底仅在状态同时满足以下条件时禁止发送：

```go
state.Reason == "rate_limited" && state.OpenUntil > now
```

但同一保护状态后续记录失败时，会直接覆盖 `Reason`：

```go
s.Reason = v.Class
```

**复现场景**

1. 同一渠道、模型、凭证有两个在途请求。
2. 第一个返回 429，设置冷却期限。
3. 第二个较晚返回带 `model_not_found` 的 404，保护原因变为 `model_unavailable`。
4. 冷却期限仍未结束，但普通兜底已被允许发送。

隔离测试结果：剩余保护时间为 **900 秒**，`CaptureFallback` 仍返回成功。

**影响**

限流/额度冷却约束被绕过，可能在冷却期内继续向上游施压，放大 429 风险。

**建议**

将不可绕过的限流期限独立保存（例如 `RateLimitedUntil`），不要用可被覆盖的错误原因判断冷却。

---

### 2. [中] Realtime 成功清理仍不完整，上游失败计数会累积

**位置**

- `relay/realtime_protection.go:83`
- `internal/upstream/state.go:274`

**问题**

本次修复了本地路由层的连续失败计数，但 Realtime 每个业务结果仍复制连接建立时的 `captured`：

```go
attempt := *captured
```

其中 `Versions`、`Failures` 不会随该会话后续失败更新。成功清理上游计数时，可能因初始失败数为 0，或版本已不匹配而跳过。

**复现结果**

同一会话连续经历 5 组「失败 → 成功」，上游状态最终仍为：

```text
failures=5
open=true
```

**影响**

本地路由层看起来已经恢复，上游保护层仍会熔断，两层状态不一致。

**建议**

为 Realtime 业务结果维护会话内的上游版本与失败快照，同时保留对外部较新失败的版本保护。

---

## 三、运行边界风险

### 3. [中] 所有普通兜底请求都会争用全局数据库写锁

**位置**：`internal/upstream/state.go:130`

**问题**

`CaptureFallback` 无条件进入 `write()`，先更新共享的 `upstream_guard`，之后才判断是否需要限频。

这意味着：

- 健康渠道仅因流量规则或平台候选耗尽进入普通兜底时，也要获取全局保护写锁。
- 限频被拒绝的请求同样需要获取它。

**影响**

故障期间大量请求进入兜底时，可能与失败记录、恢复及探测竞争，增加数据库排队和延迟。尚未通过压测量化影响。

**建议**

健康且未跨越保护的兜底保留只读准入；受保护的兜底再进入事务复查并占用限频槽。

---

### 4. [低] Realtime 发送前仍可能使用旧兜底配置

**位置**

- `relay/realtime.go:107`
- `relay/realtime.go:169`

**问题**

Realtime 在请求开始时保存 `policy`，候选选择读取实时配置，发送准入却使用请求开始时的旧配置。

**影响**

若关闭兜底或增大限频间隔发生在「选中候选」和「发送准入」之间，该在途请求仍可能按旧策略发送。

**建议**

明确配置只影响新请求；或在发送前重新读取兜底开关和限频间隔，使其与普通 HTTP 路径保持一致。

---

## 四、验证情况

| 检查项 | 结果 |
| --- | --- |
| 提交中的针对性回归测试 | 通过 |
| 第 1、2 项复现（Go overlay 隔离测试） | 已复现，未改动工作区 |
| `go build ./...`、`go vet ./...` | 通过 |
| 前端类型检查与生产构建 | 通过 |
| 网关配置页浏览器验证 | 通过（中文桌面、英文/阿拉伯文窄屏，含键盘切换、保存、刷新回显） |
| 全量 Go 测试 | 仍有 34 个顶层失败 |

**关于全量测试失败**

上述 34 个顶层失败已在未修改的父提交 `1610653` 基线副本中复现，不能归因于本次提交，也不能视为全量通过。涉及重试断言、日志计数、流式响应和探测等既有路径。

---

## 五、优先建议

1. **优先修复第 1 项**：独立保存限流期限，防止 429 冷却被覆盖绕过。
2. **优先修复第 2 项**：补齐 Realtime 会话内的上游失败计数清理与版本保护。
3. 评估第 3 项的写锁争用，必要时改为只读准入 + 受保护兜底事务复查。
4. 明确第 4 项配置热更新的生效边界。

---

## 六、修复方案分析（原文追加）

对照评审记录和当前代码，建议优先修复 **429 冷却与 Realtime 状态更新**，配置热更新可同步修正，写锁优化单独处理。本轮仅阅读和分析，未修改文件，也未重新运行测试。

1. **429 冷却：独立保存期限，并覆盖完整状态生命周期**

   当前 [state.go](/D:/Workspace/StarHub/internal/upstream/state.go:149) 把 `Reason` 同时用于错误展示和冷却判断，而后续失败会覆盖它。仅阻止覆盖 `Reason` 容易影响诊断，建议新增 `RateLimitedUntil`：

   - 收到 `rate_limited` 时，更新为 `max(原期限, 当前时间 + verdict.Delay)`。
   - 其他错误可以更新 `Reason`、`OpenUntil`，但不能缩短或清除限流期限。
   - 普通兜底直接检查 `RateLimitedUntil > now`，检查应独立于 `Reason` 和 `OpenUntil`。

   **期限必须来自这次 429 的延迟，不能复制混合保护期限 `OpenUntil`。** 例如 429 要求冷却 30 秒，后到的模型错误把保护延长到 900 秒，二者应分别记录。

   需要同时检查以下路径：

   - `CompleteProbe`：探测遇到 429，包括产生其他作用域保护时，也必须记录独立期限。
   - `Claim`、`claimSlot`、`ProbeAdmitted`：不能提前穿过有效限流期限。
   - `recoverFallback`：在途成功不能提前清除有效冷却，原有版本保护仍需保留。
   - 探测成功、人工 `Reset`：明确清理新字段，其中人工重置继续作为显式解除保护的入口。

   **迁移不能只加字段。** 现有 `Reason == rate_limited` 的行可以保守地用 `OpenUntil` 回填；原因已经被覆盖的历史行无法仅靠状态表准确还原，需要从事件记录重建，或者设计短期保守兼容策略。

   此外，当前分类器会把部分带额度错误码的 429 归为 `quota_exhausted`。本次应先修复 `rate_limited` 的既有语义；是否把额度隔离也变成不可绕过保护，应另行明确。

2. **Realtime：维护会话版本，但只能跟随自己提交的状态变化**

   根因位于 [realtime_protection.go](/D:/Workspace/StarHub/relay/realtime_protection.go:83)：每个业务结果复制连接建立时的 `captured`，后续失败没有更新会话中的 `Versions`、`Failures`。

   推荐增加会话状态，并让上游记录逻辑返回“本次事务实际提交的状态变化”：

   - 每个业务结果仍有独立事件 ID。
   - 失败提交成功后，更新对应作用域的会话版本与失败数。
   - 后续成功使用该快照做版本条件更新，清理未熔断的连续失败。
   - 深拷贝 `Attempt` 中的 map，避免浅拷贝共享修改。
   - 事务失败、回滚或事件重复时，不能推进会话快照。

   **不能在成功到达时重新读取最新状态，再拿最新版本执行清理。** 那会把其他请求刚写入的失败一并清除。

   更严格的更新规则是：只有事务更新前的版本仍等于会话预期版本，才允许会话跟随这次变化。若已出现外部版本变化，应使该作用域本轮恢复资格失效，不能把外部失败吸收到会话快照里。

   同时保留恢复边界：

   - 普通会话成功只清理尚未打开的连续失败。
   - 已打开的保护继续遵循探测或已授权的兜底恢复流程。
   - 不要为了修复计数，自动把后来出现的所有保护加入 `Protected`。
   - 如果支持同一连接多个业务响应交错，还需按响应标识关联开始时快照，防止较早响应的成功清理较晚响应的失败。

3. **写锁争用：增加只读快路径，保留受保护准入的事务复查**

   [CaptureFallback](/D:/Workspace/StarHub/internal/upstream/state.go:130) 当前无条件进入 `write()`。可以调整为：

   - 先只读检查状态；有效 429 冷却直接拒绝。
   - 没有上游保护、也没有平台保护时，直接返回捕获快照。
   - 需要跨越保护时，再进入写事务，重新检查状态并原子占用限频槽。

   锁外检查只能用于提前返回；**真正的受保护放行必须在事务内重新判断**。多个副本争用同一凭证时，仍须保证只有一个成功。

   这里存在明确取舍：健康快路径采用与普通 `Capture` 一致的“读取时准入”语义，读取后仍可能出现新的失败。如果要求准入与并发失败严格串行，就不能完全消除协调成本。建议把这项作为独立优化，用并发测试和延迟数据判断收益。

4. **配置热更新：发送准入读取当前策略，并明确限频间隔的生效方式**

   [Realtime 发送准入](/D:/Workspace/StarHub/relay/realtime.go:169) 应像普通 HTTP 路径一样，传入发送前读取的 `upstream.CurrentPolicy()`。

   请求开始时保留重试次数、总重试窗口快照即可；兜底开关和准入间隔在每次实际拨号前更新。已经建立的 WebSocket 无需因关闭兜底而中断。

   但评审中的“增大间隔即时生效”还涉及 [FallbackSlot](/D:/Workspace/StarHub/internal/upstream/fallback.go:20)：当前只保存 `NextAttempt`，其中已经包含旧间隔。**仅重新读取配置，不能延长已有槽位的期限。**

   两种可选语义：

   | 生效方式 | 实现 |
   |---|---|
   | 新间隔用于下一次成功占槽 | 保留现有字段，明确已有期限保持原值 |
   | 增大间隔立即约束下一次发送 | 保存 `LastAttempt`，按当前间隔重新计算准入期限，并兼容旧槽位 |

   若希望后台增大间隔后立即降频，建议采用第二种。

建议实施时先完成第 1、2 项，再处理第 4 项，最后独立优化第 3 项。验收至少覆盖：

| 场景 | 预期 |
|---|---|
| 429 后到达模型错误，或再次收到更短的 429 | 原限流期限不缩短，期间兜底被拒绝 |
| 冷却到期，但其他保护仍存在 | 可以按限频规则兜底，普通流量仍受保护 |
| 同一 Realtime 会话连续 5 组失败→成功 | 未熔断状态的失败计数归零 |
| 会话成功前插入外部失败 | 外部失败保留 |
| 事务回滚、重复事件 | 会话快照不错误推进 |
| 多副本并发兜底、发送前关闭兜底 | 限频只放行一个；关闭后准入被拒绝 |

修复后再运行针对性测试及 `go build ./...`、`go vet ./...`、`go test ./...`。评审记录中的 34 个基线失败应继续单独对照，不能据此宣称全量测试通过。

---

## 七、修复实施记录（2026-11-24）

本节记录按上述建议已落地的实现，改动尚未提交。

### 1. 429 冷却独立保存

- `upstream.State` 新增 `RateLimitedUntil`（带索引）。
- `RecordCommitted` 仅在 `Class == "rate_limited" && Immediate` 时写入 `max(原期限, created + Delay)`，其他失败不会缩短或清除。
- 普通兜底 `capture` 直接检查 `RateLimitedUntil`，不再依赖可被覆盖的 `Reason`/`OpenUntil`。
- `Blocked`、`Claim`、`claimSlot`、`ProbeAdmitted`、`CurrentUpstreamStates` 均加入限流期限判断。
- `CompleteProbe` 失败时记录独立期限（含跨作用域宽化），成功时清除。
- `recoverFallback` 与 `FallbackRecoveryCurrent` 在有效限流期内拒绝提前恢复。
- `Reset` 显式清除该字段。
- 新增 `migrateRateLimitedUntil`：对 `reason = rate_limited` 的行用 `OpenUntil` 回填；对原因已被覆盖的行，用最近 `rate_limited` 事件结合当前 `rate_limit_seconds` 做保守重建。

### 2. Realtime 会话版本跟随

- `upstream.RecordCommitted` 返回本次事务实际提交的 `Versions`、`Failures`、`Stale`。
- 失败提交成功后更新会话版本；成功清理使用会话快照做版本条件更新。
- 外部版本变化标记为 `Stale`，会话永久停止跟随该作用域，避免吸收外部失败。
- 深拷贝 `Versions/Failures/Protected/Recovered`，Record 不会改写会话状态。
- 重复事件返回空提交，事务失败不推进快照。

### 3. 兜底只读快路径

- `CaptureFallback` 先做只读 `capture`。
- 无上游保护且无平台跨越时直接返回，不进入全局写事务。
- 需要跨越保护时才进入 `write`，重新判定并原子占用 `upstream_fallback_slots`。

### 4. 发送前读取当前策略与间隔

- Realtime 每次拨号前读取 `upstream.CurrentPolicy()` 做发送准入；重试预算仍使用请求开始快照。
- `FallbackSlot` 新增 `LastAttempt`，占槽时按当前间隔从 `LastAttempt` 重算期限；间隔增大可立即约束下一次发送，且不会缩短既有期限。

### 验证

- 新增回归测试覆盖：限流不被覆盖、快路径不写槽位、间隔增大延长期限、提交只跟随自身版本、重复事件不推进、Realtime 交替失败成功归零、外部失败保留。
- `go build ./...`、`go vet ./...` 通过。
- `internal/upstream`、`relay`、`config` 全量测试通过。
- `model` 定向测试通过；仅剩 `TestScoredRoutingConstraintsSurviveStaleSnapshot`、`TestLocalCircuitHalfOpenAndLateSuccessFence` 两个既有基线失败。
- `internal/router` 的 `TestPlaygroundUsesExistingRelayAuthorizationAndBilling` 仍为基线 `usage logs: 7` 失败，未新增偏差。

---

## 八、实现交付报告（原样记录）

已按文档第七节建议完成实现，改动尚未提交。

**1. 429 冷却独立化（`internal/upstream/state.go`）**
- `State` 新增 `RateLimitedUntil`，不再依赖会被覆盖的 `Reason`/`OpenUntil`。
- `RecordCommitted` 用本次 429 的 `Delay` 写入；后续其他失败无法缩短或清除。
- `Blocked`、`Claim`、`claimSlot`、`ProbeAdmitted`、`CurrentUpstreamStates`、`recoverFallback`、`FallbackRecoveryCurrent` 均新增限流期限判断；`Reset`、探测成功显式清理。
- 新增 `migrateRateLimitedUntil`：`reason=rate_limited` 行用 `OpenUntil` 回填，原因被覆盖的行用近期事件保守重建。

**2. Realtime 失败计数（`relay/realtime_protection.go`、`internal/upstream/state.go`）**
- 新增 `RecordCommitted`，返回本次事务真正提交的版本/失败数/失效作用域。
- `realtimeProtectionSession` 深拷贝快照，只跟随自身提交；外部版本变化标记 `Stale` 并停止跟随，外部失败不会被成功清理。
- 重复事件、事务失败不推进会话快照。

**3. 兜底只读快路径（`internal/upstream/state.go:CaptureFallback`）**
- 健康兜底直接只读返回，不再获取全局写锁。
- 仅跨越上游/平台保护时才进入写事务，重新判定并原子占用限频槽。

**4. 发送前读取当前配置（`relay/realtime.go`、`internal/upstream/fallback.go`）**
- Realtime 每次拨号前读取 `CurrentPolicy()`；重试预算仍用请求开始时快照。
- `FallbackSlot` 新增 `LastAttempt`，间隔增大可立即约束下一次发送，且不缩短既有期限。

**验证**
- 新增回归测试：限流不被覆盖、快路径不写槽位、间隔增大延长、提交只跟随自身版本、重复事件不推进、Realtime 交替失败成功归零、外部失败保留。
- `go build ./...`、`go vet ./...` 通过；`internal/upstream`、`relay`、`config` 全量通过。
- `model` 定向测试通过，仅剩两个既有基线失败；`internal/router` 仍为原有 `usage logs: 7` 基线失败，未新增偏差。
- 实施记录已补入 `tmp/upstream-fallback-commit-review-2026-11-24.md` 第七节。

需要我提交到本地吗？