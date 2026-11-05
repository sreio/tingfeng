# 听风 Tingfeng

个人使用的轻量监控与通知服务。Go 单进程内嵌 Vue 页面，SQLite 保存数据。关闭浏览器后服务器继续运行，无需 Redis、外部数据库或浏览器渲染进程。本项目独立于 monitor-push 和原型。

## 功能

- HTTP JSON：GET / POST、参数、请求头、请求体、JSON 字段提取。
- 静态网页：CSS 选择器提取文本或属性；RSS / Atom / RDF 订阅与新条目去重。
- 分别配置采集与发送：变化、越过阈值、新条目、每 N 分钟/小时/天、每天定时；支持跨午夜时段。
- 一人绑定多个通知方式，每条提醒单独选择渠道；企业微信、钉钉、飞书、Server 酱、SMTP 邮箱和自定义 Webhook。
- 三步新建监控，内联创建接收人与渠道、真实采集测试、字段点选、规则试算、消息预览与真实发送测试。
- 暂停、立即检查、复制草稿、多条提醒、异常状态、失败重试。
- 可选历史、数据表、数值趋势、CSV；每个监控独立设置天数、条数上限、日志级别及记录阶段。
- 初始化密码、登录、修改/重置密码、配置导入导出、完整数据库备份。

## Docker 启动

安装 Docker 和 Compose v2，在项目目录运行 `make up`，打开 http://127.0.0.1:8080 。

1. 执行 `make setup-token`，在自己的终端读取初始化令牌。
2. 页面输入令牌并设置密码；初始化成功后令牌删除，没有默认密码。
3. 新建监控：采集什么 → 何时提醒 → 发给谁。最后一步可以直接添加“我自己”和渠道。

没有 make 时运行 `docker compose up -d --build`；读取令牌用 `docker compose cp tingfeng:/data/setup.token - | tar -xO`。镜像没有 shell，不能在容器里执行 cat。

| 命令 | 用途 |
| --- | --- |
| make up / make down | 启动 / 停止，不删除数据卷 |
| make status / make logs | 容器状态 / 最近日志 |
| make setup-token | 读取初始化令牌；初始化后文件不存在是正常现象 |
| make reset-password | 使旧登录失效，保留监控并重新进入初始化 |
| make backup | 短暂停止服务，复制完整数据卷到 backups/时间戳，再自动启动 |
| make build / make run | 构建单文件程序 / 本地运行 |
| make test / make verify | 竞态及前端逻辑测试 / 测试、生产构建、go vet |

`docker compose down -v` 会删除数据卷，请勿当作日常停止命令。

## VPS 与资源

默认一个采集 worker，最多四个。Compose 限制 1 CPU / 192 MiB，Go 内存软限制 96 MiB；这不是最低配置保证，实际占用随响应和任务数量变化。单次响应限制 2 MiB、提取值 32 KiB、订阅结果 128 KiB，待发送队列最多 1000 条。

低配机器建议从少量监控、5 分钟以上采集间隔、仅记录错误开始。历史默认 7 天 / 10000 条，过程日志 3 天 / 5000 条，发送记录 7 天 / 10000 条。每次执行落实单监控上限，每小时清理到期记录并增量回收 SQLite 空间。

“存储预算”是数据库与 WAL 的软预算，默认 200 MB。达到 90% 时停止新增历史和过程日志；清理可能提前移除最旧历史，优先保留配置、最新状态和待发队列。它不是磁盘硬配额。

**极低内存 VPS 上建议只运行，不编译。** 在开发机按服务器架构构建：

    docker buildx build --platform linux/amd64 -t tingfeng:1.0.0 --load .
    docker save tingfeng:1.0.0 | gzip > tingfeng-amd64.tar.gz

传到 VPS 后：

    gunzip -c tingfeng-amd64.tar.gz | docker load
    docker tag tingfeng:1.0.0 tingfeng:local
    docker compose up -d --no-build

ARM64 使用 linux/arm64。复制项目的 compose.yaml 和需要的 .env；镜像不包含用户数据。

### 访问服务器

默认只绑定回环地址，可用 SSH 转发：

    ssh -L 8080:127.0.0.1:8080 user@your-vps

也可以将 HTTPS 反向代理转发到 127.0.0.1:8080。复制 .env.example 为 .env，设置 `PUBLIC_URL=https://monitor.example.com`、`COOKIE_SECURE=true` 后重新启动。PUBLIC_URL 必须与浏览器实际访问的 origin 一致，代理应保留 Host。需要其他监听地址时修改 BIND_ADDRESS。

密码使用 PBKDF2 派生保存；Cookie 为 HttpOnly / SameSite=Strict；写操作校验 CSRF；HTTPS 入口使用 Secure Cookie。

## 监控与通知语义

