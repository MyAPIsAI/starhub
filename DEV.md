# 本地开发指南

当前项目支持 Go 后端与 Vite 前端分别启动。前端开发服务提供热更新，保存 React、TypeScript 或 CSS 源文件后即可在浏览器中查看变化，无需重新构建 Go 程序。

## 环境要求

- Go 1.26+
- Node.js 24，具体版本参考 `web/.node-version`
- npm；也可使用本地已安装的 Bun 启动前端

以下命令适用于 Windows PowerShell，项目目录为 `D:\Workspace\StarHub`。

## 分别启动前后端

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
