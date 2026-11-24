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

---

## 九、最新提交复核分析（原文追加）

对比文档第六至八节与最新提交 **`b9bb7a2`（Harden rate-limit cooldown and realtime protection recovery）**，结论是：**主要修复已落地，但还不能认定全部修好；本轮复现了 4 个遗漏问题。**

| 文档要求 | 当前情况 |
|---|---|
| 429 冷却独立保存 | 新产生的失败已修复；历史状态迁移仍有问题 |
| Realtime 交替失败、成功不累积 | 常规场景已修复；特定并发顺序仍会误清外部失败 |
| 健康兜底避免全局写锁 | 已实现只读快路径，受保护路径保留事务复查 |
| 发送前读取当前配置 | 已实现；新槽位支持按当前间隔计算期限，旧槽位保留原期限 |

以下按影响说明。

1. **[高] 旧数据库升级后，新字段为 `NULL`，冷却回填可能完全失效**

   位置：[字段定义](/D:/Workspace/StarHub/internal/upstream/state.go:63)、[迁移条件](/D:/Workspace/StarHub/model/upstream_migration.go:21)、[事件回填条件](/D:/Workspace/StarHub/model/upstream_migration.go:50)。

   `RateLimitedUntil` 没有默认值或非空约束。SQLite 给旧表增加该列后，历史行的值为 `NULL`。

   迁移却分别使用：

   ```sql
   rate_limited_until = 0
   rate_limited_until < ?
   ```

   两个条件都不会匹配 `NULL`。读取到 Go 结构体后又表现为 `0`，因此普通兜底认为没有限流冷却。

   **隔离复现结果：**从旧表结构升级，原状态仍为 `rate_limited`，保护剩余 600 秒；迁移后新字段仍为 `NULL`，`CaptureFallback` 返回成功。

   **修复方向：**明确处理存量 `NULL`，再执行期限回填；字段增加合理的默认值和非空约束，不能只修改查询条件。

2. **[中] Realtime 初始没有状态行时，仍可能吸收并清除外部失败**

   位置：[RecordCommitted 版本判断](/D:/Workspace/StarHub/internal/upstream/state.go:379)。

   当前判断是：

   ```go
   expected, known := a.Versions[s.ID]
   if known && expected != prior {
       committed.Stale[s.ID] = true
   }
   ```

   如果连接建立时没有对应状态行，`known` 就是 `false`。即使状态后来已由外部请求创建，也不会标记 `Stale`，反而会跟随包含外部失败的新版本。

   复现顺序：

   - Realtime 会话建立，对应状态不存在。
   - 外部请求失败，创建版本 1。
   - 会话自身失败，将状态写成版本 2，并跟随该版本。
   - 会话随后成功，将两次失败全部清零。

   **已通过实际 `recordRealtimeBusiness` 调用复现：**`invalid=false`，最终 `failures=0`，外部失败被清除。

   **修复方向：**“初始不存在”也必须是明确的快照状态。只有数据库更新前的状态仍然不存在，才能视为自身首次创建；不能把“快照没有版本”理解为允许跟随任意版本。

3. **[高] 历史长 `Retry-After` 会在迁移中丢失**

   位置：[迁移窗口与期限计算](/D:/Workspace/StarHub/model/upstream_migration.go:35)。

   当前只扫描最近 `rate_limit_seconds` 秒的事件，并使用：

   ```text
   事件时间 + 当前 rate_limit_seconds
   ```

   重建期限。它没有使用历史 429 事件已经保存的 `OpenUntil`，也没有保留原来的显式延迟。

   例如默认限流配置为 30 秒，但上游返回 `Retry-After: 600`。一分钟后错误原因被覆盖，此时升级：

   - 429 事件已经超出 30 秒扫描窗口。
   - 实际冷却仍剩 540 秒。
   - 迁移没有重建冷却，兜底直接放行。

   **上述场景已复现。** 即使先修复第 1 项的 `NULL`，这一问题仍然存在。

   **修复方向：**结合历史事件保存的保护期限保守恢复，扫描范围应覆盖仍可能有效的显式冷却；同时检查其后是否发生有效重置或恢复，不能只套用当前默认配置。