首次成功采集只建立变化、阈值、RSS 基准，不批量推送旧数据。阈值表示**越过边界**，持续高于阈值不会每次通知。关闭历史不影响最新值和比较；已有历史按策略清理，不立即删除。

全天间隔从启用时间计时；暂停后重启监控重新计时，重启程序不会重置。指定时段按每天开始时刻对齐，结束时刻不发送，支持跨午夜。每日定时使用系统设置的时区；修改时区或提醒配置会重新计算。界面显示未来三次发送时间。

离线期间错过的发送时刻最多合并为一次，避免重启刷屏。限定时段外不会补发，已过期的排队重试也会跳过。没有成功数据时跳过；有成功数据但新采集失败时，定时消息仍携带最近成功值及真实采集时间。来源没有更新时间时明确标注。

网页只支持静态 HTML，不执行 JavaScript；动态网页请使用其数据接口。数据报告提供基础数值趋势与 CSV，复杂报表可后续扩展。

### 提取多个字段、换算与消息模板

第一步可从测试响应连续添加字段，或手动填写 JSON 路径 / 网页 CSS 选择器，每个来源最多 16 个字段。每个字段有独立的名称、变量名、单位和换算规则，支持加、减、乘、除及仅设置小数位。变量名需以字母开头，使用字母、数字、下划线且不重复。

例如把字段原值 `90441` 配置为「除以 100，保留 2 位」会得到 `904.41`；单位由用户填写。换算采用十进制运算并四舍五入，结果以十进制字符串保存以保留精度。单位是显示文字；变化判断、阈值和历史记录使用换算后的数值。字段不存在、非数字参与换算或除数为零时会明确报错，不发送不完整结果。

变化提醒可选择「任一字段有变化」或一个指定字段；阈值提醒默认判断第一个字段，也可切换到其他字段。历史记录保存当时的字段名称、原值、换算结果和单位，不随后续配置修改。列表默认展示第一个字段，详情和历史可查看全部字段，CSV 增加「字段快照 JSON」列。

第三步的「消息内容」提供默认消息和自定义模板，点击变量可插入光标位置，预览与实际发送使用同一个渲染器。例如：

```text
{{.Title}}
买入价：{{.Fields.buyprice}} {{.Units.buyprice}}
卖出价：{{.Fields.sellprice}} {{.Units.sellprice}}
采集时间：{{.CollectedTime}}
```

`Fields` 是换算后的显示值，`Units` 是单位，`RawFields` 可读取换算前的值。变量名与提取字段中的配置一致；模板引用不存在的变量会报错。已有单值监控继续兼容，可点击「添加字段 / 设置数值换算」后升级并重新测试。

变量按钮带中文名称、用途说明和示例。点击会在光标位置插入变量，也可替换选中文字；示例只解释含义，实际发送内容以下方预览为准。「采集时间」是听风取到数据的时间，「来源更新时间」由数据源提供，两者不一定相同。

本版本数据库结构升级为 2，首次启动自动为历史表添加字段快照列，原记录保留。升级前请备份；旧程序不能直接打开版本 2 数据库，回退需要恢复升级前的备份。

### 内部网络来源开关

在「系统设置 → 运行与存储 → 内部网络来源」直接切换，自动保存，重启后保留，无需改文件。未在页面设置过时，沿用 `TINGFENG_ALLOW_PRIVATE`（Docker 对应 `ALLOW_PRIVATE`）的默认值；一旦在页面设置，页面值优先，关闭也可以覆盖环境变量的 `true`。

开启后采集及通知渠道（含邮件）可访问服务器内网、本机和保留地址，只应配置可信目标。关闭后新请求立即恢复公网限制，HTTP 重定向和复用连接也遵守新策略；已发出的请求不会中断。只有通过登录和 CSRF 验证的管理员才能修改。恢复配置备份时保留当前实例的网络开关，避免导入监控时意外扩大访问范围。

### 渠道配置

| 类型 | 需要配置 |
| --- | --- |
| 企业微信 | 群机器人 Webhook 地址 |
| 钉钉 | 机器人地址、可选签名密钥；有关键词限制时在消息模板包含关键词 |
| 飞书 | 机器人地址和可选签名密钥 |
| Server 酱 | SCT 或 sctp 开头的 SendKey |
| 邮箱 | SMTP 主机、端口、TLS / STARTTLS、发件地址和授权码；收件地址配置在接收人上 |
| Webhook | 地址、POST / PUT、可选请求头和 JSON 模板 |

