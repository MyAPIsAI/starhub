已查看远程。现有 `resource` 正在服务旧 `new-api`；建议给 StarHub 单独部署一套，使用 `starhub-resource` 作为统一前缀。

| 项目 | 现有 resource | StarHub 配套 resource（建议） |
|---|---|---|
| 部署目录 | `/root/resource` | `/root/starhub-resource` |
| Compose 项目名 | `resource` | `starhub-resource` |
| 容器名 | `resource-dev` | `starhub-resource` |
| 镜像 | `resource-dev:latest` | `starhub-resource:latest` |
| 宿主机端口 | `3001` | `3011`，检查时未占用 |
| 对接网关 | `http://new-api:3000` | `http://starhub-backend:3000` |
| 数据库和数据卷 | 旧 resource 专用 | 新建独立数据库、数据卷 |
| 对接网络 | 旧 new-api 网络 | `starhub-resource-control` |

**只改项目名还不够。** Compose 项目名可以隔离默认命名的资源，但旧配置写死了 `container_name`，MySQL 卷还绑定了固定宿主机目录；复制部署时必须同时调整，避免容器重名或共用旧数据库。[Docker 项目隔离说明](https://docs.docker.com/compose/how-tos/project-name/)、[数据卷说明](https://docs.docker.com/reference/compose-file/volumes/#name)。

新 resource 与 StarHub 的连接应配置为：

```dotenv
# New resource
RESOURCE_NETWORK_NAME=starhub-resource-control
NEW_API_BASE_URL=http://starhub-backend:3000
```

```dotenv
# StarHub
RESOURCE_NETWORK_NAME=starhub-resource-control
RESOURCE_CATALOG_NOTIFY_URL=http://resource-backend:3001
NODE_NAME=starhub-node-1
```

两个容器加入同一个专用网络，分别使用 `resource-backend`、`starhub-backend` 别名。容器内部端口仍为 resource `3001`、StarHub `3000`；宿主机映射端口分别为 `3011`、现有的 `3008`。新配对使用独立服务令牌，两侧保持匹配。

目前 StarHub 只连接 `starhub_default` 网络，尚未接入 resource；仓库中的 `docker-compose.resource.yml` 只是连接配置，**不会创建 resource 服务**。

本次仅检查，未修改或重启远程服务。

## 配置落实记录

以上为远程检查后的分析原文。随后按该分析在当前 StarHub 仓库落实配置：

- `docker-compose.resource.yml` 保留网关连接覆盖文件的用途，默认连接独立网络 `starhub-resource-control`。
- 新增 `docker-compose.resource-service.yml`，单独启动 `starhub-resource` Compose 项目，使用镜像 `starhub-resource:latest`，宿主机绑定 `127.0.0.1:3011`，容器内部端口保持 `3001`。
- 实际实现采用 Compose 自动容器名 `starhub-resource-resource-1`，不设置固定 `container_name`；数据卷默认是 `starhub-resource_resource_data`。
- 新 resource 使用独立 SQLite 文件 `/data/resource.db`，不复用旧 resource 的 MySQL、数据卷、配置或宿主机数据目录；独立成本覆盖文件为 `deploy/resource/cost_resources.json`，初始内容为空列表。
- 新 resource 在专用网络使用 `resource-backend` 别名，回连 `http://starhub-backend:3000`；StarHub 保留 `starhub` 项目及其原数据卷，使用 `starhub-backend` 别名。
- 两侧 `METRIC_AUTHORIZATION` 均改为必填，不再提供公共默认令牌；新部署由 `init-deploy.ps1` 生成，旧数据库必须使用当前有效服务凭据。
- `.env.docker.example` 和 `DOCKER_DEPLOYMENT.md` 同步更新。已有私有 `.env.docker` 不会自动迁移，部署前应更新其中的旧网络值，同时保留原数据库加密密钥。

两个项目分别使用各自的 Compose 文件启动，resource 服务文件不能通过 `-f` 合并进网关项目。先准备匹配当前协议的 resource 镜像并启动新 resource，再使用网关连接覆盖文件更新 StarHub。具体命令见 [DOCKER_DEPLOYMENT.md](DOCKER_DEPLOYMENT.md)。

本轮仅修改当前仓库配置，未部署或重启远程服务。

配置验证通过：使用 Docker Compose 检查默认配置、独立实例的网络/镜像/端口覆盖及项目数据卷隔离；检查两侧缺少服务令牌时均拒绝启动；检查初始化脚本能生成新令牌，并由两个项目一致读取。本轮未构建 resource 镜像或执行容器启动，因此不将配置校验视为完整的服务联调。

## 主 Compose 连接配置补充

根据后续反馈，resource 地址、通知开关、服务令牌、保护来源、观测开关及专用网络连接均移入主 `docker-compose.yml`。主文件默认使用 `RESOURCE_CATALOG_NOTIFY_URL=http://resource-backend:3001`，加入 `starhub-resource-control` 网络，并发布 `starhub-backend` 网络别名；启动 StarHub 不再需要额外覆盖文件或 `-WithResource`。原 `docker-compose.resource.yml` 保留为空的兼容覆盖文件，旧命令仍然有效。

应先启动独立 resource 项目以创建共享网络，再启动 StarHub。resource 的宿主机发布端口与容器内部端口不同，网关在共享网络始终使用 `resource-backend:3001`，不应改成宿主机的 `3009` 或 `3011`。当前 resource 服务文件的宿主机默认映射保留为 `0.0.0.0:3009`；使用环境模板时，其变量会覆盖为 `127.0.0.1:3011`。

## 按远程实际网络调整

后续远程核实：`starhub_default` 由 Compose 项目 `starhub` 管理，连接正式容器 `starhub-starhub-1`，已有服务别名 `starhub`；`starhub-test_default` 由测试项目 `starhub-test` 管理，核查时为空。旧 `resource-dev` 仍连接旧 new-api 的网络。

据此替代此前新建 `starhub-resource-control` 的方案：主 `docker-compose.yml` 只使用网关自身的默认网络，保留其原项目名、端口及数据卷；resource 的网络改为 `external: true`，默认加入已有的 `starhub_default`。resource 回连地址使用远程已有的服务别名 `http://starhub:3000`；网关访问 resource 的地址仍为 `http://resource-backend:3001`。

首次部署顺序相应改为先启动 StarHub 创建默认网络，再启动 resource 加入；已有正式网络时无需另建网络。测试环境应以 `-p starhub-test` 启动网关，并在测试 resource 的独立环境文件中指定 `RESOURCE_NETWORK_NAME=starhub-test_default`，避免接入正式网络。网关的默认网络始终由其 Compose 项目名决定，resource 只加入、不创建或管理该网络。

环境模板、本地私有环境文件及部署文档同步调整，现有密钥和宿主机端口保持原样。本轮远程仅查询网络和容器信息，未重启或修改远程服务。

## 现有 StarHub 与新 resource 的最终配套复核

本次进一步核对远程实际配置：旧 `resource-dev` 属于 `resource` 项目，使用 `resource-dev:latest`、宿主机 `3001`、旧 MySQL 和旧 new-api 网络；现有 `starhub-starhub-1` 使用 `/root/starhub/docker-compose.yml`、`starhub:latest`、宿主机 `3008` 和 `starhub_starhub_data` 卷。StarHub 尚未配置 resource 地址或服务令牌，数据库也没有持久化的 resource 凭据记录。远程尚无 `/root/starhub-resource` 目录和 `starhub-resource:latest` 镜像。

当前最终配套为：

- 新 resource 使用独立目录 `/root/starhub-resource`、项目 `starhub-resource`、镜像 `starhub-resource:latest`、容器 `starhub-resource-resource-1` 和 SQLite 数据卷 `starhub-resource_resource_data`，不复用旧 resource 数据。
- 新 resource 的宿主机端口在 Compose 和环境模板中统一为 `0.0.0.0:3009`，不再被模板中的 `3011` 覆盖。
- 新 resource 以外部网络方式加入已有 `starhub_default`，使用专属别名 `starhub-resource`，保留 `resource-backend` 兼容别名。网关访问 `http://starhub-resource:3001`；新 resource 回连 `http://starhub:3000`。
- `docker-compose.resource.yml` 从空兼容文件补充为可升级远程旧主配置的增量文件，只新增配对环境变量。部署到 `/root/starhub/docker-compose.override.yml` 后，与远程原主文件合并使用；不替换现有主文件及加密设置。
- 新增 `.env.docker.resource.example`，只包含配对所需变量，不包含网关镜像、端口、root 密码或加密密钥。远程私有令牌文件由两侧共同读取，后续重建 StarHub 继续使用相同文件和覆盖配置。
- 完整文件位置、凭据准备、部署顺序、重建和验证命令见 [远程配对部署步骤](deploy/resource/REMOTE_DEPLOYMENT.md)。现有 StarHub 已创建网络，可先部署新 resource，再重建现有网关注入配对配置；仅重启容器不会读取新的 Compose 环境变量。

验证结果：本地 Compose 的两种模板、环境变量覆盖、正式/测试网络与数据卷隔离、空令牌拒绝均通过；远程将增量配置经标准输入与原主文件合并，只新增 `RESOURCE_CATALOG_NOTIFY_URL`、`RESOURCE_CATALOG_NOTIFY_ENABLED`、`METRIC_AUTHORIZATION`、`ROUTING_PROTECTION_SOURCE`、`ROUTING_OBSERVATION_V2_ENABLED` 五项，确认原镜像、端口、数据卷、网络及已有密钥配置完全保留。该远程验证未写入配置文件、未创建或重启容器，实际部署和端到端同步验证尚未执行。

## 合并为同一 Compose 项目（2026-11-24）

按最新要求，`docker-compose.resource.yml` 同时包含 StarHub 配对环境变量和完整 resource 服务，取代并移除 `docker-compose.resource-service.yml`。本节替代前述独立项目方案，前文保留为历史记录。部署时合并 `docker-compose.yml` 与 `docker-compose.resource.yml`，项目名继续使用 `starhub`；新容器为 `starhub-resource-1`，独立 SQLite 数据卷为 `starhub_resource_data`，与现有 `starhub-starhub-1`、`starhub_starhub_data` 共用 `starhub_default` 网络。旧 resource 项目保持独立。

resource 等待 StarHub 健康后启动，回连 `http://starhub:3000`；StarHub 访问 `http://starhub-resource:3001`。宿主机 resource 端口保留 `0.0.0.0:3009`。两个服务读取同一环境文件中的 `METRIC_AUTHORIZATION`，resource 要求非空。Compose 镜像回退值保留用户设置的 `resource:latest`，环境模板仍可覆盖为专用标签 `starhub-resource:latest`。不再使用 `RESOURCE_NETWORK_NAME`；测试项目通过 `-p starhub-test` 自动使用对应默认网络及独立数据卷。

`build-deploy.ps1 -WithResource` 已改为同时启动并检查两个服务；脚本只构建 StarHub，resource 镜像须提前准备。两份环境模板、主部署文档和远程操作说明已同步，远程配套文件统一放在 `/root/starhub` 下，继续使用远程原主配置及其私有环境设置。若此前已运行独立的新 resource 项目，必须先停用它并处理原 `starhub-resource_resource_data` 卷数据；合并不会自动迁移数据。

本轮验证：两种环境模板的 Compose 展开均通过，确认合并前后本地 StarHub 服务定义一致、双方凭据匹配、正式与测试项目网络及卷隔离、成本文件绑定路径正确、resource 健康依赖有效、默认镜像保留且缺少令牌时拒绝配置。PowerShell 语法及模拟 Docker 的配套部署、单网关部署、仅构建分支检查通过。本轮仅修改本地配置和文档，未启动容器或修改远程服务，未执行端到端同步验证。
