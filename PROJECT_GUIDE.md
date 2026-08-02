# Epusdt 项目结构与运行导览

这份说明面向第一次阅读 Go 项目的人，按一次请求从进入程序到写入数据库的顺序介绍。

## 一、程序怎么启动

调用链如下：

```text
src/main.go
  → command.Execute()
  → command/http_server.go
  → bootstrap.InitApp()
  → config.Init()       读取 .env
  → dao.Init()          连接主数据库和 runtime SQLite
  → route.RegisterRoute()
  → 启动 HTTP、任务、通知和区块链监听
```

主要文件：

- `src/main.go`：程序入口，并把内嵌的管理后台页面释放到二进制旁边的 `www/`。
- `src/command/root.go`：命令行入口和 `--config` 参数。
- `src/command/http_server.go`：`http start` 命令、Echo HTTP 服务和静态页面。
- `src/bootstrap/bootstrap.go`：按顺序初始化配置、日志、数据库、API Key、队列和监听任务。
- `src/config/config.go`：读取 `.env`，包含公开 URL、日志、数据库和汇率 API 配置。

## 二、API 在哪里

所有公开路由先看 `src/route/router.go`：

- `/payments/gmpay/v1/order/create-transaction`：新版创建订单 API。
- `/payments/gmpay/v1/config`：公开的网络和币种配置。
- `/payments/epay/v1/...`：易支付兼容接口。
- `/pay/...`：收银台、订单状态和网络切换。
- `/admin/api/v1/...`：管理后台 API。

一笔 GMPay 创建订单请求的代码顺序：

```text
route/router.go
  → middleware/check_sign.go       验证 PID + Secret 签名
  → controller/comm/order_controller.go
  → model/request/order_request.go 请求字段
  → model/service/order_service.go 业务和汇率计算
  → model/data/order_data.go       数据读写
  → model/mdb/orders_mdb.go        orders 表结构
  → model/response/order_response.go 返回字段
```

完整参数、签名算法和返回格式见 `wiki/API.md`。

## 三、数据库代码在哪里

数据库分为三层：

1. `src/model/mdb/`：表结构。一个 Go struct 通常对应一张表。
2. `src/model/data/`：查询、增加、修改、删除等数据操作。
3. `src/model/dao/`：数据库连接、驱动和自动建表。

重点文件：

- `model/dao/mdb.go`：根据 `db_type` 选择 MySQL、PostgreSQL 或 SQLite。
- `model/dao/mdb_mysql.go`：MySQL 连接。
- `model/dao/mdb_postgres.go`：PostgreSQL 连接。
- `model/dao/mdb_sqlite.go`：SQLite 连接。
- `model/dao/runtime_sqlite.go`：队列和临时锁使用的 runtime SQLite。
- `model/dao/mdb_table_init.go`：自动建表并写入默认链、币种、RPC 和设置。
- `model/dao/legacy_schema.go`：本仓库增加的 v0.0.x → v1 数据迁移。
- `model/mdb/orders_mdb.go`：订单表。
- `model/mdb/wallet_address_mdb.go`：钱包地址表。
- `model/mdb/settings.go`：后台设置表。

新版不再依赖 Redis。即使主数据库使用 MySQL/PostgreSQL，运行时队列和锁仍使用本地 `runtime_sqlite_filename`。

## 四、手动汇率在哪里

手动汇率保存在主数据库 `settings` 表，不在 `.env` 里硬编码：

- `src/model/mdb/settings.go`：汇率设置 key 和默认值。
- `src/model/data/settings_data.go`：读取和保存设置。
- `src/controller/admin/settings_controller.go`：管理后台保存汇率的 API。
- `src/config/settings_bridge.go`：让 config 层读取数据库设置。
- `src/config/config.go`：
  - `GetRateForCoin()`：取得某个币种汇率。
  - `getForcedRateForCoin()`：优先读取手动汇率。
  - `GetUsdtRate()`：旧 USDT/CNY 兼容入口。
- `src/model/service/order_service.go`：创建订单时实际调用汇率。

固定 `1 USDT = 7.20 CNY` 时，后台 JSON 应填写：

```json
{
  "cny": {
    "usdt": 0.1388888888888889
  }
}
```

原因是系统保存的是“1 CNY 等于多少 USDT”，也就是 `1 / 7.20`。

`src/www/` 是已经编译好的管理后台前端，不是易读的前端源代码。查看后端汇率逻辑时优先看上面列出的 Go 文件。

## 五、URL 在哪里修改

运行时修改 `.env`：

```dotenv
app_uri=https://pay.example.com
http_listen=:8000
```

- `app_uri`：用户最终访问的公开地址。
- `http_listen`：程序监听地址。
- `src/config/config.go` 的 `GetAppUri()`：读取公开地址。
- `src/model/service/order_service.go`：用 `app_uri` 生成 `payment_url`。

建议使用独立子域名，不建议直接把新版管理后台放在 `/epusdt` 子路径下，因为编译后的前端包含根路径 API。

## 六、本机传统方式运行

当前项目要求 Go 1.25 或更高。安装后：

```bash
cd src
cp .env /tmp/epusdt-v0.env.backup
cp ../env.sqlite.example .env
```

本地测试把 `.env` 改为：

```dotenv
app_uri=http://localhost:8888
http_listen=:8888
```

构建和运行：

```bash
make build BUILD_TAG=v1.0.10-custom
./bin/epusdt --config .env http start
```

