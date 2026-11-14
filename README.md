# 青衡 BI · 多层级组织数据权限分析平台

面向集团 / 大区 / 区域 / 门店多层级组织的 BI 看板平台。前后端分离、分层清晰、可二次开发，内置组织数据权限、SQL 数据集、看板与模板、带授权的分享链接、企业微信 / 微信公众号定时推送。

技术栈：

| 层 | 技术 |
|---|---|
| 前端 | Vue 3 + Vite + Element Plus + ECharts + Pinia + Vue Router |
| 后端 | Go + Gin + GORM |
| 平台库 | SQLite（默认，零依赖） / MySQL 8 |
| 业务数据源 | MySQL / PostgreSQL / Oracle / SQL Server / SQLite（后台可维护多个连接） |
| 调度 | robfig/cron v3 |
| 导出 | excelize |

---

## 1. 功能总览

- **多层级数据权限**：`all`（集团）/ `own_org_tree`（本组织及下级）/ `own_org`（仅本组织）三种数据范围，SQL 查询自动注入 `{{org_scope}}`，从数据库层收敛可见门店。
- **BI 看板**：KPI 卡、折线、柱状、饼/环图、明细表格；12 栅格布局；草稿 / 发布；复制；按权限范围实时查询。
- **个人工作台**：看板支持收藏、置顶与最近浏览记录，首页可按「默认 / 最近浏览 / 最近更新 / 访问最多 / 名称」排序并一键筛选「我的收藏」，偏好按账号持久化，多端登录一致。
- **分析体验**：组件表格 / 图表一键切换（默认 1 行数据用表格、多行数据用图表，可在编辑器里固定默认视图）、图表按指标排序与 Top N、整份看板导出 Excel（每个组件一个工作表）、单组件 CSV 导出、明细表排序 / 分页 / 当前表导出、看板与分享页全屏、本月 / 上月快捷区间；多组件并发查询。
- **指标趋势**：点击 KPI 数字或组件右上角趋势按钮，即可查看该指标最近 7 / 30 / 90 天或自定义区间的按日走势，含区间合计（比率型指标为区间均值）、日均、最低 / 峰值与数据来源；数据集自带日期列时按数据集聚合，否则按组织范围回退到销售事实，权限口径与看板一致。分享页同样可用。
- **全局筛选器**：单选 / 多选 / 日期 / 文本四类看板级筛选器，SQL 用 `{{filter:key}}`、`{{filter_in:key}}`、`{{filter_has:key}}` 注入；筛选值服务端白名单校验 + 字面量转义，支持「不在分享页显示」与 URL 持久化。
- **数字阅读**：数字统一使用无衬线等宽数字（tabular-nums），点击可在 `10,000` 与 `1万` 之间切换，全站组件一致。
- **组织经营下钻**：登录后按角色自动定位到自己的组织，默认展示下一级汇总；可逐级点击区域 / 门店继续下钻，也可切换「全部门店」平铺或「全部员工」合并视图，销售、目标与达成率随层级同步口径。
- **SQL 数据集**：管理员维护受控 SQL，前台通过参数占位符查询；只允许单条 `SELECT` / `WITH`。
- **数据源管理**：后台维护多个 MySQL / PostgreSQL / Oracle / SQL Server / SQLite 连接，DSN 使用 AES-GCM 加密存储，支持连接测试与只读标记。
- **经营目标**：按月 / 季 / 年维护组织目标，支持 Excel 模板导入导出，看板与下钻视图实时计算达成率。
- **模板中心**：沉淀看板结构，一键按当前用户权限生成新看板。
- **组织架构**：树形维护 + Excel 导入 / 导出；支持集团、大区、区域、门店四级。
- **用户与角色**：角色绑定数据范围与权限点；用户绑定组织；通讯录支持 Excel 模板导入与导出。
- **授权分享**：分享链接带数据范围快照、有效期、最大访问次数、撤销；公共页无需登录。
- **定时推送**：Cron 调度；企业微信文本 / 文本卡片 / 机器人 Webhook；微信公众号模板消息；推送模板样式后台可配；每个接收人按自身数据范围生成独立授权链接。
- **审计与记录**：后台自动记录写操作、登录（含失败）、导出与分享访问，包含账号、IP、状态码、耗时；分享 Token 在日志中脱敏；管理后台可按关键词、操作类型、状态与日期筛选并分页查看。

