# 实施与验收范围

独立的听风项目，不修改 monitor-push 或原型仓库。Golang 单进程内嵌 Vue，SQLite 保存配置、运行状态、可选历史及投递队列。

交付范围：管理员初始化/登录/改密/CLI 恢复；API 参数、请求体及认证；静态网页 CSS 提取；RSS/Atom 去重；独立采集间隔与变化、阈值、每日及间隔通知；多规则、多接收人、多渠道；可靠投递与有限重试；日志按过程开关与保留；历史表、趋势、CSV；配置导出/恢复；清理与空间限制；Docker、Compose、Makefile、文档及自动化验证。

运行限制明确显示：网页采集不执行 JavaScript；首次采集只建立基线；定时发送最近一次成功结果；默认阻止内网/元数据请求，内部来源需显式配置；Webhook 投递是至少一次语义，超时重试可能重复。单实例使用本地 SQLite，不依赖 Redis。

目录：cmd/tingfeng、internal/{config,domain,storage,source,notify,engine,httpapi,auth,secure}、web（嵌入资源）、frontend/src/{components,views,lib}、docs；测试与对应 Go 包和前端模型同目录。

验证：Go 单元与 HTTP 集成测试、竞态检测、Vue 构建、浏览器真实点击、Docker 镜像与低资源容器验收。外部通知以本地可验证的 HTTP/SMTP 接收器测试，不使用真实个人凭据。

交付范围已完成。具体执行结果见 VERIFICATION.md，README 为使用和部署入口。
