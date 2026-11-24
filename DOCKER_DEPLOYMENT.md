# Docker Compose 部署

这套配置参考同级 Talen 的构建部署流程，使用 StarHub 已有的多阶段 Dockerfile 构建前端和 Go 后端。默认部署单实例、纯 Go SQLite，数据保存在 Compose 命名卷 `starhub_data` 中。宿主机默认访问地址为 `http://127.0.0.1:3008`，容器内部监听 `3000`。

## 首次启动

需要 Docker Engine 的 Linux 容器模式和支持 `up --wait` 的 Docker Compose 插件。Windows 可使用 Docker Desktop 和 Windows PowerShell 5.1 或 PowerShell 7。构建在容器中完成，无需在宿主机安装 Node、npm 或 Go。

在仓库根目录执行：

```powershell
.\init-deploy.ps1
docker build -t starhub-resource:latest ../resource
.\build-deploy.ps1 -WithResource
```

`-WithResource` 合并 `docker-compose.yml` 与 `docker-compose.resource.yml`，同时部署 StarHub 和新 resource，共用项目默认网络。resource 等待 StarHub 健康后启动。上述镜像构建命令要求同级 `../resource` 为协议匹配的源码；已有 `starhub-resource:latest` 镜像时可省略构建。部署脚本只构建 StarHub，resource 镜像须提前准备。两个服务使用同一份私有环境文件；仅部署网关时可省略 `-WithResource`。

初始化脚本从 `.env.docker.example` 创建私有 `.env.docker`，生成随机 root 初始密码、稳定加密密钥和资源平台服务凭据，不在终端输出这些值，也不覆盖已有文件。从该文件读取 root 初始密码，用户名为 `root`。如果已手动复制模板，需要自行填写必填项；初始化脚本不会改写它。

部署脚本先验证 Compose 配置，再构建镜像；构建成功后通过 Compose 更新服务，等待 `/api/health` 检查通过。已有数据卷会保留，脚本不会执行 `down -v`。初始化 root 密码只对新数据库生效，不能用修改环境变量的方式重置已有账号密码。容器重启会清除当前进程内的登录会话，需要重新登录。

`.env.docker` 与本地直接启动 Go 程序使用的 `.env` 分开。所有 Compose 命令都应指定 `--env-file .env.docker`；Shell 中同名环境变量优先于文件。部署脚本中的相对文件路径以仓库根目录解析，可以从其他目录调用。

## 常用操作

以下为单独操作网关的示例。配套部署时，脚本命令加 `-WithResource`；直接使用 Compose 时加 `-f docker-compose.yml -f docker-compose.resource.yml`，尤其是 `down`，以便同时管理这两个服务及其网络。

```powershell
# Build an image without starting or replacing any service.
.\build-deploy.ps1 -BuildOnly

# Deploy an image that was already built locally.
.\build-deploy.ps1 -SkipBuild

# Build for an ARM64 Docker host, or rebuild without the layer cache.
.\build-deploy.ps1 -GOARCH arm64 -BuildOnly
.\build-deploy.ps1 -NoCache

# Use an independent project and private environment file.
.\init-deploy.ps1 -EnvFile .env.docker.staging
.\build-deploy.ps1 -EnvFile .env.docker.staging -ProjectName starhub-staging

docker compose --env-file .env.docker ps
docker compose --env-file .env.docker logs --tail 100 -f starhub
docker compose --env-file .env.docker stop starhub
docker compose --env-file .env.docker start starhub
docker compose --env-file .env.docker down
```

`-BuildOnly` 与 `-SkipBuild` 互斥，`-WaitTimeout` 默认 180 秒。首次构建需要下载基础镜像、npm 依赖和 Go 模块。`GOPROXY` 可以在 `.env.docker` 中覆盖。`-GOARCH` 仅在本次脚本调用中覆盖 `STARHUB_PLATFORM`，结束后恢复原环境；长期使用 ARM64 时在文件中设置 `STARHUB_PLATFORM=linux/arm64`。

