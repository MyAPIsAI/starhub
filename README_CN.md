# StarHub

StarHub 是一个使用 Go 编写的 LLM API 网关服务,可将多个上游渠道聚合为统一的 OpenAI / Claude 风格 API,并提供用户、令牌、渠道、配额与计费比率的管理能力。

> 当前版本:`v0.1.0`(纯 Go 实现,无 CGO 依赖,单二进制部署)

## 功能特性

- **多协议中继(P3)**
  - **任意入站协议 × 任意出站渠道**:`/v1/chat/completions`(OpenAI 格式)与 `/v1/messages`(Claude 格式)请求可路由到任意类型的渠道,`relay/dto` 层的统一中间表示(IR)负责双向协议转换;渠道与客户端协议相同时走字节级透传
  - **多渠道类型**:OpenAI 兼容(1)、Claude(14)、Gemini(25)、AWS Bedrock(15,请求由 SigV4 签名,流式响应解析 AWS 二进制事件流)——覆盖「鉴权头不同 / 路径风格不同 / 签名 V4」三类典型差异,新渠道只需实现 `relay/channel.ChatAdaptor` 并在 `relay/relay_adaptor.go` 注册
  - **全端点家族**:`/v1/embeddings`、`/v1/rerank`、`/v1/audio/speech`(TTS)、`/v1/audio/transcriptions|translations`(STT,multipart 可重放)、`/v1/images/generations|edits`(按张计价、流式图片 SSE)、`/v1/video/generations`(Sora 风格任务 API)、`/v1/realtime`(WebSocket,零第三方依赖的 RFC 6455 实现)
  - **任务型 API(提交-轮询)**:`TaskAdaptor` 接口内建计费钩子(`ValidateRequestAndSetAction → EstimateBilling(预扣) → 提交 → 轮询 ParseTaskResult → AdjustBillingOnComplete(终态差额结算)`),后台 reaper 兜底完成无人轮询的任务,失败任务全额退款
- **多渠道路由**:按「分组 + 模型」选择可用渠道,跨协议故障切换(首个渠道 5xx 时自动切换到其它协议的渠道),上游返回 `401/403` 时自动禁用渠道
- **配额与计费**
  - 预扣费 + 失败退款 + 事后按实际 usage 结算,三段式对文本 / 图片 / 任务 / Realtime 全部生效
  - 分组倍率(group ratio)、模型倍率(model ratio)、补全倍率(completion ratio)可动态配置
  - 换算关系:`1 USD = 500000` 配额
- **管理 API**:用户 / 渠道 / 令牌 / 选项 / 日志的完整增删改查
- **权限体系**:普通用户(1)、管理员(10)、超级管理员(100)三级角色,会话鉴权 + Bearer Token 鉴权双通道
- **零依赖部署**:内置纯 Go SQLite(基于 glebarez/sqlite),无需外部数据库

## 快速开始

### 环境要求

- Go 1.26+

### 构建与运行

```bash
# 克隆项目
git clone <repo-url> starhub
cd starhub

# 直接运行
go run .

# 或构建单二进制
go build -o starhub .
./starhub
```

启动后默认监听 `:3000`,数据库文件为工作目录下的 `starhub.db`(首次启动自动建表并写入种子数据)。

### 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `STARHUB_PORT` | `3000` | HTTP 监听端口 |
| `STARHUB_DB` | `starhub.db` | SQLite 数据库文件路径 |
| `STARHUB_ROOT_PASSWORD` | `123456` | root 用户初始密码(**生产环境务必修改**) |
| `STARHUB_CHANNEL_TEST_INTERVAL` | `1800` | 渠道后台测活间隔(秒,`0` 关闭) |
| `STARHUB_TASK_POLL_INTERVAL` | `60` | 任务后台轮询间隔(秒,`0` 关闭) |

### 首次登录

默认内置超级管理员账号:

- 用户名:`root`
- 密码:`123456`(或 `STARHUB_ROOT_PASSWORD` 设置的值)

## 使用指南

### 1. 登录管理后台 API

```bash
curl -X POST http://localhost:3000/api/user/login \
  -H "Content-Type: application/json" \
  -d '{"username":"root","password":"123456"}'
```

登录成功后:

- 响应体中返回 `session` 字段(有效期 7 天)
- 同时设置名为 `session` 的 HttpOnly Cookie

后续管理接口可通过 Cookie 携带会话,也可使用 `Authorization: Bearer <session>` 头(见 `middleware/session.go`)。

### 2. 创建上游渠道(管理员)