---

## 2. 目录结构

```text
data-web/
├─ README.md
├─ server/                         # Go 后端
│  ├─ cmd/api/main.go              # 程序入口
│  ├─ internal/
│  │  ├─ config/                   # 环境变量配置（支持 .env）
│  │  ├─ database/                 # GORM 初始化（SQLite / MySQL）
│  │  ├─ model/                    # 数据模型
│  │  ├─ repository/               # 数据访问层
│  │  ├─ service/                  # 业务逻辑层（权限、SQL 查询、推送…）
│  │  ├─ handler/                  # HTTP 接口层
│  │  ├─ router/                   # 路由与中间件装配、静态资源托管
│  │  ├─ middleware/               # JWT、权限、管理员校验
│  │  ├─ scheduler/                # Cron 定时任务
│  │  ├─ channel/                  # 企业微信 / 微信发送器
│  │  ├─ pkg/sqlguard/             # SQL 安全校验与参数注入
│  │  └─ seed/                     # 演示组织、用户、数据、模板
│  ├─ data/                        # SQLite 数据文件与运行日志
│  └─ .env.example
└─ web/                            # Vue 前端
   ├─ src/api/client.js
   ├─ src/stores/auth.js
   ├─ src/router/index.js
   ├─ src/layouts/AppLayout.vue
   ├─ src/components/              # 图表、KPI、表格、分享弹窗等
   └─ src/views/                   # 看板、模板、分享、权限、后台管理
```

后端严格按 `handler → service → repository → model` 分层，`service` 不依赖 Gin，便于单元测试与复用。

---

## 3. 快速启动

### 3.1 环境要求

- Go 1.26+
- Node.js 20+（推荐 22）
- 无需安装数据库，默认使用 SQLite

### 3.2 启动后端

```powershell
cd E:\workspace\biaoge\data-web\server
Copy-Item .env.example .env
go mod download
go run ./cmd/api
```

健康检查：<http://127.0.0.1:8486/health>

> `.env` 位于 `server/` 目录，程序启动时自动读取；系统环境变量优先级高于 `.env`。

### 3.3 启动前端（开发模式）

```powershell
cd E:\workspace\biaoge\data-web\web
npm install
npm run dev
```

访问 <http://localhost:5173>，`/api` 已代理到 `8486`。

### 3.4 单端口生产运行

前端构建后由 Gin 直接托管，只需启动一个进程：

```powershell
cd E:\workspace\biaoge\data-web\web
npm run build

cd ..\server
go build -o bin\api.exe ./cmd/api
.\bin\api.exe
```

访问 <http://localhost:8486> 即为完整站点（`WEB_DIST_DIR` 默认 `../web/dist`）。

### 3.5 局域网访问

服务默认监听全部网卡（`HOST` 留空 → `:8486`），同一局域网的电脑 / 手机 / 平板可直接访问；
只需要额外放行 Windows 防火墙端口（管理员 PowerShell 执行一次即可）：

```powershell
New-NetFirewallRule -DisplayName "Qingheng BI 8486" -Direction Inbound `
  -Protocol TCP -LocalPort 8486 -Action Allow -Profile Any
```

查看本机局域网地址后访问 `http://<本机IP>:8486`：

```powershell
Get-NetIPAddress -AddressFamily IPv4 |
  Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' } |
  Select-Object IPAddress, InterfaceAlias
```

说明：

- `HOST=` 留空 = 监听全部网卡；填 `127.0.0.1` 则只允许本机访问。
- `WEB_BASE_URL` 留空时，分享链接 / 企业微信回调按浏览器实际访问的 Host 自动生成，因此局域网访问生成的分享链接会自动带上局域网 IP 与 8486 端口。
- 若公司网络有多层 NAT 或终端安全软件，需要同步放行 TCP 8486。

---

## 4. 演示账号

| 用户名 | 密码 | 角色 | 数据范围 | 可见门店数 |
|---|---|---|---|---|
| admin | admin123 | 平台管理员 | 全部 | 15 |
| laozong | 123456 | 集团老总 | 全部 | 15 |
| huadong | 123456 | 华东大区经理 | 本组织及下级 | 9 |
| shanghai | 123456 | 上海区域经理 | 本组织及下级 | 3 |
| jiangsu | 123456 | 江苏区域经理 | 本组织及下级 | 3 |
| guangzhou | 123456 | 广州区域经理 | 本组织及下级 | 3 |
| store01 | 123456 | 静安旗舰店店长 | 仅本门店 | 1 |

