# Docker Compose 部署

这套配置参考同级 Talen 的构建部署流程，使用 StarHub 已有的多阶段 Dockerfile 构建前端和 Go 后端。默认部署单实例、纯 Go SQLite，数据保存在 Compose 命名卷 `starhub_data` 中。宿主机默认访问地址为 `http://127.0.0.1:3008`，容器内部监听 `3000`。

## 首次启动

需要 Docker Engine 的 Linux 容器模式和支持 `up --wait` 的 Docker Compose 插件。Windows 可使用 Docker Desktop 和 Windows PowerShell 5.1 或 PowerShell 7。构建在容器中完成，无需在宿主机安装 Node、npm 或 Go。

在仓库根目录执行：

```powershell
.\init-deploy.ps1
.\build-deploy.ps1
```

初始化脚本从 `.env.docker.example` 创建私有 `.env.docker`，生成随机 root 初始密码、稳定加密密钥和资源平台服务凭据，不在终端输出这些值，也不覆盖已有文件。从该文件读取 root 初始密码，用户名为 `root`。如果已手动复制模板，需要自行填写必填项；初始化脚本不会改写它。

部署脚本先验证 Compose 配置，再构建镜像；构建成功后通过 Compose 更新服务，等待 `/api/health` 检查通过。已有数据卷会保留，脚本不会执行 `down -v`。初始化 root 密码只对新数据库生效，不能用修改环境变量的方式重置已有账号密码。容器重启会清除当前进程内的登录会话，需要重新登录。

`.env.docker` 与本地直接启动 Go 程序使用的 `.env` 分开。所有 Compose 命令都应指定 `--env-file .env.docker`；Shell 中同名环境变量优先于文件。部署脚本中的相对文件路径以仓库根目录解析，可以从其他目录调用。

## 常用操作

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

`docker-compose.resource.yml` 是可选的本地联调覆盖文件。先启动匹配分支的 resource Compose 项目，然后在 StarHub 的 `.env.docker` 中设置：

```dotenv
RESOURCE_NETWORK_NAME=talen-resource-dev
RESOURCE_CATALOG_NOTIFY_URL=http://resource-backend:3001
NODE_NAME=starhub-node-1
```

resource 开发 Compose 默认创建 `talen-resource-dev` 网络，生产 Compose 默认创建 `talen-resource`；以实际部署的网络名为准。StarHub 只加入已有网络，不自动创建或接管 resource 的网络。`NODE_NAME` 在同一部署中应保持稳定。

配置 resource 的 `NEW_API_BASE_URL=http://starhub-backend:3000`，并将其 `METRIC_AUTHORIZATION` 配为与 StarHub 相同的服务凭据。StarHub 一侧填写不带 `Bearer ` 前缀的原始 token；resource 的配置格式以其分支文档为准。已有数据库曾轮换或撤销凭据时，应使用当前有效凭据，不能假定修改初始环境变量会覆盖数据库状态。

```powershell
.\build-deploy.ps1 -WithResource

docker compose --env-file .env.docker -f docker-compose.yml -f docker-compose.resource.yml ps
docker compose --env-file .env.docker -f docker-compose.yml -f docker-compose.resource.yml logs --tail 100 starhub
```

后续更新继续使用 `-WithResource` 或同一组 `-f` 参数。此覆盖文件使用独立别名 `starhub-backend`，可以和 Talen 的 `talen-backend` 区分；它不会修改 resource 配置，也不会自动切换其上游或启用 `resource_provider_mode=commercial`。商业路由模式通过现有管理配置启用，首次收到有效快照前业务请求会被就绪门槛阻挡。切换既有 resource 的上游前，应核对目录来源与历史数据，详见 [RESOURCE_PLATFORM.md](RESOURCE_PLATFORM.md)。

此覆盖文件只提供共享 Docker 网络下的本地联调连接，不配置 mTLS。跨主机或生产环境的双向认证代理、访问边界和证书应按 [RESOURCE_PLATFORM.md](RESOURCE_PLATFORM.md) 配置；共享网络本身不代表这些条件已经满足。

## 数据与密钥

`/data` 卷保存 SQLite 及其 WAL。`.env.docker` 中的 `STARHUB_CRYPTO_SECRET` 和可选的版本化 wrapping keys 必须保留，使用已有数据库时应恢复原密钥，不要重新生成。密钥与数据库的备份恢复步骤见 [CREDENTIAL_STORAGE.md](CREDENTIAL_STORAGE.md) 和 [PRODUCTION.md](PRODUCTION.md)。运行中的 SQLite 应使用已有快照功能备份，不要仅复制主数据库文件。

`.env`、`.env.*`、本地数据及构建缓存已排除在 Docker 构建上下文之外。环境模板可以提交，生成的 `.env.docker*` 私有文件由 Git 忽略。普通停止或更新保留卷，`docker compose down -v` 会删除数据卷，因此不属于日常更新流程。