自定义 `-ProjectName` 时，后续 Compose 命令也必须带相同的 `-p`，否则会操作默认项目。不同项目应配置不同的 `STARHUB_HOST_PORT`，需要独立保留镜像时也应配置不同的 `STARHUB_IMAGE`。命名卷随项目名隔离；改变项目名不会自动迁移原数据库。正常更新保留相同项目名和原加密密钥。

默认仅发布到宿主机回环地址。需要从其他机器访问时配置 `STARHUB_HOST_IP`；经 HTTPS 反向代理访问时设置 `STARHUB_COOKIE_SECURE=true`，按实际代理网络配置 `STARHUB_TRUSTED_PROXIES`，并保留请求原始 Host。直接使用本机 HTTP 时保持 `STARHUB_COOKIE_SECURE=false`。

Compose 的健康检查验证数据库连接，不等待商业路由快照。流量入口的就绪检查应使用 `/api/ready`，具体行为见 [PRODUCTION.md](PRODUCTION.md)。日志写入容器标准输出并按大小轮转，停止服务预留 55 秒以配合现有优雅退出流程。

## 对接同级 resource

已有远程 `/root/starhub` 部署时，按 [远程配对部署步骤](deploy/resource/REMOTE_DEPLOYMENT.md) 添加增量配置、部署新 resource 并重建现有 StarHub，不要用本地完整主配置覆盖远程配置。

`docker-compose.resource.yml` 同时包含 StarHub 配对环境变量与完整的新 resource 服务，和 `docker-compose.yml` 合并使用，统一属于 `starhub` 项目。原独立 resource 服务文件已移除。该覆盖文件也能给远程旧版主配置添加配对变量；StarHub 原有镜像、端口、数据卷及加密设置仍由主配置决定。远程旧实例的区别及配置演变见 [Diff_Resource.md](Diff_Resource.md)。

在 StarHub 的 `.env.docker` 中设置：

```dotenv
RESOURCE_IMAGE=starhub-resource:latest
RESOURCE_HOST_IP=0.0.0.0
RESOURCE_HOST_PORT=3009
NEW_API_BASE_URL=http://starhub:3000
RESOURCE_CATALOG_NOTIFY_URL=http://starhub-resource:3001
NODE_NAME=starhub-node-1
```

两个服务使用同一项目的默认网络 `starhub_default`，由 Compose 管理，无需预先创建外部网络或设置 `RESOURCE_NETWORK_NAME`。旧环境文件中的该变量不再使用。新 resource 不接入旧 resource 的网络。

resource 使用 `http://starhub:3000` 回连网关；网关使用 `http://starhub-resource:3001` 访问 resource。resource 容器内部监听 `3001`，宿主机映射保留为 `0.0.0.0:3009`；StarHub 宿主机端口仍为 `3008`。默认容器名分别为 `starhub-resource-1` 和 `starhub-starhub-1`。`NODE_NAME` 在同一部署中应保持稳定。

部署测试环境时，两个文件合并使用 `-p starhub-test`，两项服务自动进入 `starhub-test_default`，数据卷也按项目隔离。测试环境还需使用独立的宿主机端口、节点名和服务凭据。

新 resource 使用 SQLite `/data/resource.db`，默认保存在 `starhub_resource_data` 卷中，与 StarHub 的 `starhub_starhub_data` 及旧 resource 数据独立。`deploy/resource/cost_resources.json` 是独立成本覆盖列表，初始为空，渠道和模型由 StarHub 同步。远程该文件应位于 `/root/starhub/deploy/resource/cost_resources.json`，绑定路径相对于第一个 Compose 文件解析。

成本文件必须随配置上传；只复制 Compose 文件会出现 `bind source path does not exist`。未配置成本覆盖时文件内容为 `[]`，创建及校验命令见 [远程配对部署步骤](deploy/resource/REMOTE_DEPLOYMENT.md)。也可用 `RESOURCE_COST_FILE` 指向已有的 StarHub 专用 JSON 文件，容器内 `COST_DATA_PATH` 保持不变。同级 resource 的镜像构建不包含该文件，不能通过删除挂载解决；也不应直接复用旧 resource 的成本数据。