```bash
curl -X POST http://localhost:3000/api/channel \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <session>" \
  -d '{
    "name": "openai-main",
    "type": 1,
    "key": "sk-xxxx",
    "base_url": "https://api.openai.com",
    "models": "gpt-4o,gpt-4o-mini",
    "group": "default"
  }'
```

- `models` 为逗号分隔的模型列表
- `base_url` 不含 `/v1` 路径(由适配器拼接),留空使用各协议官方默认地址

支持的渠道类型:

| type | 协议 | 鉴权 / 路径风格 | key 形态 |
| --- | --- | --- | --- |
| `1` | OpenAI 兼容 | `Authorization: Bearer`,`/v1/chat/completions` 等标准路径 | API key(多 key 用换行分隔) |
| `14` | Anthropic Claude | `x-api-key` + `anthropic-version`,`/v1/messages` | API key |
| `15` | AWS Bedrock | SigV4 签名,`/model/{modelId}/invoke[-with-response-stream]` | `accessKeyId\|secretAccessKey\|region` |
| `25` | Google Gemini | `x-goog-api-key`,`/v1beta/models/{model}:generateContent` | API key |

任何入站协议可以路由到任何渠道类型:例如客户端用 Claude 格式(`/v1/messages`)请求 `gpt-4o-mini`,网关会做 DTO 互转后发给 OpenAI 渠道,再把响应转回 Claude 格式(含流式事件与错误封套的互转)。协议相同时(如 Claude 入站 → Claude 渠道)走字节级透传。

### 3. 创建 API 令牌

```bash
curl -X POST http://localhost:3000/api/token \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <session>" \
  -d '{"name": "my-token"}'
```

响应中的 `key` 字段即为可用密钥(`sk-` 前缀,48 位随机字符)。默认配额 `500000`($1),可选字段:`remain_quota`、`unlimited_quota`、`expired_time`(Unix 时间戳)、`group`。

### 4. 调用中继接口

```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "stream": false,
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

流式调用示例:

```bash
curl -N http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hello"}]}'
```

Claude 格式入站示例(`/v1/messages` 可路由到任意类型渠道,响应保持 Claude 格式):

```bash
curl http://localhost:3000/v1/messages \
  -H "x-api-key: sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "hello"}]
  }'
```

视频任务示例(提交-轮询,终态自动结算):

```bash
curl -X POST http://localhost:3000/v1/video/generations \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"sora-2","prompt":"a cat surfing","seconds":8,"n":2}'

curl http://localhost:3000/v1/video/generations/<id> \
  -H "Authorization: Bearer sk-xxxx"
```

### 5. 查询可用模型

```bash
curl http://localhost:3000/v1/models \
  -H "Authorization: Bearer sk-xxxx"
```

## 管理 API 一览

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/user/login` | 公开 | 登录 |
| `POST` | `/api/user/logout` | 登录用户 | 登出 |
| `GET` | `/api/user/self` | 登录用户 | 当前用户信息 |
| `GET` / `PUT` | `/api/user` | 管理员 | 用户列表 / 更新用户 |
| `POST` / `GET` / `PUT` / `DELETE` | `/api/channel`(`/{id}`) | 管理员 | 渠道管理(支持 1/14/15/25 四类) |
| `POST` | `/api/channel/test/{id}` | 管理员 | 渠道测活(探针按渠道协议自动转换) |
| `POST` / `GET` / `PUT` / `DELETE` | `/api/token`(`/{id}`) | 登录用户 | 令牌管理 |
| `GET` / `PUT` | `/api/option` | 管理员 | 系统选项(含倍率配置) |
| `GET` | `/api/log` | 登录用户 | 消费日志 |

## 中继 API 一览

