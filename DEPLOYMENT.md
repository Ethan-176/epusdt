# Epusdt v1.0.10 自定义 Docker 部署

本仓库已升级到官方稳定版 `v1.0.10`。运行配置与镜像分离，数据库、公开 URL、端口和汇率都可以自行管理。

## 1. 选择数据库

全新或低并发部署推荐 SQLite：

```bash
cp env.sqlite.example env
cp .env.docker.example .env
docker compose up -d --build
```

使用 Compose 内置 MySQL：

```bash
cp env.mysql.example env
cp .env.docker.example .env
```

把两个文件中的数据库密码改成相同值，再启动：

```bash
docker compose -f docker-compose.yaml -f docker-compose.mysql.yaml up -d --build
```

使用已有的外部 MySQL 时只启动主 compose，并在 `env` 中填写外部数据库地址。容器中的 `127.0.0.1` 指容器自身；macOS/Windows 上访问宿主机可用 `host.docker.internal`，Linux 推荐使用数据库的局域网地址或 Docker 网络服务名。

首次连接旧 v0.0.x 数据库前必须先备份。程序会在创建 v1 索引前自动迁移旧钱包和订单字段：

- `wallet_address.token` → `wallet_address.address`
- 钱包网络默认迁移为 `tron`
- `description` → `remark`
- 旧订单中的钱包地址从 `orders.token` 移到 `receive_address`
- 旧订单币种和网络分别标记为 `USDT`、`tron`

## 2. 自定义 URL 与端口

修改 `env`：

```dotenv
app_uri=https://pay.example.com
http_listen=:8000
```

修改根目录 `.env` 可以改变宿主机端口：

```dotenv
EPUSDT_PORT=8888
```

`app_uri` 必须是用户浏览器能够访问的外部地址，建议使用独立支付子域名。反向代理应把该域名完整转发到 `127.0.0.1:8888`，并传递 `Host`、`X-Forwarded-For` 和 `X-Forwarded-Proto`。

## 3. 手动汇率

启动后登录管理后台，进入“系统设置 → 支付/汇率”：

1. 在强制汇率 JSON 中填写自定义汇率。
2. 保存后立即写入数据库，无需重启容器。
3. 手动汇率优先于 `api_rate_url`，将对应币种设为正数即可避免使用外部汇率。

例如，固定 `1 USDT = 7.20 CNY`：

```json
{
  "cny": {
    "usdt": 0.1388888888888889
  }
}
```

这里存储的是“1 单位法币能购买多少币”，所以 `cny.usdt = 1 / 7.20`。

## 4. 常用命令

```bash
docker compose ps
docker compose logs -f epusdt
docker compose build --pull epusdt
docker compose up -d
docker compose down
```

不要使用 `docker compose down -v`，除非明确要删除 SQLite runtime 和 Compose MySQL 数据卷。

## 5. 独角数卡

创建订单地址改为：

```text
https://pay.example.com/payments/gmpay/v1/order/create-transaction
```

新版使用后台“API 密钥”页面生成的 `PID + Secret`。仓库中的旧版独角数卡插件已经同步更新了参数和签名方式。