如果已经按此前独立项目方案运行了新 resource，原卷名是 `starhub-resource_resource_data`。合并不会自动迁移该卷；须先停用原新 resource 并备份、迁移其数据或显式复用原卷，再启动合并配置，避免端口、网络别名冲突和误用空数据库。旧 `/root/resource` 实例不属于此次合并范围。

`METRIC_AUTHORIZATION` 是这对服务共用的独立凭据，环境文件中只保存非空裸令牌。StarHub 容器收到裸令牌；resource Compose 服务自动添加 `Bearer ` 前缀，因为其采集逻辑将该配置原样用作 Authorization 请求头。两个容器的变量字符串因此不同，但实际令牌相同。不要在共享环境文件中添加前缀。新部署由 `init-deploy.ps1` 生成；已有环境文件须补齐当前有效凭据并保留原加密密钥。不要复制旧 resource 的 token。已有数据库曾轮换或撤销凭据时，不能假定修改初始环境变量会覆盖数据库状态。

先准备与当前 StarHub 协议匹配的 resource 镜像。Compose 保留 `resource:latest` 默认值，环境模板通过 `RESOURCE_IMAGE=starhub-resource:latest` 指定专用标签；实际部署按环境文件取值。若从同级 resource 源码构建，可执行 `docker build -t starhub-resource:latest ../resource`；不要覆盖旧 `resource-dev:latest`。旧源码及镜像未经本轮兼容性验证。

```powershell
# Deploy both services using the prepared images.
.\build-deploy.ps1 -WithResource -SkipBuild

docker compose --env-file .env.docker -p starhub -f docker-compose.yml -f docker-compose.resource.yml ps
docker compose --env-file .env.docker -p starhub -f docker-compose.yml -f docker-compose.resource.yml logs --tail 100 starhub resource
```

后续配套更新继续使用 `-WithResource`，或显式合并两个 Compose 文件。此配置不会修改旧 resource，也不会自动启用 `resource_provider_mode=commercial`。商业路由模式通过现有管理配置启用，首次收到有效快照前业务请求会被就绪门槛阻挡，详见 [RESOURCE_PLATFORM.md](RESOURCE_PLATFORM.md)。

主配置只提供共享 Docker 网络下的本地联调连接，不配置 mTLS。跨主机或生产环境的双向认证代理、访问边界和证书应按 [RESOURCE_PLATFORM.md](RESOURCE_PLATFORM.md) 配置；共享网络本身不代表这些条件已经满足。

## 数据与密钥

`/data` 卷保存 SQLite 及其 WAL。`.env.docker` 中的 `STARHUB_CRYPTO_SECRET` 和可选的版本化 wrapping keys 必须保留，使用已有数据库时应恢复原密钥，不要重新生成。密钥与数据库的备份恢复步骤见 [CREDENTIAL_STORAGE.md](CREDENTIAL_STORAGE.md) 和 [PRODUCTION.md](PRODUCTION.md)。运行中的 SQLite 应使用已有快照功能备份，不要仅复制主数据库文件。

`.env`、`.env.*`、本地数据及构建缓存已排除在 Docker 构建上下文之外。环境模板可以提交，生成的 `.env.docker*` 私有文件由 Git 忽略。普通停止或更新保留卷，`docker compose down -v` 会删除数据卷，因此不属于日常更新流程。

## 可选 config.yml 配置

`config.yml` 不是必需项。当前 `docker-compose.yml` 完全通过 `environment` 启动：容器内端口 `3000`、SQLite 路径 `/data/starhub.db`、root 初始密码和加密密钥均由环境变量提供，不挂载配置文件也能正常运行。

只有在以下情况才需要：

- 预置 `initial_options`，尤其是环境变量不支持的 `upstream_protection`、`channel_traffic_allocation`。
- 用 YAML 统一管理 `server`、`database`（driver、连接池）、`security.force_secure_cookie`。
- 不想在 Compose 中堆叠大量环境变量。

注意：**环境变量优先于 YAML**。Compose 中的 `STARHUB_PORT=3000`、`STARHUB_DB=/data/starhub.db` 会覆盖 YAML 中的 `server.port`、`database.sqlite_path`。

### 准备配置文件

