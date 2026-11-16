# 本地开发指南

当前项目支持 Go 后端与 Vite 前端分别启动。前端开发服务提供热更新，保存 React、TypeScript 或 CSS 源文件后即可在浏览器中查看变化，无需重新构建 Go 程序。

## 环境要求

- Go 1.26+
- Node.js 24，具体版本参考 `web/.node-version`
- npm；也可使用本地已安装的 Bun 启动前端

以下命令适用于 Windows PowerShell，项目目录为 `D:\Workspace\StarHub`。

## 分别启动前后端

### 启动前：保持凭证加密配置一致

复用已有数据库时，必须使用创建凭证时相同的 `STARHUB_CRYPTO_SECRET`；如使用版本密钥环，还需恢复对应的 `STARHUB_CREDENTIAL_KEYS` 和 `STARHUB_CREDENTIAL_ACTIVE_KEY`。上游的 `sk-…` 接口密钥不是这个加密密钥。

Go 服务不会自动读取前端的 `web/.env.local` 来设置这些变量。关闭原终端、换终端或切换为发布程序启动时，应从相同的安全配置来源加载原值，再启动服务。不要生成新值来尝试解密旧数据库，也不要把真实密钥写入版本库、命令历史或日志。

如果已找回原来的 `STARHUB_CRYPTO_SECRET`，可在准备启动 Go 的 PowerShell 中通过隐藏输入加载，避免把原值写入命令历史：

```powershell
$credentialSecret = Read-Host "Original STARHUB_CRYPTO_SECRET" -AsSecureString
$env:STARHUB_CRYPTO_SECRET = [System.Net.NetworkCredential]::new('', $credentialSecret).Password
Remove-Variable credentialSecret
```

上面的赋值仅对当前 PowerShell 及随后启动的子进程生效，不会替你永久保存密钥。首次使用全新数据库前，应先在受保护的配置来源保存持久密钥，并确保之后每次启动都加载同一个值。加密配置及轮换说明见 [CREDENTIAL_STORAGE.md](CREDENTIAL_STORAGE.md)。

如果日志出现 `no persistent crypto secret`，当前程序使用的是未保存的进程随机密钥；该进程退出后，新进程无法解密它写入的渠道凭证或客户令牌。`credential migration could not decrypt a stored value` 表示程序正在阻止使用无法解密的已有数据；不能通过删除密文或跳过检查来安全恢复。原密钥无法找回时，需要先保留备份，再明确决定如何重新配置受影响的凭证。

### 终端一：启动 Go 后端

```powershell
cd D:\Workspace\StarHub
$env:STARHUB_PORT = "3008"
go run .
```

后端监听 3008 端口，供前端代理访问。默认使用当前工作目录下的 `starhub.db`；如果当前开发环境已有数据库、配置文件或凭证相关环境变量，应继续沿用，以保持现有数据和配置。

需要指定配置文件时，在同一终端中使用实际的配置路径，例如：

```powershell
go run . --config .\config.yml
```

`STARHUB_PORT` 环境变量优先于配置文件中的 `server.port`。上述示例中的 `config.yml` 需已存在。

### 终端二：启动 Vite 前端

在 `web/.env.local` 中设置后端代理地址，使其与后端实际监听端口一致。本机使用：

```dotenv
STARHUB_API_URL=http://127.0.0.1:3008
```

新环境可从 `web/.env.example` 复制为 `web/.env.local`，再调整代理地址。`.env.local` 已被 Git 忽略，不会提交本机配置。

```powershell
cd D:\Workspace\StarHub\web
npm run dev
```

首次使用或需要按锁文件重新安装依赖时，先运行：

```powershell
npm ci
```

如果使用已安装的 Bun，也可用以下命令启动：

```powershell
bun.cmd run dev
```

浏览器访问 **http://127.0.0.1:5173**。两个终端均需保持运行，分别按 `Ctrl+C` 停止对应服务。

## 请求代理与登录

当前 `web/vite.config.ts` 设置前端端口为 5173，并读取 `STARHUB_API_URL`，将 `/api` 和 `/v1` 请求代理到配置的后端地址。本机配置对应：

```text
浏览器 → Vite 前端 :5173
              └─ /api、/v1 → Go 后端 :3008
```

前端的 `VITE_API_BASE_URL` 保持默认空值，让浏览器通过前端地址发送相对路径请求。Vite 将 API 请求转发到 Go 后端，登录也使用这一代理链路。

Vite 使用启动模式加载 `web` 目录下的 `.env` 文件。相同键的优先级为：启动终端已有的环境变量 > `.env.[mode].local` > `.env.[mode]` > `.env.local` > `.env`；`STARHUB_API_URL` 未设置或为空时，代理目标回退到 `http://127.0.0.1:3000`。普通 `npm run dev` 的模式为 `development`。