官方说明：[企业微信](https://developer.work.weixin.qq.com/document/path/99110)、[钉钉](https://open.dingtalk.com/document/robots/customize-robot-security-settings)、[飞书](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)、[Server 酱](https://sct.ftqq.com/docs/getting-started/sendkey/)。

保存的鉴权、机器人地址、密钥不会通过配置 API 回显；编辑留空保留。更换来源域名或 SMTP 主机/账号后，需重新填写相应认证；更换渠道类型不沿用旧密钥。

消息支持 Go text/template，变量：.Title、.Monitor、.Value、.Previous、.Rule、.SourceURL、.SourceTime、.CollectedTime、.Text、.Items、.Fields、.Units、.RawFields。Webhook JSON 示例：

    {"title": {{json .Title}}, "content": {{json .Text}}, "value": {{json .Value}}}

自由文本用 json 函数转义，不要直接加引号。邮箱强制加密连接并校验证书。

通知先入 SQLite 队列，失败最多尝试 3 次，间隔约 10 秒、1 分钟。进程中断后通过 60 秒租约恢复。手动渠道测试只尝试一次，也记录结果。修改/暂停/删除监控会取消旧代次待发消息；外部正在处理的请求无法撤回。

采用“至少一次”投递：外部已收到、程序却在保存成功状态前崩溃时，重试可能重复。Webhook 可用 X-Tingfeng-Delivery-ID 去重，邮件重试保持同一 Message-ID。

## 备份和恢复

页面配置导出不含历史、管理员密码、渠道密钥和请求鉴权。恢复按 ID 合并，同 ID 覆盖，其余保留。导入的监控暂停待检查、渠道停用；填写密钥并测试后再启用。

**完整备份必须同时保留 tingfeng.db 和 master.key。** 丢失密钥无法解密配置；同时取得二者的文件系统访问者可以解密，因此备份应私密保存。

Docker 使用 `make backup`，短暂停止服务保证一致性，复制整个数据目录到 backups 后自动恢复运行，不在数据卷内堆积备份。

恢复时先备份当前实例并停止，使用新的空数据卷，避免旧 WAL 混入。可修改 compose.yaml 的卷名称后执行 `docker compose create --force-recreate`，再将备份复制到 /data。运行 UID/GID 为 65532:65532，恢复后需保证数据目录归该用户所有。最后 `docker compose start`。完整恢复保留原密码及调度状态，可能继续投递未完成消息。

宿主二进制支持在线一致性备份：

    TINGFENG_DATA_DIR=./data ./bin/tingfeng backup --out ./backups/manual-20261105

恢复时停止服务，将备份目录作为新的 TINGFENG_DATA_DIR 启动。重置管理员执行 `./bin/tingfeng reset-password --confirm`，并指定原数据目录。

## 开发与目录

需要 Go 1.26（go.mod 工具链 1.26.10）、Node 24 和 npm。运行时无需 Node。`make build` 构建，`./bin/tingfeng` 运行。

开发使用两个终端：`make dev-api`（8080）与 `make dev-web`（5174，代理 API）。PUBLIC_URL 留空。首次直接 go run 前可先 make web。

| 环境变量 | 默认 | 用途 |
| --- | --- | --- |
| TINGFENG_DATA_DIR | ./data | 数据与密钥目录 |
| TINGFENG_LISTEN | 127.0.0.1:8080 | HTTP 监听 |
| TINGFENG_PUBLIC_URL | 空 | 外部 HTTPS origin |
| TINGFENG_COOKIE_SECURE | false | 限制 Cookie 使用 HTTPS |
| TINGFENG_ALLOW_PRIVATE | false | 内网访问初始默认值；系统设置页面保存的开关优先 |
| TINGFENG_WORKERS | 1 | 采集并发 1～4 |
| GOMEMLIMIT | Docker 96MiB | Go 软内存目标 |
| GOMAXPROCS | Docker 1 | 最大并行 Go 执行线程 |

默认阻止内网、回环、链路本地和云元数据请求；重定向重新校验地址，跨域清除请求头。允许内网必须在服务器显式配置。

    cmd/tingfeng/       入口、CLI、生命周期
    internal/auth/     密码、初始化、会话、限流
    internal/config/   环境变量
    internal/domain/   模型、校验、凭据保留
    internal/engine/   采集调度、规则、投递调度
    internal/httpapi/  API、CSV、备份、静态页面
    internal/notify/   通知渠道、模板、SMTP
    internal/secure/   加密与网络边界
    internal/source/   JSON、HTML、RSS/Atom 提取
    internal/storage/  SQLite、历史、日志、队列、清理
    frontend/src/      Vue 页面、组件与表单
    web/               构建资源嵌入 Go
    docs/              架构与验收
    Dockerfile         多阶段构建、非 root、无 shell
    compose.yaml       数据卷、健康检查、资源限制
    Makefile           开发、构建、验证、部署

SQLite 数据目录应位于本机磁盘；不支持多个实例共享同一目录。扩展渠道时修改 domain 校验、notify 分发和前端渠道表单，并增加协议测试。