在 Compose 目录下新建 `deploy/config.yml`（可基于仓库根目录的 `config.example.yml`）：

```yaml
version: 1

server:
  port: 3000

database:
  driver: sqlite
  # Docker 中必须是绝对路径，且位于 /data 卷上
  sqlite_path: /data/starhub.db

security:
  force_secure_cookie: false

initial_options:
  channel_test_interval: 1800
  task_poll_interval: 60
  cookie_secure: false
  ip_rate_limit: 240
  user_rate_limit: 120
  token_rate_limit: 60
  model_rate_limit: 200
  upstream_protection: '{"ordinary_fallback":true,"fallback_interval_seconds":30}'
  channel_traffic_allocation: '{"rules":[]}'
```

要点：

- 必须 `version: 1`；未知字段或非法值会导致启动失败。
- `sqlite_path` 用绝对路径。若写相对路径，会按 YAML 所在目录解析，落到 `/etc/starhub/starhub.db`，不在 `/data` 卷上。
- **不要把密钥写进 `config.yml`**。YAML 没有密钥字段，继续使用 `STARHUB_CRYPTO_SECRET`、`STARHUB_ROOT_PASSWORD` 等环境变量。
- `initial_options` 只对数据库里**缺失**的选项生效。已有 `starhub_data` 卷不会因此改写；要改现有值请用管理后台，或删除卷后重建。

### 挂载到 docker-compose.yml

推荐用 `STARHUB_CONFIG` 指定路径，并只读挂载文件。在现有 `starhub` 服务下增量修改：

```yaml
services:
  starhub:
    image: ${STARHUB_IMAGE:-starhub:local}   # 构建的是 starhub:latest 时见下方说明
    volumes:
      - starhub_data:/data
      - ./deploy/config.yml:/etc/starhub/config.yml:ro
    environment:
      STARHUB_CONFIG: /etc/starhub/config.yml
      # 保留现有 environment 其他项
```

也可以不改 environment，改用 `command` 追加启动参数。Dockerfile 使用 exec 形式的 ENTRYPOINT，参数会被追加：

```yaml
    command: ["--config", "/etc/starhub/config.yml"]
    volumes:
      - starhub_data:/data
      - ./deploy/config.yml:/etc/starhub/config.yml:ro
```

两种方式等价，`STARHUB_CONFIG` 更简单。也可以挂载整个目录：

```yaml
    volumes:
      - starhub_data:/data
      - ./deploy:/etc/starhub:ro
```

### 镜像 tag 说明

Compose 默认使用：

```yaml
image: ${STARHUB_IMAGE:-starhub:local}
```

如果本地构建的是 `starhub:latest`，需要设置：

```powershell
$env:STARHUB_IMAGE="starhub:latest"
```

或在 `.env.docker` 中加入：

```dotenv
STARHUB_IMAGE=starhub:latest
```

然后执行：

```powershell
docker compose --env-file .env.docker up -d --no-build
```

使用 `.env.docker` 时，其中的变量只用于 Compose 的 `${...}` 替换；容器内环境变量仍需在 `environment:` 中声明（例如上面新增的 `STARHUB_CONFIG`）。

### 注意事项

- **挂载路径文件必须存在**：若 `./deploy/config.yml` 不存在，Docker 可能创建一个同名目录，导致启动报错。请先创建文件。
- **权限**：容器以 uid `10001` 的 `starhub` 用户运行。宿主机文件需可读（一般 `0644` 即可）；Linux 下 `0600 root` 会读不到。
- **不要用相对 `sqlite_path`**：否则数据库可能写到 `/etc/starhub`，容器重启后丢失，且与 `/data` 卷备份不一致。
- **YAML 与 env 冲突**：若想让 YAML 的 `port`/`sqlite_path` 生效，需删除 Compose 中对应的 `STARHUB_PORT`、`STARHUB_DB`；但 `ports` 映射仍假设容器内是 `3000`，建议保留 env 或让 YAML 使用一致的值。
- **变更已有数据库选项**：`initial_options` 不会覆盖已存在的值，需在管理界面（如 `/console/gateway-settings`）修改。

