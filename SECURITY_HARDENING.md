# Epusdt receiving-address hardening

## What this build protects

- `payment_wallet_allowlist` pins the only receiving addresses that may be used
  by a deployment. Once it is non-empty, missing networks and addresses fail
  closed.
- Wallet rows outside the allowlist are excluded from supported assets and
  order allocation, cannot be added through the admin API, and cannot be
  enabled.
- Existing order rows are checked again when loaded and before checkout data is
  returned. A direct database rewrite of `orders.receive_address` therefore
  returns an error instead of showing an unknown address to the payer.
- TRON, Solana, EVM, TON, and Aptos addresses submitted through the admin API
  receive network-specific syntax validation.
- Telegram wallet-management commands default to disabled. Telegram payment
  notifications are independent and continue to work.
- The SPA is served directly from the read-only executable. The service no
  longer deletes and extracts `www/` at startup, allowing the installation
  directory and binary to be owned by root.

This does not make a fully compromised host trustworthy. Root access, a
modified executable plus modified root-owned config, or a compromised merchant
application can still alter payment behavior. Use the host controls below as a
second boundary.

## Required production configuration

First list every active address and verify it manually against the wallet you
control:

```sql
SELECT id, network, address, status, deleted_at
FROM wallet_address
ORDER BY network, id;
```

Then add the verified addresses to `.env`. The format is comma-separated
`network:address` entries:

```dotenv
payment_wallet_allowlist=tron:T_YOUR_VERIFIED_ADDRESS
telegram_wallet_management_enabled=false
```

Multiple addresses or networks are supported:

```dotenv
payment_wallet_allowlist=tron:T_ADDRESS_1,tron:T_ADDRESS_2,ethereum:0x_ADDRESS
```

## Merchant-selected wallet IDs

The GMPay and EPay order endpoints accept an optional signed `wallet_id`
parameter. It refers to the numeric ID shown in Admin Console -> Addresses; it
is not a receive address supplied by the merchant. When present, the gateway
uses only that row and rejects the order unless the row:

- belongs to the requested network;
- is enabled and not deleted in the admin console; and
- is present in `payment_wallet_allowlist`.

For GMPay JSON, include the numeric value in the normal signature input:

```json
{
  "network": "tron",
  "token": "usdt",
  "wallet_id": 2
}
```

Omitting `wallet_id` preserves automatic multi-wallet allocation. A placeholder
order cannot carry `wallet_id` before it has a network; pass `wallet_id` to
`/pay/switch-network` when completing that placeholder instead. Existing
concrete orders are never moved to a different wallet.

Do not start with an example or shortened address. Startup rejects malformed
entries. After the allowlist is enabled, a network omitted from it cannot
create an on-chain order.

Check historical order addresses before rollout as well:

```sql
SELECT network, receive_address, COUNT(*) AS orders_count
FROM orders
WHERE receive_address <> '' AND pay_provider IN ('', 'on_chain')
GROUP BY network, receive_address
ORDER BY network, orders_count DESC;
```

Include every legitimate historical address that may still be queried, manually
verified, or have a callback resent. An omitted historical on-chain address is
also treated as a possible database rewrite and returns error `10052`.

## Root-owned installation

The service account needs write access only to `runtime/`. A typical layout is:

```text
/opt/csreaper-epusdt/              root:root             0755
/opt/csreaper-epusdt/epusdt       root:csreaper-epusdt  0750
/opt/csreaper-epusdt/.env         root:csreaper-epusdt  0640
/opt/csreaper-epusdt/runtime/      csreaper-epusdt:csreaper-epusdt 0750
```

Apply permissions after replacing the executable:

```bash
sudo chown root:root /opt/csreaper-epusdt
sudo chmod 0755 /opt/csreaper-epusdt
sudo chown root:csreaper-epusdt /opt/csreaper-epusdt/epusdt /opt/csreaper-epusdt/.env
sudo chmod 0750 /opt/csreaper-epusdt/epusdt
sudo chmod 0640 /opt/csreaper-epusdt/.env
sudo chown -R csreaper-epusdt:csreaper-epusdt /opt/csreaper-epusdt/runtime
sudo chmod 0750 /opt/csreaper-epusdt/runtime
```

The old extracted `www/` directory is unused by this build. Remove it only
after the new binary has started successfully and both cashier and admin SPA
routes have been tested.

## Binary checksum gate

Create a root-owned checksum after installing a trusted binary:

```bash
cd /opt/csreaper-epusdt
sha256sum epusdt | sudo tee epusdt.sha256 >/dev/null
sudo chown root:root epusdt.sha256
sudo chmod 0644 epusdt.sha256
```

Add this line to the service `[Service]` section so systemd refuses to start a
binary that no longer matches:

```ini
ExecStartPre=/usr/bin/sha256sum --check /opt/csreaper-epusdt/epusdt.sha256
```

The checksum file must remain root-owned. Update it deliberately whenever the
binary is upgraded.

## Recommended systemd sandbox

Create a drop-in with `sudo systemctl edit csreaper-epusdt`:

```ini
[Service]
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
RestrictSUIDSGID=true
LockPersonality=true
CapabilityBoundingSet=
AmbientCapabilities=
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
ReadWritePaths=/opt/csreaper-epusdt/runtime
UMask=0077
ExecStartPre=/usr/bin/sha256sum --check /opt/csreaper-epusdt/epusdt.sha256
```

Then verify before leaving the maintenance window:

```bash
sudo systemctl daemon-reload
sudo systemctl restart csreaper-epusdt
sudo systemctl status csreaper-epusdt --no-pager
sudo journalctl -u csreaper-epusdt -n 100 --no-pager
curl -fsS http://127.0.0.1:8176/payments/gmpay/v1/config
```

If systemd reports a denied filesystem path, inspect the journal and add only
that exact required runtime path to `ReadWritePaths`; do not make the whole
installation directory writable.

## Database and network boundary

- Keep MySQL bound to loopback/private interfaces and block port 3306 at the
  cloud firewall.
- Give the application user access only to its own schema. Do not grant global
  privileges, `FILE`, `PROCESS`, `SUPER`, `CREATE USER`, or `GRANT OPTION`.
- Keep `http_listen=127.0.0.1:8176`; expose the service only through the two
  reviewed Nginx virtual hosts.
- Keep the payment-domain Nginx configuration on a positive allowlist of public
  payment routes. Do not expose `/admin`, `/admin/api`, `/sign-in`, or SPA
  fallback on the payment domain.
- Back up MySQL and `runtime/` before upgrades, but never distribute the real
  `.env` with a binary.

## Release verification

Build from a clean, reviewed commit, publish a SHA-256 checksum separately, and
record all three values for each deployment: Git commit, build version, and
binary checksum. `go mod verify`, `go test ./...`, and `go vet ./...` should pass
before release.

After rollout, create one minimal-value real order and compare the API
`receive_address`, cashier QR/text, database `orders.receive_address`, and the
wallet you control. If the service logs a `[security]` allowlist violation,
stop accepting orders and investigate rather than adding the unknown address
to the allowlist.