角色权限点：

| 角色 | 权限点 |
|---|---|
| 平台管理员 | `*`（全部） |
| 集团老总 | `dashboard:view` `dashboard:create` `share:create` `push:view` |
| 大区 / 区域经理 | `dashboard:view` `dashboard:create` `share:create` |
| 店长 | `dashboard:view` `share:create` |
| 门店员工 | `dashboard:view` |

> 首次启动且 `SEED_DEMO_DATA=true` 时会自动写入演示数据；生产请改为 `false`。

---

## 5. 数据权限设计

### 5.1 解析规则

| 角色 `data_scope` | 可见组织 |
|---|---|
| `all` | 全部组织 |
| `own_org_tree` | 用户所属组织及其所有下级（大区 → 区域 → 门店） |
| `own_org` | 仅用户所属组织（门店店长） |

### 5.2 SQL 注入

数据集 SQL 使用占位符，服务端在执行前替换：

```sql
SELECT ROUND(SUM(revenue),2) AS revenue
FROM sales_facts
WHERE stat_date BETWEEN {{start_date}} AND {{end_date}}
  AND org_id IN ({{org_scope}})
```

| 占位符 | 说明 |
|---|---|
| `{{org_scope}}` | 当前用户可见组织 ID 列表，如 `1,2,3`；未授权时不可能返回越权数据 |
| `{{start_date}}` | 查询开始日期 |
| `{{end_date}}` | 查询结束日期 |

### 5.3 SQL 安全限制

- 仅允许单条 `SELECT` / `WITH` 查询；
- 禁止注释、多语句与 `INSERT/UPDATE/DELETE/DROP/ALTER/TRUNCATE/GRANT/CALL` 等危险关键字；
- 单次查询最多返回 2000 行，超时默认 10 秒；
- 日期跨度最长 366 天；
- 建议生产环境为数据集使用**只读数据库账号**。

### 5.4 权限校验位置

- 前端：路由守卫（`admin` / `permission` 元信息）+ 菜单显隐；
- 后端：JWT 中间件 + `RequireAdmin` / `RequirePermission`；
- 数据：`service.Query` 中注入 `{{org_scope}}`，看板、分享、推送统一走该链路。

### 5.5 组织经营下钻

前端入口：**分析工作台 → 组织下钻**（`/drill`）。接口：

```http
GET /api/v1/drill?period_type=month&period=2026-11&metric=revenue&mode=level&org_id=
```

| 参数 | 说明 |
|---|---|
| `org_id` | 可选，不传时自动定位当前用户所属组织；管理员默认定位根组织 |
| `period_type` | `month` / `quarter` / `year` |
| `period` | 如 `2026-11`、`2026-Q4`、`2026` |
| `metric` | `revenue` / `orders` / `gross_profit` / `customers` / `gross_margin` |
| `mode` | `level` 逐级下钻 / `all` 全部门店平铺 / `employees` 全部员工合并 |

角色行为示例：

- 大区经理进入后看到所辖区域（小区）的销售额、目标与达成率；点击区域进入该区域门店明细；
- 区域经理进入后直接看到本区域门店；点击门店进入该门店员工销售明细；
- 「全部门店」把当前权限范围内的所有门店跨层级平铺，并显示所属区域；
- 「全部员工」把当前范围内所有门店的员工销售合并展示。

页面同时提供月度 / 季度 / 年度切换、销售额 / 订单 / 毛利 / 客流 / 毛利率指标切换、销售与目标对比图、贡献排行和当前视图 CSV 导出。

目标口径：节点优先使用自身目标；未单独设置时自动向下汇总子组织目标，避免门店级目标导入后区域层级达成为空。

每一层级的组织集合都会与当前用户数据范围求交，越权节点不会出现在返回结果中。

### 5.6 看板全局筛选器

编辑器左侧「看板设置 → 全局筛选器」可为看板添加最多 8 个筛选器；阅读页与分享页顶部渲染筛选栏，取值通过 SQL 占位符注入数据集查询。