也可在前端启动终端临时指定代理目标：

```powershell
$env:STARHUB_API_URL = "http://127.0.0.1:3008"
npm run dev
```

该变量用于 Vite 开发服务器代理，不会作为 `VITE_` 环境变量自动暴露给浏览器，也不会改变 Go 后端的监听端口或发布程序的配置。

本地分离开发应保留该代理方式。直接把 API 基地址设成另一个端口，会改变浏览器的请求来源关系；当前 HTTP 客户端使用 `credentials: "same-origin"`，后端管理接口也有跨来源写请求保护。

本地 HTTP 开发时不要启用 `STARHUB_COOKIE_SECURE=true`；同时检查所用配置文件和数据库中的 Cookie 设置是否要求 HTTPS。

## 修改如何生效

| 修改内容 | 生效方式 |
| --- | --- |
| React、TypeScript、CSS 源文件 | 保存后 Vite 自动热更新；部分变更会触发整页刷新 |
| Go 后端代码 | 停止后重新执行 `go run .`，使用配置文件时保留原参数 |
| Vite 代理配置或前端环境变量 | 修改后重启前端开发服务 |
| 已嵌入 Go 程序的前端页面 | 重新构建前端及包含前端资源的 Go 程序 |

开发时访问 5173 端口，才能看到 Vite 提供的最新页面。普通 `go run .` 是 API 模式，访问后端根路径 `/` 返回 404 属于正常行为。带有嵌入前端的发布程序提供构建时的页面，不会随着源码保存而热更新。

Go 后端目前使用普通 `go run .` 启动，不会自动监听源码变化并重启。

## 本地开发中的双重验证设置

后台账号首次登录仍需修改初始密码。在本地开发环境（`STARHUB_ENV` 未设置或为 `development`），尚未启用双重验证的后台账号可在绑定步骤点击“暂时跳过”进入后台，包括已生成绑定密钥但尚未验证动态码的情况。

跳过由后端确认，仅对当前登录会话有效；刷新页面可继续使用该会话，退出后重新登录会再次提示设置。跳过不会启用身份验证器，也不会成为双重验证通过的凭据。账号密码、角色、安全版本或双重验证启用状态发生变化后，跳过授权不再有效。

自助改密只刷新发起请求会话的密码证明，并清除该会话之前的跳过授权；其他旧会话不能重新申请跳过，需重新登录。已启用双重验证的会话不会因密码证明刷新而延续其双重验证权限。

初始改密成功后，若双重验证状态查询暂时失败，页面会保留“密码已更改”的成功提示，并提供“重新加载状态”按钮。重试只查询状态，不会再次提交改密；是否显示跳过按钮仍以后端返回值为准。

`STARHUB_ENV=production` 时不提供跳过功能；已经启用双重验证的账号仍需正常验证动态码。旧后端需要重启后才能向前端返回跳过能力。

## 端口配置注意事项

- `config.example.yml` 的 `server.port` 为 **3008**，与本机 `web/.env.local` 中的代理目标一致。未提供配置及环境变量时，Go 后端和 Vite 代理均默认使用 **3000**。
- 如果需要让后端使用其他端口，应修改后端 YAML 的 `server.port` 或 `STARHUB_PORT`，同时调整 `web/.env.local` 的 `STARHUB_API_URL`，然后重启对应服务，无需修改业务代码或 Vite 配置代码。
- Vite 不会自动读取 Go 后端的 YAML 或 `STARHUB_PORT`；两端配置需保持一致。若配置文件值与实际访问目标不符，检查启动终端是否设置了覆盖它的环境变量。
- 前端设置了 `strictPort: true`，5173 被占用时启动会报错，不会自动切换到其他端口。可指定一个空闲的前端端口，例如 `npm run dev -- --port 5174`，并访问终端输出的地址。

## 常用验证命令

后端在项目根目录执行：

```powershell
go build ./...
go vet ./...
go test ./...
```

前端在 `web` 目录执行：

```powershell
npm run typecheck
npm run test:unit
npm run build
```

日常查看前端修改不需要反复运行发布构建；保持 Vite 开发服务运行即可。

Windows 下发布构建会执行 `npm ci`，运行中的 Vite 可能占用 Rollup 原生模块，使依赖清理报 `EPERM`。先在前端终端按 `Ctrl+C`，再到项目根目录执行 `node scripts/build-release.mjs`；也可使用 `node scripts/build-release.mjs --stop-dev-server`，由脚本停止检测到的本项目 Vite/esbuild 进程。构建完成后重新运行 `npm run dev`。脚本默认只报告占用，不会终止其他项目的进程或先删除部分依赖。
