# Biaoge BI

自托管商业智能平台：对接多种外部数据库，编写可复用的 SQL 数据集，通过拖拽式仪表盘和 ECharts 图表展示数据，支持多用户/角色、公开分享、Excel 导出与定时报表。

技术栈：Go 1.26 + Gin + GORM · Vue 3 + TypeScript + Element Plus + ECharts。前后端可编译为单个可执行文件。

## 功能

- **数据源**：MySQL / PostgreSQL / Oracle（纯 Go go-ora 驱动，无需 Instant Client）/ SQLite；连接测试、连接池管理、密码 AES-256-GCM 加密存储（API 永不回显）
- **SQL 数据集**：单条只读 SQL、`{{param}}` 参数绑定、列元数据自动推断、带参预览
- **安全查询引擎**：只读语句校验（拦截 DML/DDL/多语句/注释绕过）、只读事务、超时、行数上限、参数绑定防注入、进程内结果缓存
- **图表**：柱状 / 折线 / 饼图 / 表格 / KPI；服务端聚合（SUM/AVG/COUNT/COUNT DISTINCT/MIN/MAX）；中文字段别名
- **仪表盘**：12 栅格拖拽布局、文本组件、下拉/日期/日期区间/数字/文本筛选器、筛选器→图表参数联动、一键预览
- **权限**：JWT 登录、admin/user 角色、仪表盘按用户授权（只读/可编辑）、公开分享链接（可过期、可撤销、限流，不泄露 SQL）
- **导出与报表**：图表/仪表盘导出 Excel；cron 定时生成报表并按 SMTP 发送，执行记录可下载
- **复制与保护**：一键复制图表/仪表盘（含组件、筛选器与联动）；删除数据源/数据集/图表前检查引用，避免产生坏链
- **审计日志**：登录、数据源/数据集/图表/仪表盘变更、授权、分享等操作留痕，管理员可筛选查看
- **企业微信集成**：OAuth 扫码登录、通讯录（部门/成员）定时同步、按部门下发看板（授权上级部门后子部门自动继承）、企微 userid 白名单自动授予管理员
- **时间粒度**：日期维度支持按日/周/月/季/年聚合；多选筛选绑定 `list` 参数生成 `IN` 查询
- **导出与刷新**：图表支持 Excel / CSV（UTF-8 BOM）；仪表盘可设置 15 秒～5 分钟自动刷新
- 元数据库默认 SQLite（纯 Go 驱动、无 CGO），可切换 MySQL

## 快速开始

### 生产模式（单二进制）

```bash
# 1. 构建前端
cd web && npm install && npm run build && cd ..

# 2. 编译（内嵌前端）
go build -trimpath -ldflags="-s -w" -o bi.exe ./cmd/server

# 3. 启动
cp config.example.yaml config.yaml   # 按需修改
./bi.exe -config config.yaml
```

浏览器打开 <http://localhost:2345>。首次启动若无用户，会按配置创建管理员；若 `auth.adminPassword` 留空，则随机生成并只打印一次到控制台日志。

一键脚本：`bash scripts/build.sh`。

### 开发模式

```bash
# 终端 1：后端（dev 标签，不嵌入前端）
go run -tags dev ./cmd/server

# 终端 2：前端（Vite 代理 /api 到 127.0.0.1:2345）
cd web && npm install && npm run dev
```

访问 <http://localhost:5173>。

## 配置

复制 `config.example.yaml` 为 `config.yaml`。所有键都可用 `BI_` 前缀环境变量覆盖（如 `BI_SERVER_PORT`、`BI_AUTH_JWTSECRET`）：

| 配置 | 说明 |
| --- | --- |
| `metadata.driver` / `metadata.dsn` | 元数据库 `sqlite`（默认）或 `mysql` |
| `auth.jwtSecret` | JWT 密钥，至少 32 字节，生产必须修改 |
| `security.secretKey` | 数据源密码加密密钥；留空则由 JWT 密钥派生（保持一致即可跨重启解密） |
| `datasource.*` | 外部数据源连接池、查询超时、结果行数上限 |
| `smtp.*` | 定时报表邮件；未启用时仍可生成并下载报表 |
| `wecom.*` | 企业微信 OAuth 登录与通讯录同步（CorpID / AgentId / Secret / RedirectURI / 管理员白名单） |
| `scheduler.*` | 调度开关、报表目录、保留天数 |

> Linux/macOS 下把 `metadata.dsn` 默认值中的 `data/` 目录创建好即可，程序会自动创建父目录。

## 演示数据

仓库附带演示数据生成与种子脚本，一条命令得到一个完整可用的演示工作区：

```bash
# 1. 生成演示 SQLite 库（约 1 万笔订单、12 区域、42 产品、300 客户）
python scripts/gen_demo_data.py data/demo_sales.db

# 2. 启动服务后灌入演示内容（幂等，可重复执行）
./bi.exe -config config.yaml        # 另一个终端
BI_ADMIN_PASSWORD=<管理员密码> python scripts/seed_demo.py
```

