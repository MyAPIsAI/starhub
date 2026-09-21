# StarHub

StarHub 是一个使用 Go 编写的 LLM API 网关服务,可将多个上游 OpenAI 兼容渠道聚合为统一的 OpenAI 风格 API,并提供用户、令牌、渠道、配额与计费比率的管理能力。

> 当前版本:`v0.1.0`(纯 Go 实现,无 CGO 依赖,单二进制部署)

## 功能特性

- **OpenAI 兼容中继**
  - `POST /v1/chat/completions`:聊天补全,支持流式(SSE)与非流式响应
  - `GET /v1/models`:按分组列出可用模型
- **多渠道路由**:按「分组 + 模型」选择可用渠道,自动注入 `stream_options.include_usage`,上游返回 `401/403` 时自动禁用渠道
- **配额与计费**
  - 预扣费 + 失败退款 + 事后按实际 usage 结算
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

- `type: 1` 表示 OpenAI 兼容渠道(当前仅支持该类型)
- `models` 为逗号分隔的模型列表
- `base_url` 不含 `/v1` 路径(由适配器拼接)

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
| `POST` / `GET` / `PUT` / `DELETE` | `/api/channel`(`/{id}`) | 管理员 | 渠道管理 |
| `POST` / `GET` / `PUT` / `DELETE` | `/api/token`(`/{id}`) | 登录用户 | 令牌管理 |
| `GET` / `PUT` | `/api/option` | 管理员 | 系统选项(含倍率配置) |
| `GET` | `/api/log` | 登录用户 | 消费日志 |
| `POST` | `/v1/chat/completions` | Bearer Token | 聊天补全中继 |
| `GET` | `/v1/models` | Bearer Token | 模型列表 |

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

即每 100 万 tokens 在 `ratio = 1` 时计费 $1(500000 配额)。`other_ratio` 由特殊乘数(音频/图片分辨率/视频秒数,`relay.PriceData.AddOtherRatio`)累乘,供后续图片/视频中继接入。

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

- `n`(`1..10`)与 `max_tokens` / `max_completion_tokens`(`0..100000`)在**请求验证阶段**拒绝越界(400),乘数上界决定了预扣额度不可能被恶意放大
- 流式请求自动注入 `stream_options.include_usage`;上游不返回 usage 时按本地累计的 SSE 内容估算,不漏计
- 上游 usage 的 `prompt_tokens_details.cached_tokens`、`completion_tokens_details.audio_tokens` 等细分参与结算,且细分值被钳制在总量之内(谎报的 usage 无法放大折扣)
- 请求失败、上游 4xx/5xx 错误时退还预扣配额

## 项目结构

```
starhub/
├── main.go              # 入口:加载配置、初始化数据库、注册路由
├── e2e_test.go          # 端到端集成测试(含 mock 上游)
├── common/              # 常量、上下文键、加解密、统一响应、配额饱和换算(quota_math.go)
├── controller/          # HTTP 处理器(user/channel/token/option/log/relay)
├── middleware/          # 鉴权(UserAuth/AdminAuth/TokenAuth)、会话、渠道路由(Distribute)
├── model/               # GORM 数据模型与数据访问层(SQLite,含事务化配额操作 quota.go)
├── relay/               # 上游适配器(openai)、计价引擎(pricing.go)、配额三段式(quota.go)
├── pkg/billingexpr/     # 计费表达式引擎(阶梯价/动态价,设计约束见 expr.md)
├── setting/             # 倍率与价格运行时配置(内存 + 数据库持久化)
└── doc/                 # 架构分析等参考文档
```

### 中继链路

```
Client ──TokenAuth(校验令牌/配额/分组)──▶ Distribute(解析并缓存 请求体/模型/stream)
      ──▶ Relay(选渠道 → 预扣费 → 重试循环:5xx/超时/网络错误按优先级阶梯切换渠道,
              4xx 直接透传终止;成功结算,全失败退款)──▶ 后台测活(自动禁用/恢复渠道)
```

各中间件通过 request context 传递状态(`CtxUsingGroup` → `CtxRequestBody`/`CtxRequestModel`),渠道选择与重试由 `controller/relay.go` 的重试循环完成(轨迹 `used_channels` 记录 `1->2->3`),见 `main.go` 中 `NewRouter` 的注释约定。

## 测试

```bash
go test ./...        # 全部测试(单元 + e2e)
go test -v ./...     # 详细输出
```

测试覆盖:加密与密钥校验(`common`)、配额结算(`relay`)、倍率解析(`setting`)、数据层(`model`)、渠道健康规则(`service`)以及完整中继链路(`e2e_test.go`,内置 mock 上游服务器)。

## License

本项目暂未声明开源协议,默认保留所有权利。