简言之：当前 Compose 不加 `config.yml` 就能用；需要加时，写一个指向 `/data/starhub.db` 的 `version: 1` 文件，只读挂到 `/etc/starhub/config.yml`，再用 `STARHUB_CONFIG` 或 `command: ["--config", ...]` 传入即可。

## 环境变量默认值与优先级

`docker-compose.yml` 提供网关及 resource 连接的默认值；合并 `docker-compose.resource.yml` 后，新 resource 与网关共用项目默认网络。配套部署要求显式提供非空 `METRIC_AUTHORIZATION`；`.env.docker` 是配置覆盖来源。

主要默认值：

| 变量 | 开发默认值 |
|---|---|
| `STARHUB_IMAGE` | Compose 为 `starhub:latest`，模板为 `starhub:local` |
| `STARHUB_PLATFORM` | `linux/amd64` |
| `STARHUB_HOST_IP` | `127.0.0.1` |
| `STARHUB_HOST_PORT` | `3008` |
| `STARHUB_COOKIE_SECURE` | `false` |
| `STARHUB_TRUSTED_PROXIES` | 空 |
| `STARHUB_ADMIN_IP_WHITELIST` | 空 |
| `TZ` | `Asia/Shanghai` |
| `NODE_NAME` | `starhub-node-1` |
| `GOPROXY` | `https://proxy.golang.org,direct` |
| `STARHUB_ROOT_PASSWORD` | `docker-compose.yml` 中的开发默认值 |
| `STARHUB_CRYPTO_SECRET` | `docker-compose.yml` 中的开发默认值 |
| `METRIC_AUTHORIZATION` | 必填，`init-deploy.ps1` 为新部署生成独立令牌 |
| `RESOURCE_IMAGE` | Compose 为 `resource:latest`，模板为 `starhub-resource:latest` |
| `RESOURCE_HOST_IP` | `0.0.0.0` |
| `RESOURCE_HOST_PORT` | `3009` |
| `RESOURCE_COST_FILE` | `./deploy/resource/cost_resources.json`，宿主机上必须存在 |
| `NEW_API_BASE_URL` | `http://starhub:3000` |
| `RESOURCE_CATALOG_NOTIFY_URL` | `http://starhub-resource:3001` |
| `RESOURCE_CATALOG_NOTIFY_ENABLED` | `1` |
| `ROUTING_PROTECTION_SOURCE` | `talen` |
| `ROUTING_OBSERVATION_V2_ENABLED` | `1` |

优先级从高到低：

1. Shell 环境变量。
2. `docker compose --env-file <file>` 指定的文件（推荐 `.env.docker`）。
3. Compose 自动加载的仓库根目录 `.env`（仅在不使用 `--env-file` 时）。
4. `docker-compose*.yml` 中的 `${VAR:-default}` 默认值。

注意事项：

- 仓库根目录 `.env` 仍用于本地直接运行 Go，其中包含 `STARHUB_CRYPTO_SECRET`。如果直接 `docker compose up` 而不带 `--env-file .env.docker`，Compose 会自动加载 `.env`，其加密密钥会覆盖 Compose 默认值，可能与已有 `/data` 卷的密钥不一致并导致无法解密。请始终使用 `--env-file .env.docker`。
- `.env.docker.example` 中的密钥和 `docker-compose*.yml` 中的默认值仅用于本地开发。`init-deploy.ps1` 创建 `.env.docker` 时会整行替换这些值，并生成新的随机 root 密码、加密密钥和资源令牌，因此模板中的默认值不会进入正式部署。
- 这些默认密钥已进入受版本控制的文件。不要用于生产，也不要推送到共享仓库；生产环境应使用 `init-deploy.ps1` 生成的私有 `.env.docker`，并妥善备份。
- 需要局域网访问时，在 `.env.docker` 设置 `STARHUB_HOST_IP=0.0.0.0`，并评估未启用 TLS 时的暴露风险。
- 若本地镜像 tag 为 `starhub:latest`，在 `.env.docker` 设置 `STARHUB_IMAGE=starhub:latest`；否则使用 `build-deploy.ps1` 生成的 `starhub:local`。
