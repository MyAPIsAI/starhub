# 现有 StarHub 与新 resource 配套部署

使用远程 `/root/starhub/docker-compose.yml` 与新增 `docker-compose.resource.yml` 合并部署。两项服务统一属于 `starhub` 项目，保留现有 `starhub-starhub-1` 和 `starhub_starhub_data`，新增 `starhub-resource-1`、`starhub_resource_data`，共用 `starhub_default`。旧 `/root/resource` 项目不参与此次部署。

## 准备文件与镜像

保留远程主配置、原环境文件和加密密钥。上传以下文件，目录结构为：

```text
/root/starhub/
  docker-compose.yml                  (existing)
  docker-compose.resource.yml
  .env.docker.resource.example
  deploy/resource/cost_resources.json
```

覆盖文件同时添加 StarHub 配对环境变量和新 resource 服务，不单独启动。使用下文显式 `-f` 命令，无需另复制为 `docker-compose.override.yml`。若已有其他覆盖文件包含必要配置，命令中也须保留它。

准备协议匹配的 resource 镜像；模板使用 `RESOURCE_IMAGE=starhub-resource:latest`，可从匹配源码构建 `docker build -t starhub-resource:latest /path/to/resource`，或导入已构建镜像。Compose 未指定该变量时回退为 `resource:latest`。旧 `resource-dev:latest` 不作为兼容性已验证的替代镜像。

新 resource 默认发布 `0.0.0.0:3009`，容器内为 `3001`。成本文件从 `/root/starhub/deploy/resource/cost_resources.json` 只读挂载，初始内容为 `[]`；不要覆盖已有自定义成本数据。

若只上传了 Compose 文件，启动前还需创建成本文件。同级 resource 的 Compose 同样挂载宿主机成本 JSON，其 Dockerfile 不会复制该文件；缺少文件时不能直接去掉挂载。以下命令只在文件缺失时创建空列表，已有内容保持不变：

```bash
cd /root/starhub
python3 - <<'PY'
import json
from pathlib import Path

path = Path('deploy/resource/cost_resources.json')
path.parent.mkdir(parents=True, exist_ok=True)
try:
    with path.open('x', encoding='utf-8') as output:
        output.write('[]\n')
except FileExistsError:
    pass
if not path.is_file():
    raise SystemExit('Cost path must be a regular JSON file, not a directory.')
if not isinstance(json.loads(path.read_text(encoding='utf-8-sig')), list):
    raise SystemExit('Cost file must contain a JSON array.')
print('Cost file ready.')
PY
```

已有 StarHub 专用成本文件时，可在 Compose 使用的环境文件中设置 `RESOURCE_COST_FILE=/absolute/path/cost_resources.json`，不必移动文件。容器内路径及 `COST_DATA_PATH` 仍为 `/etc/resource/cost_resources.json`；不要将 StarHub 指向旧 resource 的成本文件。保留 `create_host_path: false`，避免文件缺失时被自动创建成目录。

如果 StarHub 已配置正确配对地址及有效令牌，只需启动 resource：

```bash
docker compose --env-file /root/starhub/.env.docker.resource \
  -p starhub -f /root/starhub/docker-compose.yml -f /root/starhub/docker-compose.resource.yml \
  up -d --no-deps --no-build --pull never --wait resource
```

此命令不会更新 StarHub 的环境变量；令牌须与正在运行的 StarHub 匹配。环境文件沿用实际部署文件，下文说明了如何保留原私有环境设置。

如果此前已部署独立的 `starhub-resource` 项目，须先备份并停用该新 resource，迁移其 `starhub-resource_resource_data` 卷数据或显式配置复用原卷，再启动合并配置。合并不会自动迁移数据库，两个新 resource 也不能同时占用端口和网络别名。此前检查远程时尚未部署该独立项目；执行时以实际状态为准。

## 配对环境变量

在远程创建仅包含配对设置的私有环境文件；重复执行保留已有文件：

```bash
cd /root/starhub
python3 - <<'PY'
from pathlib import Path
import os
import secrets

os.umask(0o077)
target = Path('.env.docker.resource')
if not target.exists():
    text = Path('.env.docker.resource.example').read_text()
    assert '\nMETRIC_AUTHORIZATION=\n' in text
    text = text.replace('\nMETRIC_AUTHORIZATION=\n', '\nMETRIC_AUTHORIZATION=' + secrets.token_hex(32) + '\n')
    with target.open('x') as output:
        output.write(text)
print('Pairing environment ready; credentials were not printed.')
PY
```

该文件的 `METRIC_AUTHORIZATION` 只保存非空裸令牌。StarHub 容器接收裸令牌；resource 服务配置自动添加 `Bearer ` 前缀，用作完整 Authorization 请求头。不要将共享文件改成带前缀的值。resource 回连 `http://starhub:3000`，StarHub 通知 `http://starhub-resource:3001`。若 StarHub 已配置、轮换过服务凭据，应填写当前有效凭据，不能通过随机生成初始令牌覆盖数据库中的凭据状态。

以下命令适用于原主配置内已固定网关设置的部署。如果原部署依赖 `.env.docker` 或其他私有环境文件，应在每条命令中先加 `--env-file /root/starhub/.env.docker`，再保留 `--env-file /root/starhub/.env.docker.resource`，避免丢失原镜像、端口或加密参数。后一个文件仅覆盖配对变量。

## 部署与后续更新

```bash
docker image inspect starhub-resource:latest --format '{{.Id}}'
docker compose --env-file /root/starhub/.env.docker.resource \
  -p starhub -f /root/starhub/docker-compose.yml -f /root/starhub/docker-compose.resource.yml \
  config --quiet

docker compose --env-file /root/starhub/.env.docker.resource \
  -p starhub -f /root/starhub/docker-compose.yml -f /root/starhub/docker-compose.resource.yml \
  up -d --no-build --pull never --wait
```

若更改了 `RESOURCE_IMAGE`，镜像检查命令也应使用对应标签。Compose 会因新增配对环境变量重建 StarHub，并等待其健康后启动 resource。StarHub 数据卷保留，内存登录会话需要重新建立。单独 `docker restart` 不会加载新的 Compose 环境变量。

后续更新任一镜像后，继续执行相同 `up` 命令；不要遗漏覆盖文件或原私有环境文件。`update-image.sh` 更新镜像后仍需通过 Compose 更新容器。

## 核验与停止

```bash
docker compose --env-file /root/starhub/.env.docker.resource \
  -p starhub -f /root/starhub/docker-compose.yml -f /root/starhub/docker-compose.resource.yml ps
docker compose --env-file /root/starhub/.env.docker.resource \
  -p starhub -f /root/starhub/docker-compose.yml -f /root/starhub/docker-compose.resource.yml \
  logs --tail 100 starhub resource

docker exec starhub-starhub-1 curl --fail --silent http://starhub-resource:3001/api/health
docker exec starhub-resource-1 wget -q -O - http://starhub:3000/api/health
```

健康检查验证进程、数据库及网络连通。还需确认目录同步和路由状态无鉴权、协议或快照错误，再按业务需要启用商业路由模式；Compose 不会自动切换该模式。

日常更新使用 `up -d`。需要停止整套服务时，使用相同环境文件及两个 `-f` 参数执行 `stop`；拆除容器和项目网络时执行 `down`，保留数据卷，不加 `-v`。旧 resource 属于另一项目，不在这些命令范围内。

测试部署将项目名改为 `-p starhub-test`，自动使用 `starhub-test_default` 及独立数据卷，同时为两项服务配置不同的宿主机端口、独立凭据和节点名，无需 `RESOURCE_NETWORK_NAME`。