| 类型 | 阅读页控件 | SQL 占位符 |
|---|---|---|
| `select` 单选 | 下拉（可搜索、可清空） | `{{filter:key}}` |
| `multiselect` 多选 | 多选下拉 | `{{filter_in:key}}` + `{{filter_has:key}}` |
| `date` 日期 | 日期选择器 | `{{filter:key}}` |
| `input` 文本 | 文本输入框 | `{{filter:key}}` |

单选 / 日期写法（空值表示「全部」）：

```sql
AND ({{filter:channel}} = '' OR channel = {{filter:channel}})
```

多选写法（`{{filter_in:key}}` 展开为 `'a','b'`；`{{filter_has:key}}` 未选择时为 `0`，已选择时为 `1`）：

```sql
AND ({{filter_has:channel}} = 0 OR channel IN ({{filter_in:channel}}))
```

注意：不要写成 `{{filter_in:key}} = ''`，多选时该占位符会展开成 `'a','b'`，比较会报 SQL 语法错误，判断「是否已选择」请统一用 `{{filter_has:key}}`。

安全与运行约束：

- 筛选值只由服务端 `quoteLiteral` 转义成字符串字面量，前端不参与 SQL 拼接；
- 单值长度 ≤ 200 字符，多选最多 50 个值；
- 服务端按看板定义做白名单校验：未定义的字段名、不在 `options` 里的取值会被丢弃，防止越权取值探测；
- 勾选「不在分享页显示」后，分享页不渲染该筛选器，服务端同时忽略分享请求里传入的该字段；
- 当前筛选写入 URL `?filters=...`，刷新、前进 / 后退、转发链接都能保持筛选状态；
- 查询缓存按「数据集 + 日期 + 组织范围 + 筛选值」隔离，切换筛选不会命中旧缓存（`nocache=1` 可强制刷新）。

组件还可以在编辑器里配置「点击联动筛选器」：图表中点击某个分类（例如某个渠道 / 区域）时，自动回填对应筛选器并重新查询。

---

## 6. 分享链接（授权访问）

1. 用户在看板详情页生成分享链接；
2. 后端保存 **SHA256 Token 哈希**、创建人、看板、组织范围快照、过期时间、最大访问次数；
3. 明文 Token 只在创建响应中返回一次，列表接口不返回；
4. 访问 `/share/:token` 时校验：存在、未撤销、未过期、未超次数；通过后按快照范围查询数据；
5. 过期 / 撤销 / 超次均返回 403。

默认有效期 24 小时，可在创建时指定，最长 365 天；`max_views=0` 表示不限次数。

---

## 7. 定时推送

### 7.1 支持通道

| 通道 | 类型 | 说明 |
|---|---|---|
| 企业微信 | `wecom` + `wecom_text` | 应用消息纯文本 |
| 企业微信 | `wecom` + `wecom_textcard` | 应用消息文本卡片，可点击跳转 |
| 企业微信 | `wecom` + Webhook | 群机器人 Markdown |
| 微信公众号 | `wechat` | 模板消息，可点击跳转 |

### 7.2 后台配置

管理后台 → 推送中心：

1. **通道配置**：企业微信 `corp_id`、`agent_id`、`secret`，或 `webhook_url`；微信公众号 `app_id`、`secret`、`template_id`；
2. **推送模板**：标题 / 正文模板、通道类型、`style_json`（按钮文案、主色）；
3. **推送任务**：选择看板、Cron、时区、通道、接收人（按用户 / 角色 / 组织）、模板；
4. 支持“立即发送”与预览。

`secret` 回传时以 `••••••••` 掩码显示，保存时后端自动保留旧值，不会因前端留空而丢失。

### 7.3 模板变量

```text
{{.DashboardName}}  {{.RecipientName}}  {{.Period}}
{{.Revenue}}  {{.Orders}}  {{.GrossProfit}}  {{.Customers}}  {{.URL}}
```

### 7.4 Cron 示例

| 表达式 | 含义 |
|---|---|
| `0 9 * * *` | 每天 09:00 |
| `0 9 * * 1` | 每周一 09:00 |
| `30 18 * * 1-5` | 工作日 18:30 |
| `0 8 1 * *` | 每月 1 日 08:00 |

### 7.5 授权链路

每个接收人按自己的组织范围查询销售数据，并生成**独立的授权分享链接**（7 天有效），卡片点击后进入 `/share/:token`，仍受组织范围与链接生命周期约束。接收人没有企业微信 UserID / 微信 OpenID 时记录为 `skipped`。

