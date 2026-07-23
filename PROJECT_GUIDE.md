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

程序第一次创建管理员时会在终端输出用户名和随机密码，请立即保存。

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

## 八、Docker

Docker 是可选项，不影响传统二进制部署。相关文件：

- `Dockerfile`：构建镜像。
- `docker-compose.yaml`：SQLite 或外部数据库部署。
- `docker-compose.mysql.yaml`：额外启动 MySQL 8.4。
- `env.sqlite.example`、`env.mysql.example`：程序配置模板。
- `.env.docker.example`：Compose 端口和 MySQL 容器密码。
- `DEPLOYMENT.md`：完整 Docker 操作说明。

## 九、新 Git 仓库第一次提交

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