4. **[中] 每次启动执行迁移，会重新施加已人工清除的冷却**

   位置：[启动调用](/D:/Workspace/StarHub/model/main.go:206)、[事件回填](/D:/Workspace/StarHub/model/upstream_migration.go:50)。

   该迁移每次初始化数据库都会执行，事件回填没有检查后续的人工重置或恢复记录，也没有限制仅处理尚未迁移的历史状态。

   **已复现：**

   - 产生一次 429。
   - 调用 `Reset`，成功清除保护。
   - 在事件仍处于扫描窗口内模拟下一次启动。
   - 迁移重新写入剩余约 29 秒的限流期限。

   此时状态变成：

   ```text
   reason=manual_reset
   open_until=0
   rate_limited_until>now
   ```

   还会产生准入不一致：直接调用普通 `Capture` 可以通过，而 `CaptureFallback` 被拒绝。

   **修复方向：**让历史回填成为有明确执行标记的迁移，并尊重之后发生的重置、恢复。正常启动不应反复重放旧失败。

还有两项边界需要保留：

- **交错业务响应仍未按响应标识隔离。** 当前仍按终态帧到达顺序推进会话快照，没有实现文档提到的“按响应标识关联开始时快照”。若允许同一连接多个响应交错，这项保证仍未完成；本轮未将其列为已复现问题。
- **间隔增大的测试证明力不足。** 现有测试里的旧 `NextAttempt` 本来就没有到期，即使去掉新计算逻辑也会被拒绝。应补充“旧期限已过、按新间隔计算仍未到期”的场景。当前实现本身包含这一计算。

本轮验证结果：