未配置真实凭证且 `ALLOW_MOCK_PUSH=true` 时记录为 `simulated`，不会调用外部接口，便于联调。

### 7.6 企业微信应用配置要点

- 企业微信后台创建自建应用，取得 `CorpID`、`AgentID`、`Secret`；
- 应用可见范围需包含接收人；
- 服务器出口 IP 加入企业微信可信 IP（如企业微信要求）；
- 接收人的 `wecom_user_id` 为用户账号（UserID），不是姓名。

---

## 8. 企业微信组织同步与授权登录

### 8.1 企业微信后台准备

1. 企业微信管理后台 → 应用管理 → 自建 → 创建应用，记录 `CorpID`（我的企业 → 企业信息）、`AgentID` 与应用 `Secret`。
2. 在「应用 → 网页授权及 JS-SDK」中配置可信域名；使用浏览器扫码登录时，还需在「企业微信授权登录」中配置授权回调域。
3. 为应用开启通讯录读取权限（部门列表、成员列表），否则同步会返回 `60011`。
4. 平台侧回调地址固定为：`https://你的域名/api/v1/auth/wecom/callback`。

### 8.2 平台配置

管理员登录后进入「组织架构 → 企业微信同步」：

| 配置项 | 说明 |
| --- | --- |
| CorpID | 企业微信企业 ID |
| AgentID | 自建应用 AgentID，扫码登录必填 |
| 应用 Secret | AES-GCM 加密存储，接口只返回掩码；留空表示不修改 |
| 启用组织同步 | 开启后才允许发起同步任务 |
| 同步成员 | 按企微 UserID 创建 / 更新平台账号 |
| 默认角色 | 同步新建成员的初始角色，留空使用权限最低的角色 |
| 组织编码前缀 | 默认 `WECOM_DEPT_`，用于生成唯一组织编码 |
| 层级映射 | 部门层级 → 集团 / 大区 / 区域 / 门店 |
| 启用企业微信登录 | 登录页展示企业微信登录入口 |
| 登录方式 | `auto` 企微内静默 / 浏览器扫码；`oauth` 仅静默；`qr` 仅扫码 |
| 企微内自动登录 | 企业微信客户端打开登录页时自动授权 |

保存后点击「测试连接」，可验证凭证并查看部门与成员数量。

### 8.3 异步同步与进度

- 同步任务在后台 Goroutine 执行，接口立即返回任务快照。
- 前端每 1.5 秒轮询 `/api/v1/admin/wecom/jobs/:id`，展示 `processed / total`、部门数以及新增 / 更新 / 跳过 / 失败明细。
- 同一时间只允许一个同步任务，重复发起返回 `409`。
- 服务重启时残留的 `pending / running` 任务会被标记为 `failed`。
- 部门按 `external_source=wecom + external_id` 幂等更新，不覆盖手工维护的组织编码。
- 成员按 `WecomUserID` 匹配；历史账号用户名恰好等于企微 UserID 时自动建立绑定。
- 已离职成员跳过；状态异常的成员同步为停用。
- 可选择「停用企业微信中已移除的部门」，仅在本次任务无失败时执行。

### 8.4 登录链路

企业微信客户端内：

```text
企业微信内置浏览器
→ /api/v1/auth/wecom/authorize?mode=auto
→ 后端识别 UA=wxwork，返回 snsapi_base 静默授权地址
→ /api/v1/auth/wecom/callback 换取成员 UserID
→ 查找平台账号并签发 3 分钟一次性 ticket
→ 302 到前端 /wecom/callback?ticket=...
→ 前端 POST /api/v1/auth/wecom/exchange 换取正式 JWT
```

普通浏览器：

```text
登录页点击「企业微信扫码登录」
→ 后端返回 wwlogin/sso/login?login_type=CorpApp 扫码地址
→ 扫码授权后回调，后续流程与企微内一致
```

安全约束：

- 长期 JWT 不进入 URL，只使用一次性 ticket。
- `state` 使用 JWT 签名，10 分钟有效，登录与绑定用途分离。
- 回调跳转只允许站内相对路径，防止开放重定向。
- 未绑定平台账号的用户会提示联系管理员。

### 8.5 账号绑定