种子脚本会创建：
- 数据源「演示销售库」
- 数据集「销售明细」（日期区间参数）、「区域目标」、「地区列表」
- 10 个图表：KPI ×3、折线趋势、地区/渠道/客户分层柱图、品类饼图、产品 TOP20、订单明细表
- 仪表盘「销售分析驾驶舱」（11 个组件、2 个日期筛选器、20 条联动绑定）
- 演示用户 `analyst / analyst12345`、`viewer / viewer12345`
- 一个公开分享链接

打开 `http://localhost:2345/dashboards/1/view`，切换日期筛选器可看到所有图表联动刷新。

## 企业微信集成

1. 企业微信管理后台创建**自建应用**，记录 `corpId`、`agentId`、`secret`
2. 在「应用管理 → 网页授权及JS-SDK」配置可信域名，把回调地址填入 `wecom.redirectUri`（如 `https://bi.example.com/wecom/callback`）
3. 授权通讯录读取权限（或单独配置 `wecom.contactSecret`）
4. 启动服务后进入「系统设置 → 企业微信集成」点击「立即同步通讯录」
5. 登录页出现「企业微信扫码登录」；员工首次扫码自动开通账号
6. 在仪表盘「分享与授权 → 部门授权」把看板下发给部门，员工只看到自己部门及被下发上级部门的看板

权限规则：

- 员工所属部门来自企微通讯录，支持一人多部门
- 授权给上级部门后，其所有**子部门**员工自动继承（只读/可编辑）
- 个人授权优先于部门授权合并（取更高权限）
- 管理员可在 `wecom.superAdminIds` 配置企微 userid 白名单，同步后自动成为管理员
- 未同步到本地的员工首次扫码会自动补拉通讯录后建号；`autoCreateUser: false` 时需管理员先开通

## 使用流程

1. 管理员登录 → **数据源** → 新建并测试连接
2. **数据集** → 编写 `SELECT ... WHERE d >= {{from_date}}`，声明参数（类型/默认值），保存后自动推断列
3. **图表** → 选择数据集、维度与指标，实时预览后保存
4. **仪表盘** → 新建 → 添加组件 → 拖动/缩放 → 添加筛选器并在「联动设置」中绑定参数名 → 保存
5. **分享与授权** → 生成公开链接或给指定用户授权
6. **定时报表** → 选择仪表盘、cron 表达式（支持秒级）、收件人；可立即执行并下载

### SQL 参数说明

- 数据集 SQL 中使用 `{{param_name}}`，执行时替换为驱动占位符（`?` 或 `$n`），值通过参数绑定传入，**绝不拼接字符串**
- 参数名仅允许小写字母/数字/下划线；SQL 中引用的参数必须先在数据集中声明
- 日期区间筛选器会分别提供开始（`valueIndex=0`）和结束（`valueIndex=1`）两个绑定值

### Cron 示例

`0 0 9 * * *` 每天 9:00 · `0 */30 * * * *` 每 30 分钟 · `0 0 9 * * 1` 每周一 9:00

## 目录结构

```
cmd/server/           入口、装配、集成测试
internal/config       配置（YAML + BI_* 环境变量）
internal/model        GORM 模型
internal/datasource   外部数据库连接池与 DSN
internal/query        只读校验、参数绑定、聚合 SQL、执行器、缓存
internal/service      业务逻辑
internal/handler      HTTP 处理器
internal/scheduler    cron 调度与 SMTP
internal/export       Excel 导出
internal/webui        前端构建产物（embed）
web/                  Vue 3 前端
data/                 运行时数据（元数据库、示例库、报表）
```

## 测试

```bash
go test ./...
cd web && npx vue-tsc --noEmit
```

`cmd/server/app_test.go` 使用临时 SQLite 元数据库与数据源执行端到端 API 测试，覆盖登录/角色/限流、只读拦截、参数校验、聚合、筛选联动、导出、分享撤销等。

演示数据脚本：`python scripts/gen_demo_data.py`（生成库）+ `python scripts/seed_demo.py`（灌入演示内容）。

## 设计取舍与限制（v1）

- 数据源与数据集管理仅管理员；数据集/图表是内部共享工作区（未做数据集级授权）
- 数据集/图表为内部共享工作区；看板支持个人授权、部门授权与公开链接三种下发方式
- 参数支持标量、日期区间与多选 `list`；暂不支持数据集建模/联表 UI
- 元数据表使用 GORM AutoMigrate，生产长期升级前建议切换版本化迁移
- JWT 单令牌（默认 12 小时），无 refresh token；分享链接承担长期只读访问
- 仪表盘/图表仅导出 Excel；单图 PNG 可在前端获取，仪表盘 PNG/PDF 延后
- 结果缓存为进程内 LRU（默认 60 秒），多实例部署需外接 Redis（预留接口）
- 数据源主机由管理员配置，属于受信边界（可访问内网地址）