| 方法 | 路径 | 入站格式 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/v1/chat/completions` | OpenAI | 聊天补全(流式 / 非流式) |
| `POST` | `/v1/messages` | Claude | 聊天补全(流式 / 非流式),Claude SDK 可直接对接 |
| `POST` | `/v1/embeddings` | OpenAI | 向量嵌入 |
| `POST` | `/v1/rerank` | OpenAI | 重排(Cohere 兼容格式) |
| `POST` | `/v1/audio/speech` | OpenAI | 语音合成(音频透传) |
| `POST` | `/v1/audio/transcriptions` | multipart | 语音转文字(表单 + 文件,重试可重放) |
| `POST` | `/v1/audio/translations` | multipart | 语音翻译 |
| `POST` | `/v1/images/generations` | OpenAI | 文生图(按张计价,`stream` 支持 SSE) |
| `POST` | `/v1/images/edits` | multipart | 图生图(multipart 重放,`n` 与 JSON 路径同界) |
| `POST` | `/v1/video/generations` | OpenAI | 视频任务提交(Sora 风格) |
| `GET` | `/v1/video/generations/{id}` | - | 视频任务轮询(终态触发结算) |
| `GET` | `/v1/realtime?model=...` | WebSocket | Realtime 会话双向透传,`response.done` 的 usage 结算 |
| `GET` | `/v1/models` | - | 模型列表 |

中继鉴权支持 `Authorization: Bearer sk-...` 与 `x-api-key: sk-...` 两种头(Claude SDK 默认使用后者)。所有中继端点共享 `TokenAuth → UserRateLimit → Distribute → Relay` 链路与三段式计费。

## 计费说明

计费采用三段式资金安全模型(「预扣 → 结算 → 退款」):

```
请求前  PreConsumeQuota    按 tokenizer 估算的 prompt + max_tokens×n 的补全预算预扣
响应后  PostConsumeQuota   按上游真实 usage(含 cache / audio 细分)结算,多退少补
失败时  RefundPreConsumedQuota  全额退还预扣
```

- **预扣防并发透支**:预扣在单一写事务内完成「校验 + 扣减」(SQLite 串行化写入,等效 `lockForUpdate` 行锁),用户余额 1 元同时发 10 个请求不可能全部放行
- **结算原子落账**:差额调整、token / user / channel 的 used_quota 累计与消费日志在同一事务内提交,账目始终对得上
- **计价优先级**:表达式计价(`model_price_expr`) > 按次计价(`model_price`) > token 计价(倍率公式)
- 所有 float→int 换算走 `common/quota_math.go` 饱和截断(防溢出回绕成负数=送钱),被截断/钳制的请求在日志中留下 `quota_saturation` 审计标记

token 计价公式位于 `relay/pricing.go`:

```
有效输入  = 文本 + cached × cache_ratio + 输入音频 × audio_ratio
有效输出  = 文本 × completion_ratio + 输出音频 × audio_ratio
消耗配额  = (有效输入 + 有效输出) × model_ratio / 1,000,000 × 500,000 × group_ratio × other_ratio
```

即每 100 万 tokens 在 `ratio = 1` 时计费 $1(500000 配额)。`other_ratio` 由特殊乘数累乘(`relay.PriceData.AddOtherRatio`):图片的「尺寸 × 品质 × 张数」、视频任务的「秒数 × 张数」都挂在同一条链上,复用同一套钳制与审计。

可在 `PUT /api/option` 中热更新的选项:

| 选项键 | 形态 | 说明 |
| --- | --- | --- |
| `group_ratio` | `{"default":1}` | 分组倍率 |
| `model_ratio` | `{"gpt-4o":2.5}` | 模型 token 倍率(未命中回退 `1.0`) |
| `completion_ratio` | `{"gpt-4o":4}` | 补全倍率(未命中回退 `4.0`) |
| `model_price` | `{"model":0.01}` | 按次计价(USD/次,命中后忽略 token 数) |
| `model_price_expr` | `{"model":"<expr>"}` | 表达式计价(USD/次),支持阶梯价/动态价,语法与安全约束见 `pkg/billingexpr/expr.md` |
| `cache_ratio` | `0.5` | cached prompt token 折扣(OpenAI 口径 0.5) |
| `audio_ratio` | `40` | audio token 乘数 |

### 请求侧校验与计量

- 聊天:`n`(`1..10`)与 `max_tokens` / `max_completion_tokens` / Claude `max_tokens`(`0..100000`)在**请求验证阶段**拒绝越界(400),乘数上界决定了预扣额度不可能被恶意放大
- 图片:`n`(`1..128`)在 JSON 路径与 multipart 路径共享同一计费闸门(含 uint 回绕负值);全角 `×` 尺寸直接拒绝并提示用 `x`
- 任务:`seconds`(`1..60`)与 `n`(`1..10`)同上
- 流式请求自动注入 `stream_options.include_usage`;上游不返回 usage 时按本地累计的 SSE 内容估算,不漏计
- 上游 usage 的 `prompt_tokens_details.cached_tokens`、`completion_tokens_details.audio_tokens` 等细分参与结算,且细分值被钳制在总量之内(谎报的 usage 无法放大折扣)
- 请求失败、上游 4xx/5xx 错误时退还预扣配额

### 各端点的计费口径

| 端点 | 预扣 | 结算 |
| --- | --- | --- |
| chat / messages | prompt 估算 + max_tokens×n 预算 | 上游 usage(含 cache / audio 细分),缺省回退本地估算 |
| embeddings / rerank | 输入 token 估算 | 上游 usage 的输入 token |
| TTS | 按次价或输入 token 估算 | 音频响应无 usage,按输入计 |
| STT | 按次价或音频时长估算(文件字节数启发式) | verbose_json 的 duration(≈25 token/秒),否则维持估算 |
| images | 按次价 × 尺寸倍率 × 品质倍率 × n | 上游实际返回张数([1,128] 之外的恶意值回退请求值;流式按 completed 事件计数) |
| video 任务 | 按秒价 × seconds × n | 终态差额结算:成功按请求参数,取消半价,失败全额退款 |
| realtime | 按次价或保守 token 预算 | `response.done` 的 usage(含 audio 细分),缺省按会话文本估算 |

## 项目结构

```
starhub/
├── main.go                        # 入口:加载配置、初始化数据库、注册路由、启动任务 reaper
├── e2e_test.go                    # P0-P2 端到端集成测试(含 mock 上游)
├── e2e_p3_test.go                 # P3 端到端测试:协议矩阵 / 全端点 / 任务 / Realtime
├── common/                        # 常量、上下文键、加解密、统一响应、配额饱和换算、multipart 解析
├── controller/                    # HTTP 处理器;relay.go 系列承载共享重试循环与各模式流程
├── middleware/                    # 鉴权(UserAuth/AdminAuth/TokenAuth)、会话、渠道路由(Distribute,支持 multipart)
├── model/                         # GORM 数据模型(user/token/channel/ability/log/option/task)与事务化配额操作
├── relay/
│   ├── dto/                       # 统一中间表示(IR)+ 各协议 wire DTO 与互转(openai/claude/gemini/bedrock/image/audio/task)
│   ├── channel/                   # ChatAdaptor / TaskAdaptor 接口与四个供应商包(openai/claude/gemini/aws)
│   ├── relay_adaptor.go           # 适配器注册 switch(新渠道在此接线)
│   ├── chat_writer.go             # IR → 客户端协议的响应/流式序列化(P3.2 矩阵的出站侧)
│   ├── pricing.go / quota.go      # 计价引擎与三段式配额(所有模式共用)
│   ├── image_pricing.go           # 按张计价(尺寸×品质×张数,含护栏)
│   ├── task_billing.go            # TaskPricing → 配额换算
│   ├── ws.go / realtime.go        # RFC 6455 帧编解码与 Realtime 会话中继
│   └── validate.go / estimate.go  # 请求侧校验与 token 估算
├── pkg/billingexpr/               # 计费表达式引擎(阶梯价/动态价,设计约束见 expr.md)
├── service/                       # 渠道测活、任务轮询(reaper)、禁用策略
├── setting/                       # 倍率与价格运行时配置(内存 + 数据库持久化)
└── doc/                           # 需求与架构参考文档
```

### 中继链路

```
Client ──TokenAuth(校验令牌/配额/分组)──▶ UserRateLimit ──▶ Distribute(解析并缓存
      请求体/模型/stream;multipart 缓存原始字节与 Content-Type)
      ──▶ Relay(选渠道 → 预扣费 → 重试循环:5xx/超时/网络错误按优先级阶梯切换渠道,
              不支持该模式的渠道跳过;4xx 错误封套按入站协议转换后透传终止;
              成功结算,全失败退款)──▶ 后台测活(自动禁用/恢复渠道)+ 任务 reaper(终态结算)
```

各中间件通过 request context 传递状态(`CtxUsingGroup` → `CtxRequestBody`/`CtxRequestModel`/`CtxRequestContentType`),渠道选择与重试由 `controller/relay.go` 的 `relayFlow` 完成(轨迹 `used_channels` 记录 `1->2->3`),见 `main.go` 中 `NewRouter` 的注释约定。

## 测试

```bash
go test ./...        # 全部测试(单元 + e2e)
go test -v ./...     # 详细输出
```

测试覆盖:加密与密钥校验(`common`)、配额结算与图片计价不变量(`relay`)、协议互转(`relay/dto`)、SigV4 官方文档向量与事件流解析(`relay/channel/aws`)、WebSocket 帧编解码(`relay/ws`)、倍率解析(`setting`)、数据层(`model`)、渠道健康规则(`service`)以及完整中继链路(`e2e_test.go` / `e2e_p3_test.go`,内置各协议 mock 上游:OpenAI / Claude / Gemini / Bedrock / 图片 / 任务 / Realtime)。

## License

本项目暂未声明开源协议,默认保留所有权利。