- 管理员可在「用户与角色」中直接填写企业微信 UserID。
- 普通用户可在「数据权限」页使用「绑定企业微信」，授权后自动写入绑定关系。
- 「解除绑定」后只能使用账号密码登录。

### 8.6 相关接口

```http
GET    /api/v1/auth/wecom/config
GET    /api/v1/auth/wecom/authorize?mode=auto&redirect=/dashboards
GET    /api/v1/auth/wecom/callback
POST   /api/v1/auth/wecom/exchange
GET    /api/v1/auth/wecom/bind-url?redirect=/access
DELETE /api/v1/auth/wecom/bind

GET    /api/v1/admin/wecom/config
PUT    /api/v1/admin/wecom/config
POST   /api/v1/admin/wecom/test
POST   /api/v1/admin/wecom/sync
GET    /api/v1/admin/wecom/jobs?limit=20
GET    /api/v1/admin/wecom/jobs/:id
```

## 9. 组织架构导入 / 导出

管理后台 → 组织架构 → 导出，得到 `.xlsx`，表头：

| 类型 | 名称 | 编码 | 上级编码 | 负责人 | 电话 | 地址 | 状态 |
|---|---|---|---|---|---|---|---|
| company | 众智零售集团 | HQ | | 陈总 | | | active |
| big_region | 华东大区 | BR_EAST | HQ | 周明 | | | active |
| region | 上海区域 | RG_SH | BR_EAST | 李娜 | | | active |
| store | 静安旗舰店 | ST_SH_JA | RG_SH | 店长01 | 13800000000 | 上海市… | active |

规则：

- 按 `编码` 幂等更新 / 新增；
- `上级编码` 为空表示根节点；
- 类型支持中文别名（公司 / 集团 / 大区 / 区域 / 门店）或英文枚举；
- 导入会自动计算层级 `level`；
- 导出的文件可直接修改后再次导入。

---

## 10. 配置项

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8486` | 服务端口 |
| `DB_DRIVER` | `sqlite` | `sqlite` / `mysql` |
| `DB_DSN` | `data/bi.db` | SQLite 路径或 MySQL DSN |
| `JWT_SECRET` | 演示值 | **生产必须更换**，≥16 位 |
| `JWT_EXPIRE_HOURS` | `24` | 登录有效期 |
| `API_BASE_URL` | `http://localhost:8486` | 后端地址 |
| `WEB_BASE_URL` | `http://localhost:5173` | 生成分享链接的前端地址 |
| `CORS_ORIGINS` | `http://localhost:5173,...` | 允许跨域来源，逗号分隔 |
| `APP_TIMEZONE` | `Asia/Shanghai` | 应用时区 |
| `AUTO_MIGRATE` | `true` | 自动迁移表结构 |
| `SEED_DEMO_DATA` | `true` | 写入演示数据 |
| `ALLOW_MOCK_PUSH` | `true` | 无凭证时模拟推送 |
| `SQL_QUERY_TIMEOUT_SECONDS` | `10` | 数据集查询超时 |
| `WEB_DIST_DIR` | `../web/dist` | 前端构建产物目录 |

### MySQL 切换

```env
DB_DRIVER=mysql
DB_DSN=bi:bi_password@tcp(127.0.0.1:3306)/bi_platform?charset=utf8mb4&parseTime=True&loc=Local
```

建库后启动即自动迁移。

---

## 11. 主要接口

统一响应：

```json
{ "code": 0, "message": "ok", "data": {} }
```

鉴权头：`Authorization: Bearer <token>`

### 认证与组织

```http
POST /api/v1/auth/login
GET  /api/v1/auth/me
GET  /api/v1/orgs/tree
GET  /api/v1/roles
GET  /api/v1/drill?period_type=&period=&metric=&mode=&org_id=
```

### 用户（管理员）

```http
GET    /api/v1/users
POST   /api/v1/users
PUT    /api/v1/users/:id
DELETE /api/v1/users/:id
GET    /api/v1/users/export
GET    /api/v1/users/template
POST   /api/v1/users/import
```

### 数据集

```http
GET    /api/v1/datasets
GET    /api/v1/datasets/:id/preview
POST   /api/v1/datasets        # 管理员
PUT    /api/v1/datasets/:id    # 管理员
DELETE /api/v1/datasets/:id    # 管理员
POST   /api/v1/datasets/:id/to-dashboard   # 管理员，一键生成看板
```