| 检查 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go test ./internal/upstream ./relay ./config` | 通过 |
| 上述 4 类问题的 Go overlay 隔离复现 | 均复现 |
| 全量 `go test ./...` | 本轮未运行 |

建议先修复 **迁移的 `NULL` 处理和 Realtime 初始缺失版本判断**，再补齐历史冷却重建及迁移执行边界。现有测试通过，尚不足以支持文档中“外部失败不会被清理”和“历史冷却已保守重建”的完整结论。

本轮未修改仓库文件；复现用例仅放在系统临时目录，通过 Go overlay 执行。

---

## 十、复核问题修复方案分析（原文追加）

已将上一条分析原样追加为[文档第九节“最新提交复核分析”](/D:/Workspace/StarHub/tmp/upstream-fallback-commit-review-2026-11-24.md:342)，`git diff --check` 通过。代码未修改。

建议将修复分为两组：**重做冷却迁移，统一解决三个迁移问题；补齐 Realtime 的初始状态判断，解决误清外部失败。**

1. **先修正字段及迁移顺序，兼容已经升级过的数据库**

   涉及 [State 定义](/D:/Workspace/StarHub/internal/upstream/state.go:63) 和 [数据库初始化](/D:/Workspace/StarHub/model/main.go:193)。

   `RateLimitedUntil` 应使用非空、默认零的字段约束，但不能只加标签。已有数据库可能已经存在该列，且包含 `NULL`；直接收紧约束可能导致迁移失败。

   推荐顺序：

   - 检查表和字段是否已存在；存在时先把存量 `NULL` 归零。
   - 执行带 `not null;default:0` 的结构迁移。
   - 再执行历史期限重建。
   - 任一步失败，都应阻止该实例进入服务状态。

   `FallbackSlot.LastAttempt` 也可统一为非空默认零，旧槽位继续保留原 `NextAttempt`，不猜测历史发送时间。

   需要覆盖三条升级路径：旧版本没有该列、`b9bb7a2` 已产生空值、全新数据库。

2. **重写历史期限重建，不能继续使用“事件时间加当前默认间隔”**

   涉及 [migrateRateLimitedUntil](/D:/Workspace/StarHub/model/upstream_migration.go:18)。

   推荐以事件保存的期限为依据：

   - 对仍显示 `rate_limited`、仍处于保护中的旧状态，可以使用 `OpenUntil` 保守回填。
   - 对原因已被覆盖的状态，查找对应资源仍可能有效的限流事件，使用事件的 `OpenUntil`。
   - 不再按当前 `rate_limit_seconds` 截断扫描窗口，也不重新套用当前配置计算历史期限。
   - 探测事件还需要识别 `Kind == "probe"`、`Decision == "rate_limited"`；不能只检查 `Class == "rate_limited"`。

   **旧事件的 `OpenUntil` 可能混有其他保护，因此只能作为保守期限，无法保证精确还原原始 `Retry-After`。** 这与正常请求不同：正常请求仍应继续使用本次 429 的实际延迟。

   后续事件可以增加精确的限流期限和状态版本，跨作用域探测也应记录实际目标资源，避免以后再次依赖猜测。

3. **将历史重建改为有版本标记的一次性迁移，并尊重恢复记录**

   这是防止重启后复活冷却的关键，不能仅靠“只更新零值”解决。

   建议增加专用迁移记录，例如固定版本标识 `upstream_rate_limit_deadline_v2`。采用新版本标识，是为了让已经运行过错误迁移的数据库也能得到修复。

   执行时：

   - 在事务内取得上游保护写锁。
   - 检查该迁移是否已完成。
   - 重建需要修复的历史状态。
   - 数据更新和完成标记一起提交；失败则一起回滚。

   历史事件重放还应遵循：

   - 当前已经关闭的保护，不因旧失败重新打开。
   - 明确的人工重置、成功探测、成功兜底恢复，应使更早的限流证据失效。
   - `fallback_recovery_skipped` 不能当作成功恢复。
   - 已有可靠的独立期限，不应被历史推算随意改写。

   当前事件时间只有秒级，无法严格判断同秒事件的先后。不能拿 UUID 或请求开始时间当结果提交顺序。对于这种旧数据歧义，应明确采用保守策略：已关闭状态不复活；仍打开且证据不足的状态，才考虑沿用现有保护期限作为上界。

   如果迁移会与其他副本并发，应同步维护状态版本、探测租约，确保旧探测结果不能覆盖迁移后的保护。

4. **Realtime 显式记录“初始不存在”，取消未知版本的通配行为**

   涉及 [Capture](/D:/Workspace/StarHub/internal/upstream/state.go:164)、[RecordCommitted](/D:/Workspace/StarHub/internal/upstream/state.go:379)。

   当前只为已经存在的状态填写 `Versions`，导致“最初不存在”和“快照无效”都表现为缺少 map 项。

   推荐在捕获时，为四个作用域都建立明确快照：

   - 已存在：记录实际版本。
   - 不存在：明确记录不存在。若保证持久化版本从 1 开始，可以用版本 0 表示；否则增加 `Exists`。
   - 已失效：保持独立失效标记，不能重新解释为不存在。

   记录失败时，只有更新前的数据库状态与快照一致，才能跟随提交结果。版本判断应表达为：

   ```go
   canFollow := known && expected == prior
   ```

   不匹配时，**仍然记录本次真实失败**，但把该作用域标为 `Stale`，不允许后续成功清理它。不能因为失去跟随资格而丢掉失败记录。

   这样，“初始无状态 → 外部创建版本 1 → 会话自身失败”的场景就会失效，而“初始无状态 → 会话自身首次创建”仍可正常跟随。

5. **补齐准入一致性，防止独立期限再次依赖 `OpenUntil`**

   [capture](/D:/Workspace/StarHub/internal/upstream/state.go:178) 当前只在兜底模式检查独立冷却。建议在保护启用时，普通和兜底准入都先检查：

   ```go
   if state.RateLimitedUntil > now {
       return nil, ErrBlocked
   }
   ```

   随后再分别处理普通保护和可跨越的保护。

   [claimSlot](/D:/Workspace/StarHub/internal/upstream/probe.go:22) 的查询也应覆盖“`OpenUntil == 0`，但独立冷却仍有效”的状态。否则即使后续循环检查了 `RateLimitedUntil`，这些行也根本没有被读取。

   这属于必要的防御性收口，可以避免异常存量数据造成不同准入路径判断不一致。

6. **交错响应单独补齐，测试应验证实际调用链**

   如果支持同一 Realtime 连接多个响应交错，应在 `response.created` 时按响应标识保存快照，终态结果使用该响应开始时的快照。仅在收到成功终态时复制会话最新状态，仍可能清理另一个响应较晚产生的失败。

   无法关联到业务开始快照的成功，可以记录事件，但不应凭最新状态执行恢复。

   验收重点建议如下：

   | 场景 | 必须满足 |
   |---|---|
   | 旧表新增字段、已有字段包含 `NULL` | 升级后有效冷却仍阻止发送 |
   | 默认 30 秒，上游要求 600 秒，60 秒后升级 | 剩余冷却保留 |
   | 人工重置或成功恢复后重启 | 旧冷却不复活 |
   | 迁移重复执行、中途失败重试 | 不重复施加保护，不提前写完成标记 |
   | Realtime 初始无状态，外部先失败 | 会话成功不清除外部失败 |
   | Realtime 自身交替失败、成功 | 未熔断计数正常归零 |
   | 较早响应成功晚于另一个响应失败 | 后者失败保留 |
   | 旧槽位期限已过，新间隔期限未过 | 仍拒绝发送 |

建议三个迁移问题作为同一个修复整体交付，Realtime 版本判断单独交付；交错响应随后处理。修复完成后，应将已复现用例转为正式回归测试，再运行项目要求的 build、vet 和全量测试。本轮仅追加文档并分析方案，没有实施这些代码修复。

---

## 十一、复核问题修复交付报告（原样记录）

已按文档第九、十节建议实现第 1–5 项，第 6 项（交错响应）按文档“随后处理”暂缓。改动未提交。

**1. 字段约束与 NULL 兼容**
- `State.RateLimitedUntil`、`FallbackSlot.NextAttempt/LastAttempt` 增加 `not null;default:0`。
- 新增 `normalizeUpstreamNulls(db)`，在 AutoMigrate 之前把存量 `NULL` 归零。
- 覆盖三条升级路径：旧库无列、`b9bb7a2` 已产生 NULL、全新库。

**2. 历史期限按事件 `OpenUntil` 重建**
- `rebuildRateLimitDeadlines` 不再用“事件时间 + 当前限流秒数”。
- 识别 attempt 的 `class=rate_limited` 与 probe 的 `kind=probe & decision=rate_limited`。
- 扫描条件改为 `open_until > now`，不再被默认窗口截断；仍在 `rate_limited` 的状态用自身 `OpenUntil` 兜底。

**3. 版本化一次性迁移**
- 新增 `upstream_migrations` 表与标识 `upstream_rate_limit_deadline_v2`。
- 在事务内取得上游写锁，数据修复与完成标记一起提交。
- 尊重 `manual_reset`、`probe success`、`fallback_recovered`；`fallback_recovery_skipped` 不算恢复。
- 仅填充缺失期限，不改写已有值；写入时推进版本并清理旧探测租约。

**4. Realtime 初始缺失状态**
- `capture` 为四个作用域显式写入版本 0（表示“最初不存在”）。
- 失败仅在 `known && expected == prior` 时跟随；否则标记 `Stale`，但仍记录真实失败。

**5. 准入一致性**
- 普通与兜底 `capture` 都先检查 `RateLimitedUntil > now`。
- `claimSlot` 查询纳入 `open_until = 0` 但限流仍有效的状态。

**验证**
- 新增回归测试：NULL 归零、长 `Retry-After` 恢复、重置不被复活、迁移只执行一次、Realtime 初始无状态拒绝外部版本。
- `go build ./...`、`go vet ./...` 通过；`internal/upstream`、`relay`、`config` 全量通过。
- `model`、`service`、`controller` 定向通过；仅剩既有基线失败（`price cap ignored`、`open circuit admitted customer traffic`、重试数断言、`usage logs: 7`），无新增偏差。

需要我提交到本地吗？

---

## 十二、复核问题修复提交再审分析（原文追加）

对照文档第十一节与最新提交 **`075c60c`（Repair rate-limit migration and explicit absent snapshots）**，结论是：**修复有效，但还没有完全闭环；本轮复现了 4 类剩余问题。**

| 交付报告内容 | 复核结果 |
|---|---|
| NULL 归零及字段约束 | 已修复；旧库无列、已有可空列两条升级路径均验证通过 |
| 历史长冷却重建 | 简单场景已修复，多次限流与恢复交替时仍有遗漏 |
| 一次性迁移 | 已实现，但无法纠正部分旧迁移留下的错误值 |
| Realtime 初始不存在状态 | 已修复；实际调用验证外部失败保留 |
| 普通与兜底准入一致性 | 已实现 |

1. **[高] 旧迁移写入的错误非零期限，升级后仍不会被纠正**

   位置：[迁移筛选条件](/D:/Workspace/StarHub/model/upstream_migration.go:66)。

   当前只处理：

   ```sql
   rate_limited_until IS NULL OR rate_limited_until = 0
   ```

   但 `b9bb7a2` 的错误迁移可能留下非零值，包括：

   - 按默认间隔推算出的过短期限。
   - 人工重置后，被旧迁移重新施加的期限。

   **两个场景均已复现：**

   - 实际冷却还剩 540 秒，旧错误值已过期 30 秒；新迁移跳过该行，兜底成功放行。
   - 状态已经 `manual_reset`、`OpenUntil=0`，但旧迁移留下的冷却仍剩 500 秒；升级后继续阻止普通请求。

   因此，“只填缺失期限、不改已有值”并不能完成对错误旧版本的修复。需要区分已有值是否可靠，并检查后续重置、恢复证据。

2. **[高] 先选最长期限，再检查恢复，会漏掉恢复之后的新限流**

   位置：[选择历史事件](/D:/Workspace/StarHub/model/upstream_migration.go:82)、[恢复判断](/D:/Workspace/StarHub/model/upstream_migration.go:99)。

   当前先取 `OpenUntil` 最大的一条事件。如果该事件之后发生过恢复，就直接跳过整个状态，没有继续查找新的有效限流。

   已复现的顺序：

   ```text
   旧限流：期限很长
       ↓
   人工重置
       ↓
   新限流：期限较短，但仍有效
       ↓
   其他错误覆盖 Reason
       ↓
   执行迁移
   ```

   迁移选中已被重置的旧长期限，随后因为存在重置而退出，连新的有效限流也一起忽略。

   **复现结果：**新冷却还剩 540 秒，迁移后 `RateLimitedUntil=0`，兜底成功放行。

   正确方向应是先确定有效的恢复边界，再从该边界之后的限流证据中重建期限。

3. **[高] 同秒发生“重置→新限流”，有效冷却会被误判为已清除**

   位置：[秒级时间比较](/D:/Workspace/StarHub/model/upstream_migration.go:99)。

   当前使用：

   ```sql
   created >= evidenceAt
   ```

   把恢复事件判断为发生在限流之后。但 `Created` 只有秒级精度，同一秒内先重置、后限流，两者时间值完全相同。

   **已复现：**当前状态明确为 `Reason=rate_limited`，保护仍剩 590 秒；由于同秒存在重置事件，迁移跳过回填，兜底成功放行。

   这与文档第十节提出的“仍打开且证据不足时保守处理”不一致。简单把 `>=` 改为 `>` 也不完整，因为相反顺序“限流→重置”同样可能发生在同一秒。

   需要结合当前状态处理历史歧义；后续事件应记录能够表达提交顺序的状态版本。

4. **[中] Realtime 上游层已保留外部失败，本地路由层仍会误清**

   位置：[本地路由版本跟随](/D:/Workspace/StarHub/model/local_routing.go:198)、[Realtime 调用位置](/D:/Workspace/StarHub/relay/realtime_protection.go:182)。

   上游 `RecordCommitted` 已正确检查失败前的版本，但本地 `RecordRoutingOutcome(..., true)` 仍会无条件跟随本次失败提交后的代数，没有先检查会话原代数是否匹配。

   已复现：

   ```text
   会话捕获代数 0
       ↓
   外部请求失败，代数变为 1
       ↓
   会话自身失败，代数变为 2，并直接跟随
       ↓
   会话成功，连续失败全部归零
   ```

   **结果：**本地路由 `ConsecutiveFailures=0`，外部失败被清除。

   这是继承的既有问题，并非本次提交新增；但它仍影响相关修复的完整性。现在上游层能够保留外部失败，本地路由层却清掉它，两层状态仍可能不一致。本地路由也需要采用“仅跟随自身连续版本，否则失效”的规则。

文档明确暂缓的**交错响应隔离**仍未实现，应继续作为未完成项。此前指出的限频间隔增大测试也未调整：旧 `NextAttempt` 本来就没有到期，不能充分证明新计算逻辑有效。

本轮验证结果：

| 检查 | 结果 |
|---|---|
| `go build ./...`、`go vet ./...` | 通过 |
| `internal/upstream`、`relay`、`config` 全量测试 | 通过 |
| 本次新增的 4 个 model 迁移测试 | 通过 |
| 旧表升级、Realtime 初始缺失版本隔离验证 | 通过 |
| 上述 4 类剩余问题 | 均通过 Go overlay 复现 |
| 全量 `go test ./...` | 本轮未运行 |

建议优先修正迁移的证据筛选和恢复顺序，并处理旧错误非零值。**后续修复需要使用新的迁移版本标识**，否则已写入 `upstream_rate_limit_deadline_v2` 完成标记的数据库不会重新执行修复。

本轮未修改仓库文件；隔离用例仅写入系统临时目录。