程序会自动创建默认管理员记录，但后台不接受密码登录。首次使用前必须在数据库的 `admin_users` 表中写入登录名和标准 Base32 TOTP 密钥。

## 七、传统 Linux 服务器部署

在 Mac 上打包 Linux AMD64：

```bash
cd src
make linux-amd64 BUILD_TAG=v1.0.10-custom
```

上传这两个文件：

```text
src/bin/epusdt-linux-amd64
服务器使用的 .env
```

服务器执行：

```bash
chmod +x epusdt-linux-amd64
./epusdt-linux-amd64 --config .env http start
```

新版网页已经嵌入二进制，启动时会在二进制旁边生成 `www/`，不用单独上传前端文件。长期运行建议再配置 systemd，参考 `wiki/manual_RUN.md` 或仓库根目录的 `epctl`。

### 从旧服务器版本升级

不需要把本机数据库同步到服务器。新二进制首次连接服务器原数据库时会自动执行兼容迁移并创建后台认证表。推荐按以下顺序操作：

1. 在服务器确认架构：`uname -m`；`x86_64` 使用 `epusdt-linux-amd64`，`aarch64` 使用 `epusdt-linux-arm64`。
2. 停止旧进程，备份服务器 `.env`、主数据库和 runtime SQLite 文件；不要用本机测试数据库覆盖服务器业务数据。
3. 在服务器 `.env` 中确认 `app_uri=https://支付域名`。若后台使用独立域名，则设置 `admin_webauthn_origins=https://后台域名`、`admin_webauthn_rp_id=后台域名`；RP ID 不含协议、端口或路径。
4. 把新文件上传为临时文件，赋予执行权限，再原子替换旧二进制；保留旧二进制用于回滚。
5. 启动新程序并检查日志。迁移成功后应出现 `admin_passkeys`、`admin_auth_challenges`、`admin_login_throttles`，`admin_users` 应增加 `totp_secret` 和 `auth_version`。
6. 在服务器数据库中直接写入管理员登录名和 Base32 TOTP 密钥，然后用生产域名测试动态口令登录并重新注册通行密钥。

本机 `localhost` 注册的通行密钥与 localhost 的 RP ID 绑定，不能复制到生产域名使用。若启动迁移失败，应立即停止新程序、恢复旧二进制及数据库备份，不要在迁移失败状态下继续接收订单。

## 八、Docker

Docker 是可选项，不影响传统二进制部署。相关文件：

- `Dockerfile`：构建镜像。
- `docker-compose.yaml`：SQLite 或外部数据库部署。
- `docker-compose.mysql.yaml`：额外启动 MySQL 8.4。
- `env.sqlite.example`、`env.mysql.example`：程序配置模板。
- `.env.docker.example`：Compose 端口和 MySQL 容器密码。
- `DEPLOYMENT.md`：完整 Docker 操作说明。

## 九、后台登录安全

- 后台不接受账号密码登录，只支持“登录名 + 6 位 TOTP 动态口令”或 WebAuthn 通行密钥。
- 登录名和 TOTP 密钥只能直接写入数据库；后台不提供生成、显示、启用、关闭或更换 TOTP 密钥的接口。
- `admin_users.totp_secret` 为空时，该账号不能使用动态口令登录，也不能绑定通行密钥。
- 打开 `/admin-security.html`，输入登录名和有效动态口令即可绑定通行密钥，不需要密码或已有登录会话。
- 错误次数按账号和客户端 IP 持久化记录：30 分钟窗口内第 5、10、15 次失败分别封禁 15 分钟、2 小时、24 小时，重启服务不能绕过。
- 退出登录或删除通行密钥会使旧 JWT 立即失效。
- 后台只接受直接写入数据库的标准 Base32 TOTP 密钥，不再生成或保留初始化密码及旧版认证加密密钥。
- 通行密钥在非 localhost 环境必须使用 HTTPS。`admin_webauthn_origins` 应填写实际后台 Origin；反向代理有多个合法入口时用英文逗号分隔。`admin_webauthn_rp_id` 通常留空，由 `app_uri` 的域名自动推导。
- 支付域名和后台域名分离时，`app_uri` 保持支付域名，WebAuthn Origin/RP ID 必须显式填写后台域名；本机或支付域名注册的通行密钥不能用于后台域名。
- 钱包地址一经创建不可原地修改；后台的 PATCH 钱包接口只能修改备注。更换地址应停用或删除旧地址，再新增并复核新地址。

例如，停服并备份数据库后可执行：

```sql
UPDATE admin_users
SET username = 'your-admin-name',
    totp_secret = 'YOUR_BASE32_TOTP_SECRET',
    auth_version = auth_version + 1
WHERE id = 1;
```

请严格限制数据库文件和数据库账号权限；直接保存的 Base32 TOTP 密钥等同于登录凭据。写入后先用认证器生成动态口令测试登录，再绑定至少一把通行密钥。RPC 节点会影响人工交易核验，生产环境应配置两个以上不同运营方的节点交叉验证。

## 十、新 Git 仓库第一次提交

确认敏感文件没有出现：

```bash
git status --short
```

然后：

```bash
git add .
git commit -m "chore: initialize customized epusdt v1.0.10"
git push -u origin main
```

不要提交 `src/.env`、根目录 `env`、数据库文件、日志和编译后的二进制；这些路径已经加入 `.gitignore`。