### 经营目标

```http
GET    /api/v1/targets
GET    /api/v1/targets/achievement
GET    /api/v1/targets/periods
GET    /api/v1/targets/metrics
POST   /api/v1/targets          # 管理员
PUT    /api/v1/targets/:id      # 管理员
DELETE /api/v1/targets/:id      # 管理员
GET    /api/v1/targets/export   # 管理员
GET    /api/v1/targets/template # 管理员
POST   /api/v1/targets/import   # 管理员
```

### 数据源

```http
GET    /api/v1/admin/datasources
POST   /api/v1/admin/datasources
PUT    /api/v1/admin/datasources/:id
DELETE /api/v1/admin/datasources/:id
POST   /api/v1/admin/datasources/:id/test
```

支持的驱动：`mysql` / `postgres` / `oracle` / `sqlserver` / `sqlite`。

Oracle 连接串示例（基于 go-ora）：

```text
oracle://user:pass@127.0.0.1:1521/ORCLPDB1
oracle://user:pass@127.0.0.1:1521/?service_name=ORCLPDB1
```

查询限制会按数据源方言自动改写，无需在 SQL 中手写分页：

- MySQL / PostgreSQL / SQLite：`SELECT * FROM (...) AS bi_query LIMIT 2000`
- SQL Server：`SELECT TOP 2000 * FROM (...) AS bi_query`
- Oracle：`SELECT * FROM (...) bi_query WHERE ROWNUM <= 2000`

Oracle 注意事项：

- 日期占位符渲染为字符串字面量，比较 DATE / TIMESTAMP 字段时请显式转换：
  `WHERE sale_date >= TO_DATE('{{start_date}}', 'YYYY-MM-DD')`。
- `DBMS_*`、`UTL_*`、`OWA_*`、`XDB_*` 等系统包调用会被 SQL 安全策略拒绝。
- `NUMBER` 列会按列类型自动还原为数值，`CLOB` / `LONG` 列自动展开为文本。

### 看板

```http
GET    /api/v1/dashboards
GET    /api/v1/dashboards/:id
GET    /api/v1/dashboards/:id/data?start_date=&end_date=
GET    /api/v1/dashboards/:id/widgets/:widgetId/trend?field=&group_field=&group_value=&start_date=&end_date=&filters=
GET    /api/v1/dashboards/:id/export?format=xlsx&start_date=&end_date=
GET    /api/v1/dashboards/:id/export?format=csv&widget_id=&start_date=&end_date=
POST   /api/v1/dashboards
PUT    /api/v1/dashboards/:id
DELETE /api/v1/dashboards/:id
POST   /api/v1/dashboards/:id/duplicate
```

### 模板

```http
GET  /api/v1/templates
POST /api/v1/templates       # 管理员
DELETE /api/v1/templates/:id # 管理员，内置模板不可删除
POST /api/v1/templates/:id/use
```

### 分享

```http
POST   /api/v1/shares
GET    /api/v1/shares
DELETE /api/v1/shares/:id
GET    /api/v1/public/dashboards/:token   # 公开
GET    /api/v1/public/dashboards/:token/widgets/:widgetId/trend   # 公开，数据范围取自分享令牌
```

### 后台管理

```http
GET  /api/v1/admin/orgs/export
POST /api/v1/admin/orgs/import
POST /api/v1/admin/orgs
PUT  /api/v1/admin/orgs/:id
DELETE /api/v1/admin/orgs/:id

GET    /api/v1/admin/push/templates
POST   /api/v1/admin/push/templates
PUT    /api/v1/admin/push/templates/:id
DELETE /api/v1/admin/push/templates/:id

GET    /api/v1/admin/push/subscriptions
POST   /api/v1/admin/push/subscriptions
PUT    /api/v1/admin/push/subscriptions/:id
DELETE /api/v1/admin/push/subscriptions/:id
POST   /api/v1/admin/push/subscriptions/:id/send

GET  /api/v1/admin/push/channels
PUT  /api/v1/admin/push/channels/:channel
GET  /api/v1/admin/push/records?limit=100
POST /api/v1/admin/push/records/:id/retry   # 重试失败记录所属任务
GET  /api/v1/admin/push/preview?template_id=&dashboard_id=
GET  /api/v1/admin/audit-logs?page=1&page_size=20&keyword=&action=&status=&start=&end=
```

`audit-logs` 返回 `{ items, total, page, page_size, stats, actions }`；`status` 支持
`success`（2xx）、`failed`（≥400）、`server`（≥500）或具体状态码。

---

## 12. 前端说明

- 自适应布局：桌面侧栏 + 移动端抽屉；看板 12 栅格在窄屏自动堆叠；
- 路由守卫：未登录跳 `/login`；`/admin/*` 仅管理员；`dashboard:create` 控制新建 / 编辑；
- 公共分享页 `/share/:token` 不加载登录态、不调用 `useAuthStore`；
- 图表统一由 `ChartRenderer` / `WidgetRenderer` 渲染，新增图表类型只需扩展组件映射；
- ECharts 通过 `src/utils/echarts.js` 按需注册，构建时拆分为 `vendor-echarts` / `vendor-element` 等独立缓存块；
- 审计日志页 `/admin/audit` 支持筛选、分页与明细抽屉，仅管理员可见；
- 样式集中在 `src/assets/styles.css`，使用 CSS 变量便于换肤。

构建：

```powershell
cd web
npm run build
```

产物输出到 `web/dist`。

---

## 13. 二次开发指引

### 新增一个图表类型

1. 后端：无需改动（数据结构为通用 `config`）；
2. 前端：在 `ChartRenderer.vue` 增加 `chart_type` 分支；
3. 编辑器：在 `DashboardEditorView.vue` 的图表类型选项中加入新类型。

### 新增数据范围类型

1. `service/scope.go` 的 `Resolve` 增加分支；
2. 角色表 `roles.data_scope` 使用新枚举；
3. 确保所有查询仍通过 `DatasetService.Query` 注入范围。

### 新增推送通道

1. `model.PushTemplate.Channel` 增加类型；
2. `channel/sender.go` 增加发送实现；
3. `PushService.templateForChannel` 与后台表单补充映射。

### 新增业务数据集

管理后台 → 数据模型 → 新建，SQL 中必须包含 `{{org_scope}}`（若 `require_scope=true`），并用「预览」验证。

---

## 14. 生产上线清单

- [ ] 更换 `JWT_SECRET`，长度 ≥ 32 位随机串；
- [ ] `SEED_DEMO_DATA=false`，删除演示账号或改密；
- [ ] `WEB_BASE_URL` 改为真实域名（HTTPS），分享链接依赖它；
- [ ] 配置 `CORS_ORIGINS` 为真实前端域名；
- [ ] 数据集使用只读数据库账号，禁止 DDL/DML 权限；
- [ ] 启用企业微信 / 微信通道，关闭 `dry_run`，确认 `ALLOW_MOCK_PUSH` 策略；
- [ ] 数据库定期备份（SQLite 备份 `data/bi.db*`；MySQL 使用物理/逻辑备份）；
- [ ] 前置 Nginx / 网关做 HTTPS、限流与访问日志；
- [ ] 关注 `server/data/server.err.log` 与推送记录失败数；
- [ ] 多实例部署时，Cron 任务需改用分布式锁或单独调度进程（当前为单实例内存调度）。

---

## 15. 常见问题

**Q：登录后接口 401？**
检查 `Authorization` 是否为 `Bearer <token>`，以及 `JWT_SECRET` 是否在重启后发生变化（旧 token 会失效）。

**Q：分享链接打不开？**
确认 `WEB_BASE_URL` 与前端实际访问域名一致；检查链接是否过期、被撤销或达到最大访问次数。

**Q：推送记录都是 simulated？**
未配置真实凭证或通道未启用 / `dry_run=true`，属于预期行为。关闭 `dry_run`、启用通道并配置凭证后即可真实发送。

**Q：店长能看到别的门店数据吗？**
不会。SQL 会注入 `org_id IN (当前门店)`；分享链接与推送链接同样带范围快照。

**Q：组织导出的文件再导入报错？**
使用系统导出的模板，保持表头包含「名称」「编码」；系统会遍历所有工作表查找正确表头。

**Q：如何清空演示数据？**
停止服务，删除 `server/data/bi.db`（及 `-wal` / `-shm`），将 `SEED_DEMO_DATA=false` 后重启。

---

## 16. 许可

内部项目模板，可按需修改与二次分发。
