# Agent Proxy 双链路设计与实施 Roadmap

状态：Draft  
设计日期：2026-07-28  
目标平台：macOS 客户端 + Debian 12 ECS  
目标客户端：Codex CLI 优先，其他 Agent 按兼容性逐步接入

## 1. 文档目的

本文档定义 Agent Proxy 从“公网 Squid HTTP Proxy”逐步演进到“SSH 动态 SOCKS5 主链路”的完整方案。

演进期间保留两条链路：

1. **主链路：SSH SOCKS5**
2. **兼容链路：Squid HTTP CONNECT**

运行策略：

```text
优先尝试 SSH SOCKS5
        ↓
SOCKS5 能力、隧道和目标连通性全部正常
        ├─ 是 → 使用 SOCKS5 启动目标客户端
        └─ 否 → 检查 Squid
                    ├─ 正常 → 使用 Squid 启动目标客户端
                    └─ 异常 → 拒绝启动并输出诊断
```

本方案强调：

- 不引入 WireGuard、Tailscale 或额外账号体系。
- 复用 macOS 自带的 OpenSSH 客户端。
- 复用 ECS 已运行的 OpenSSH Server。
- SOCKS5 链路不需要代理用户名和密码。
- Squid 暂时保留，用于不支持 SOCKS5 的客户端以及主链路故障时的兼容降级。
- 降级采用 fail-closed 原则：两条代理链路都不可用时，不允许静默直连。

## 2. 背景与现状

当前链路：

```text
Codex/Agent
    ↓ HTTP Proxy + Basic Auth
公网 ECS:18443
    ↓ Squid CONNECT
chatgpt.com / OpenAI / Anthropic
```

当前实现包括：

- ECS 上的 Squid。
- 公网 TCP 代理端口。
- Squid Basic 用户名密码认证。
- macOS PAC 文件和本地 PAC HTTP Server。
- CLI `http_proxy`、`https_proxy` 环境变量。
- macOS 系统自动代理配置。

现状的主要问题：

- Basic 代理凭证经未加密的客户端到代理 HTTP 链路发送。
- Squid 公网端口增加了暴露面。
- 代理密码以明文形式存在于本地环境文件和部署信息中。
- PAC 和 CLI 的分流语义不一致。
- 启停脚本需要修改 macOS 全局网络设置。
- Squid ACL、进程生命周期和失败传播仍需加固。

## 3. 目标与非目标

### 3.1 目标

本阶段必须实现：

1. Codex 默认通过本地 SSH SOCKS5 访问 ECS 出口。
2. SOCKS5 启动或健康检查失败时，启动器自动选择 Squid。
3. 明确支持“某客户端不支持 SOCKS5，固定使用 Squid”。
4. 两条链路并行保留，互不覆盖配置。
5. 不修改 macOS 全局代理即可使用 Codex。
6. 代理选择仅作用于被包装器启动的进程。
7. 所有状态和日志不得输出秘密。
8. 隧道支持安全启动、重复启动、状态检查和可控关闭。
9. Squid 链路完成最低限度安全加固。
10. 提供自动化测试、上线、观察和回滚流程。

### 3.2 非目标

本阶段不实现：

- WireGuard 或 Tailscale。
- 全系统 VPN。
- 对运行中 WebSocket 会话进行透明热切换。
- 完全通用的任意应用协议探测。
- 多 ECS 自动选路。
- 跨设备集中身份管理。
- 域名级透明系统路由。
- 自动卸载 Squid。

## 4. 关键设计结论

### 4.1 SOCKS5 由 `ssh -D` 提供

不在本地安装第三方 SOCKS5 Server，也不在 ECS 安装额外 SOCKS5 软件。

本地执行：

```bash
ssh -N -D 127.0.0.1:1080 user@ecs
```

OpenSSH 在 `127.0.0.1:1080` 提供 SOCKS5 服务，并通过现有 SSH 加密连接将请求转发到 ECS。

### 4.2 使用 `socks5h`，由 ECS 侧解析域名

主链路环境变量：

```text
ALL_PROXY=socks5h://127.0.0.1:1080
all_proxy=socks5h://127.0.0.1:1080
```

使用 `socks5h` 的原因：

- 域名通过 SOCKS 协议交给 ECS 解析。
- 避免本地 DNS 污染或解析失败。
- 使 DNS 解析位置与出口位置保持一致。
- 不向本地 DNS 泄漏代理目标域名。

### 4.3 降级发生在进程启动前

启动器在 `exec` 目标程序前选择一条链路。

不能保证运行中的 Codex 会话从 SOCKS5 无感切换到 Squid，原因包括：

- 已建立的 TCP 连接绑定到原有传输路径。
- WebSocket 是长连接。
- TLS、认证和应用会话状态无法迁移。
- 重放正在进行的模型请求可能产生重复副作用。

因此：

```text
启动前 SOCKS5 不可用 → 自动使用 Squid
运行中 SOCKS5 断开   → 当前请求失败，提示用户重新启动
重新启动             → 再次执行 SOCKS5 → Squid 选择
```

不进行运行中请求的自动重放。

### 4.4 不允许静默直连

当用户明确通过 Agent Proxy 启动客户端时：

- SOCKS5 失败且 Squid 成功：使用 Squid。
- SOCKS5 和 Squid 都失败：终止启动。
- 不删除代理变量后继续执行。
- 不以 DIRECT 作为最终兜底。

这样可以避免用户误以为流量经过 ECS，实际却从本地直连。

## 5. 总体架构

### 5.1 双链路架构

```text
                          ┌─────────────────────────┐
                          │ agent-proxy-run         │
                          │ 兼容性 + 健康检查 + 选路 │
                          └────────────┬────────────┘
                                       │
                    ┌──────────────────┴──────────────────┐
                    │                                     │
              SOCKS5 主链路                         Squid 兼容链路
                    │                                     │
      ALL_PROXY=socks5h://127.0.0.1:1080     HTTPS_PROXY=http://user:pass@ecs:18443
                    │                                     │
          OpenSSH DynamicForward                      Squid CONNECT
                    │                                     │
                    └─────────────── ECS ─────────────────┘
                                       │
                         OpenAI / Anthropic / GitHub
```

### 5.2 SOCKS5 数据路径

```text
Codex HTTP/WebSocket
        ↓
127.0.0.1:1080
        ↓ SOCKS5
本地 ssh 进程
        ↓ SSH 加密 TCP
ECS sshd:22
        ↓
目标服务
```

### 5.3 Squid 数据路径

```text
Codex HTTP/WebSocket
        ↓ HTTP CONNECT + Basic Auth
ECS Squid:18443
        ↓
目标服务
```

## 6. 兼容性策略

### 6.1 客户端能力分级

每个客户端必须显式归类，不允许对未知客户端盲目使用 SOCKS5。

| 能力类型 | 含义 | 默认策略 |
|---|---|---|
| `socks5h` | 已验证支持 `ALL_PROXY=socks5h://` | SOCKS5 优先，Squid 降级 |
| `socks5` | 支持 SOCKS5，但域名可能本地解析 | 默认 Squid，除非显式允许 |
| `http-only` | 只支持 HTTP/HTTPS Proxy | 固定 Squid |
| `unknown` | 尚未验证 | 默认 Squid |

初始客户端矩阵：

| 客户端 | 类型 | 依据 | 初始策略 |
|---|---|---|---|
| Codex CLI 0.145.0 | `socks5h` | 本地 `codex doctor` 验证 HTTP/WebSocket 会读取 `ALL_PROXY` | SOCKS5 优先 |
| curl | `socks5h` | 原生支持 `--proxy socks5h://` | SOCKS5 优先 |
| Claude Code | `unknown` | 尚未在本仓库验证 | Squid |
| Git | `unknown` | 不作为第一阶段目标 | 保持现状 |
| npm/pnpm | `unknown` | 不作为第一阶段目标 | 保持现状 |
| Python/pip | `unknown` | SOCKS 支持可能依赖额外模块 | 保持现状 |

### 6.2 客户端配置格式

建议使用不可执行的简单配置，不使用任意 `source`。

建议文件：

```text
config/clients/
├── codex.conf
├── curl.conf
└── claude.conf
```

示例 `config/clients/codex.conf`：

```ini
CLIENT_NAME=codex
COMMAND=codex
PROXY_CAPABILITY=socks5h
PREFERRED_ROUTE=socks5
ALLOW_SQUID_FALLBACK=true
DEEP_CHECK=codex-doctor
```

配置解析器必须：

- 只接受已知键。
- 拒绝重复键。
- 拒绝命令替换、反引号和 shell 语法。
- 限制值的字符集合。
- 不通过 `source` 执行配置。

第一版也可以不引入客户端配置文件，直接在 `run.sh` 中使用受控的 `case`：

```bash
case "$client_name" in
  codex|curl)
    capability="socks5h"
    ;;
  claude)
    capability="unknown"
    ;;
  *)
    capability="unknown"
    ;;
esac
```

在客户端数量很少时，`case` 更简单、更安全。

## 7. 配置与目录设计

### 7.1 仓库目录

目标结构：

```text
agent-proxy/
├── README.md
├── config/
│   ├── proxy.conf.example
│   ├── whitelist.txt
│   ├── no_proxy.txt
│   └── clients/
│       ├── codex.conf
│       └── claude.conf
├── docs/
│   ├── repository-review.md
│   ├── socks5-squid-dual-path-design.md
│   ├── operations.md
│   └── troubleshooting.md
├── scripts/
│   ├── lib/
│   │   ├── common.sh
│   │   ├── config.sh
│   │   ├── health.sh
│   │   └── network-service.sh
│   ├── tunnel-start.sh
│   ├── tunnel-stop.sh
│   ├── tunnel-status.sh
│   ├── run.sh
│   ├── status.sh
│   ├── setup-ecs.sh
│   ├── proxy-on.sh
│   └── proxy-off.sh
└── tests/
    ├── helpers/
    ├── tunnel.bats
    ├── route-selection.bats
    ├── health.bats
    └── config.bats
```

### 7.2 本地运行时目录

```text
~/.agent-proxy/
├── run/
│   ├── ssh-control.sock
│   ├── route.lock
│   └── last-route
├── log/
│   ├── tunnel.log
│   └── route.log
├── state/
│   ├── previous-macos-proxy
│   └── last-health
├── proxy.pac
└── env.sh
```

权限要求：

```text
~/.agent-proxy              700
~/.agent-proxy/run          700
~/.agent-proxy/log          700
~/.agent-proxy/state        700
ssh-control.sock            仅当前用户可访问
last-route                  600
日志文件                    600
```

脚本入口必须执行：

```bash
umask 077
```

### 7.3 秘密目录

```text
.keys/
├── proxy.env
├── singapore-ak.pem
└── known_hosts
```

权限要求：

```text
.keys                       700
.keys/proxy.env             600
.keys/singapore-ak.pem      600
.keys/known_hosts           600
```

### 7.4 配置字段

建议逐步将 `.keys/proxy.env` 拆为非秘密配置和秘密配置。

非秘密配置：

```ini
ECS_HOST=<ECS_IP_OR_HOST>
ECS_SSH_PORT=22
ECS_SSH_USER=<SSH_USER>
SOCKS_BIND=127.0.0.1
SOCKS_PORT=1080
SQUID_HOST=<ECS_IP_OR_HOST>
SQUID_PORT=18443
HEALTH_TARGET=https://chatgpt.com/
CONNECT_TIMEOUT=5
HEALTH_TIMEOUT=12
```

秘密配置：

```ini
SSH_KEY=.keys/singapore-ak.pem
SQUID_USER=<USERNAME>
SQUID_PASS=<PASSWORD>
```

密码必须满足以下二选一条件：

1. 只使用 URL 安全字符。
2. 在生成代理 URL 前进行百分号编码。

不能将未经编码的任意密码直接拼接进 URL。

## 8. SSH 隧道设计

### 8.1 启动命令

目标命令：

```bash
ssh \
  -M \
  -S "$CONTROL_SOCKET" \
  -f \
  -N \
  -T \
  -D "${SOCKS_BIND}:${SOCKS_PORT}" \
  -p "$ECS_SSH_PORT" \
  -i "$SSH_KEY" \
  -o BatchMode=yes \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  -o TCPKeepAlive=yes \
  -o IdentitiesOnly=yes \
  -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$KNOWN_HOSTS" \
  "${ECS_SSH_USER}@${ECS_HOST}"
```

参数说明：

| 参数 | 作用 |
|---|---|
| `-M` | 创建 ControlMaster |
| `-S` | 使用独立 Control Socket |
| `-f` | 完成认证后转入后台 |
| `-N` | 不运行远程命令 |
| `-T` | 不分配终端 |
| `-D` | 创建本地 SOCKS5 动态转发 |
| `BatchMode=yes` | 禁止后台等待交互密码 |
| `ExitOnForwardFailure=yes` | 本地端口绑定失败时立即退出 |
| `ServerAliveInterval=30` | 定期应用层保活 |
| `ServerAliveCountMax=3` | 连续失败后退出 |
| `IdentitiesOnly=yes` | 只使用指定 SSH Key |
| `StrictHostKeyChecking=yes` | 强制校验 ECS 主机身份 |

### 8.2 启动流程

`tunnel-start.sh` 算法：

```text
1. set -euo pipefail
2. umask 077
3. 定位仓库根目录
4. 加载并严格校验配置
5. 创建运行时目录
6. 获取排他锁，防止并发启动
7. 检查 Control Socket
   ├─ SSH master 正常 → 进入 SOCKS 健康检查
   └─ Socket 失效     → 删除失效 Socket
8. 检查 SOCKS 端口
   ├─ 无监听 → 启动 SSH
   └─ 有监听
       ├─ 属于当前 SSH master → 继续
       └─ 属于其他进程       → 报错，不杀进程
9. 等待端口 Ready，最多 CONNECT_TIMEOUT
10. 执行 SOCKS 健康检查
11. 成功写入状态
12. 释放锁并退出 0
```

禁止使用：

```bash
pkill -f ...
```

不能通过模糊进程匹配终止其他程序。

### 8.3 停止流程

使用 Control Socket 精确停止：

```bash
ssh \
  -S "$CONTROL_SOCKET" \
  -O exit \
  -p "$ECS_SSH_PORT" \
  "${ECS_SSH_USER}@${ECS_HOST}"
```

`tunnel-stop.sh` 算法：

```text
1. 获取排他锁
2. Control Socket 不存在 → 幂等成功
3. 执行 ssh -O check
4. Master 存活 → ssh -O exit
5. 等待 SOCKS 端口释放
6. 删除失效 Socket 和状态文件
7. 不终止不属于本工具的进程
```

### 8.4 状态检查

状态分为四层：

| 层级 | 检查 | 含义 |
|---|---|---|
| L1 | Control Socket | SSH master 是否存在 |
| L2 | TCP 127.0.0.1:1080 | 本地 SOCKS 端口是否监听 |
| L3 | SOCKS5 请求目标站点 | 隧道和 ECS 出口是否工作 |
| L4 | Codex Doctor | Codex HTTP/WebSocket 是否兼容 |

普通启动只做 L1-L3。

深度诊断显式执行 L4，避免每次启动 Codex 都产生额外认证请求。

## 9. 健康检查设计

### 9.1 SOCKS5 健康检查

推荐：

```bash
curl \
  --silent \
  --show-error \
  --fail \
  --output /dev/null \
  --connect-timeout "$CONNECT_TIMEOUT" \
  --max-time "$HEALTH_TIMEOUT" \
  --proxy "socks5h://${SOCKS_BIND}:${SOCKS_PORT}" \
  "$HEALTH_TARGET"
```

要求：

- 必须使用 `socks5h`。
- 必须设置连接和总超时。
- 不输出响应正文。
- 不把认证 Token 放进健康请求。
- 目标应支持轻量请求。

如果目标站点对 `HEAD` 支持稳定，可以使用：

```bash
curl --head ...
```

否则使用 GET 并丢弃响应正文。

### 9.2 Squid 健康检查

```bash
curl \
  --silent \
  --show-error \
  --fail \
  --output /dev/null \
  --connect-timeout "$CONNECT_TIMEOUT" \
  --max-time "$HEALTH_TIMEOUT" \
  --proxy "http://${SQUID_HOST}:${SQUID_PORT}" \
  --proxy-user "${SQUID_USER}:${SQUID_PASS}" \
  "$HEALTH_TARGET"
```

使用 `--proxy-user`，避免密码出现在代理 URL。

注意：

- 命令行参数仍可能短暂出现在进程列表。
- 最终方案可从受限配置文件或文件描述符读取凭证。
- 健康检查日志不得打印完整命令。

### 9.3 健康目标选择

健康目标分为两类：

1. **基础网络目标**：检查 ECS 能否访问公网。
2. **业务目标**：检查 ChatGPT/OpenAI 的实际可达性。

建议：

```ini
HEALTH_TARGET_NETWORK=https://example.com/
HEALTH_TARGET_PROVIDER=https://chatgpt.com/
```

启动快速检查只需业务目标。

故障诊断同时检查两者，以区分：

- ECS 整体无法出网。
- 仅目标服务不可达。
- DNS 异常。
- 代理协议异常。

### 9.4 Codex 深度检查

仅在以下情况执行：

- 首次启用 SOCKS5。
- Codex 升级后。
- HTTP 正常但 Codex 仍无法连接。
- 定期人工验收。

命令结构：

```bash
env \
  -u HTTP_PROXY \
  -u HTTPS_PROXY \
  -u http_proxy \
  -u https_proxy \
  ALL_PROXY="socks5h://${SOCKS_BIND}:${SOCKS_PORT}" \
  all_proxy="socks5h://${SOCKS_BIND}:${SOCKS_PORT}" \
  codex doctor --json
```

通过条件：

- `network.provider_reachability` 为成功。
- WebSocket 检查成功，或官方明确允许的 HTTPS fallback 可用。
- 报告确认存在 `ALL_PROXY`。

诊断输出必须经过脱敏，禁止直接保存完整认证或系统环境。

## 10. 路由选择状态机

### 10.1 输入

路由选择依赖：

- 客户端能力类型。
- 用户是否强制指定路由。
- SSH 隧道启动结果。
- SOCKS5 健康检查结果。
- Squid 健康检查结果。
- 是否允许 Squid 降级。

### 10.2 用户覆盖参数

`run.sh` 应支持：

```text
--route auto
--route socks5
--route squid
--no-fallback
--deep-check
--dry-run
```

语义：

| 参数 | 行为 |
|---|---|
| `--route auto` | 默认策略，SOCKS5 优先 |
| `--route socks5` | 强制 SOCKS5；失败时按 fallback 设置处理 |
| `--route squid` | 直接使用 Squid |
| `--no-fallback` | 主链路失败后不允许使用 Squid |
| `--deep-check` | 增加客户端级检查 |
| `--dry-run` | 只输出脱敏后的决策，不启动客户端 |

### 10.3 自动选择算法

```text
读取客户端能力
    │
    ├─ http-only / unknown
    │      ├─ Squid 健康 → 选择 Squid
    │      └─ Squid 异常 → 失败
    │
    └─ socks5 / socks5h
           │
           ├─ 启动或复用 SSH 隧道
           │      ├─ 失败 → 检查 Squid
           │      └─ 成功 → SOCKS 健康检查
           │
           ├─ SOCKS 健康
           │      └─ 选择 SOCKS5
           │
           └─ SOCKS 异常
                  ├─ 允许降级且 Squid 健康 → 选择 Squid
                  └─ 否则 → 失败
```

### 10.4 伪代码

```bash
select_route() {
  case "$requested_route" in
    squid)
      require_squid_health
      SELECTED_ROUTE="squid"
      return
      ;;
    socks5|auto)
      ;;
    *)
      die "unsupported route"
      ;;
  esac

  if [[ "$client_capability" != "socks5h" &&
        "$client_capability" != "socks5" ]]; then
    try_squid_or_fail
    return
  fi

  if tunnel_start && check_socks_health; then
    SELECTED_ROUTE="socks5"
    return
  fi

  if [[ "$allow_fallback" == "true" ]] && check_squid_health; then
    SELECTED_ROUTE="squid"
    return
  fi

  die "no approved proxy route is available"
}
```

### 10.5 防抖与缓存

第一版建议每次启动都执行快速健康检查，不缓存成功结果。

原因：

- 启动频率通常不高。
- 网络状态随 Wi-Fi、运营商和 ECS 状态变化。
- 健康检查耗时可控制在数秒。
- 错误缓存会降低可靠性。

后续如需优化，可缓存最多 30 秒，并且：

- 缓存只减少重复健康检查。
- 不跳过本地端口和 SSH master 检查。
- 失败结果不长期缓存。

## 11. 环境变量注入

### 11.1 SOCKS5 模式

必须清除所有可能覆盖 `ALL_PROXY` 的 HTTP Proxy 变量：

```bash
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy
unset WS_PROXY WSS_PROXY ws_proxy wss_proxy
```

设置：

```bash
export ALL_PROXY="socks5h://${SOCKS_BIND}:${SOCKS_PORT}"
export all_proxy="$ALL_PROXY"
export NO_PROXY="localhost,127.0.0.1,::1"
export no_proxy="$NO_PROXY"
```

然后：

```bash
exec "$@"
```

使用 `exec` 的好处：

- 信号直接传递给目标程序。
- 包装器不额外保留父进程。
- 退出码等于目标程序退出码。

### 11.2 Squid 模式

必须清除 SOCKS 变量：

```bash
unset ALL_PROXY all_proxy
```

设置：

```bash
export HTTPS_PROXY="$SQUID_PROXY_URL"
export HTTP_PROXY="$SQUID_PROXY_URL"
export https_proxy="$SQUID_PROXY_URL"
export http_proxy="$SQUID_PROXY_URL"
export NO_PROXY="$NO_PROXY_VALUE"
export no_proxy="$NO_PROXY_VALUE"
```

代理 URL 中密码必须经过正确编码。

### 11.3 子进程影响

Codex 启动后创建的子进程可能继承代理环境变量。

这意味着：

- Codex 自身模型连接走选定代理。
- Codex 启动的 curl、npm 等也可能看到代理变量。
- 某些工具会忽略 SOCKS5。
- Codex 自身的网络沙箱可能注入独立的本地代理，并按配置决定是否使用上游代理。

因此验收测试必须覆盖：

- Codex 模型请求。
- WebSocket。
- Codex 内运行的 curl。
- Git、npm 等常见工具的预期行为。

如果只希望 Codex 模型连接走代理、工具流量不走代理，需要进一步区分 Codex 主进程和工具子进程环境；这不作为第一阶段目标。

## 12. 命令接口设计

### 12.1 推荐入口

```bash
bash scripts/run.sh codex
bash scripts/run.sh --route squid codex
bash scripts/run.sh --route socks5 --no-fallback codex
bash scripts/run.sh --deep-check codex
bash scripts/run.sh --dry-run codex
```

支持透传参数：

```bash
bash scripts/run.sh codex --model <MODEL>
bash scripts/run.sh codex exec "task"
```

建议通过 `--` 明确分隔包装器和客户端参数：

```bash
bash scripts/run.sh --route auto -- codex exec "task"
```

### 12.2 Shell Alias

用户可自行配置：

```bash
alias codex-sg='bash /absolute/path/agent-proxy/scripts/run.sh -- codex'
```

不建议脚本默认自动修改 `.zshrc`。

### 12.3 状态命令

```bash
bash scripts/status.sh
bash scripts/status.sh --deep
bash scripts/status.sh --json
```

输出示例：

```text
Agent Proxy
  SSH master:       running
  SOCKS listener:   127.0.0.1:1080
  SOCKS health:     healthy
  Squid port:       reachable
  Squid auth:       healthy
  Codex capability: socks5h
  Preferred route:  socks5
  Last route:       socks5
```

JSON 示例：

```json
{
  "sshMaster": "running",
  "socksListener": "healthy",
  "socksHealth": "healthy",
  "squidHealth": "healthy",
  "client": "codex",
  "capability": "socks5h",
  "preferredRoute": "socks5",
  "lastRoute": "socks5"
}
```

不得包含：

- Squid 密码。
- SSH 私钥路径以外的私钥内容。
- OAuth Token。
- 完整认证响应。

## 13. ECS 端设计

### 13.1 第一阶段保持 Squid

第一阶段不卸载 Squid，不删除现有配置。

必须先完成：

- 修复 `Safe_ports` 和 `SSL_ports` ACL。
- 限制云安全组来源 IP。
- 轮换已出现在明文文档中的密码。
- 修复部署脚本错误传播。
- 关闭 `StrictHostKeyChecking=no`。

推荐 Squid ACL：

```squid
acl SSL_ports port 443
acl SSL_ports port 8443
acl Safe_ports port 80
acl Safe_ports port 443
acl Safe_ports port 8443
acl CONNECT method CONNECT
acl authenticated proxy_auth REQUIRED

http_access deny !Safe_ports
http_access deny CONNECT !SSL_ports
http_access deny to_localhost
http_access deny to_linklocal
http_access allow authenticated
http_access deny all
```

### 13.2 SSH 用户

短期可复用现有 SSH 账户和密钥完成验证。

稳定运行前建议创建专用普通用户：

```text
agent-proxy
```

安全要求：

- 无 sudo 权限。
- 禁用密码登录。
- 使用独立 SSH Key。
- 不与 ECS 管理密钥共用。
- 允许 TCP Forwarding。
- 不承担日常管理任务。

`sshd_config` 可考虑按用户限制：

```text
Match User agent-proxy
    PasswordAuthentication no
    PubkeyAuthentication yes
    AllowTcpForwarding local
    X11Forwarding no
    AllowAgentForwarding no
    PermitTunnel no
    GatewayPorts no
```

修改 SSH 配置前必须：

1. 保持一个现有管理 SSH 会话。
2. 执行 `sshd -t`。
3. reload 而不是直接关闭服务。
4. 用第二个终端验证新用户登录。
5. 确认管理通道正常后再退出旧会话。

### 13.3 SSH 主机密钥

客户端必须维护专用 `known_hosts`。

首次录入步骤：

1. 在 ECS 控制台或可信管理通道获取 SSH Host Key Fingerprint。
2. 本地扫描主机密钥。
3. 对比指纹。
4. 确认一致后写入 `.keys/known_hosts`。
5. 设置权限为 `600`。

不能把 `ssh-keyscan` 的输出未经验证直接视为可信。

### 13.4 云安全组

阶段一：

- SSH 22 仅允许可信来源，或按现有管理策略开放。
- Squid 18443 仅允许当前客户端公网 IP。

阶段二观察稳定后：

- 继续保留 Squid，但保持来源 IP 白名单。

最终若淘汰 Squid：

- 关闭安全组 18443。
- 停止 Squid。
- 确认没有其他客户端依赖后再卸载。

## 14. 安全模型

### 14.1 保护目标

本设计保护：

- 客户端到 ECS 的 SOCKS5 业务流量。
- ECS SSH 身份。
- Squid 凭证。
- 防止代理无认证公开使用。
- 防止代理失败后静默本地直连。
- 防止本地 SOCKS 服务暴露给局域网。

### 14.2 信任边界

```text
受信任：
  当前 macOS 用户
  本地 OpenSSH
  ECS 操作系统
  ECS sshd

部分信任：
  本地网络
  公网传输链路
  ECS 云平台

不信任：
  公网扫描者
  未认证 Squid 用户
  未知本地进程
  目标站点返回的非可信内容
```

### 14.3 SOCKS5 链路安全要求

- 只绑定 `127.0.0.1`。
- 禁止绑定 `0.0.0.0`。
- 使用专用 SSH Key。
- 强制 Host Key 校验。
- SSH Key 权限为 `600`。
- 使用 `IdentitiesOnly=yes`。
- 不启用 SSH Agent Forwarding。
- 不启用 Remote Forwarding。
- 不记录请求 URL 或认证 Header。

### 14.4 Squid 降级链路安全要求

在保留期间必须承认：

- 当前 Squid Basic Auth 并不提供客户端到代理的传输加密。
- 来源 IP 白名单只能降低滥用风险，不能代替加密。
- Squid 仅作为过渡兼容链路。
- 不应把其他高价值账户密码复用为 Squid 密码。
- 密码应高强度、随机、独立，并定期轮换。

### 14.5 日志要求

允许记录：

- 时间。
- 选择的链路。
- 健康检查成功/失败。
- 耗时。
- 脱敏后的错误类别。
- 目标客户端名称。

禁止记录：

- Squid 密码。
- 完整代理 URL。
- OAuth Token。
- Authorization Header。
- SSH 私钥。
- 模型请求正文。

日志示例：

```text
2026-07-28T17:00:00+08:00 client=codex route=socks5 result=selected latency_ms=842
2026-07-28T17:15:00+08:00 client=codex route=socks5 result=failed reason=ssh_timeout
2026-07-28T17:15:03+08:00 client=codex route=squid result=selected latency_ms=1190
```

## 15. 故障处理

### 15.1 SSH 无法连接

可能原因：

- ECS 不可达。
- SSH 安全组限制。
- SSH Key 错误。
- Host Key 变化。
- 用户被禁用。

处理：

1. 不修改 `known_hosts` 自动接受新密钥。
2. 输出明确原因。
3. 检查 Squid。
4. Squid 正常则降级。
5. Squid 也失败则拒绝启动。

### 15.2 本地 1080 端口被占用

处理：

- 检查是否为当前 Control Socket 对应的 SSH master。
- 如果不是，不执行 `kill`。
- 输出占用进程 PID 和命令的脱敏摘要。
- 建议用户调整 `SOCKS_PORT` 或关闭冲突程序。
- 允许降级到 Squid。

### 15.3 SSH 存活但 SOCKS 请求失败

可能原因：

- ECS DNS 故障。
- ECS 出网故障。
- 目标服务不可达。
- SSH Channel 打开失败。
- 本地 SSH master 假活。

处理：

1. `ssh -O check`。
2. SOCKS 基础目标检查。
3. SOCKS 业务目标检查。
4. 必要时停止并重建一次隧道。
5. 只允许自动重建一次，避免无限循环。
6. 仍失败则检查 Squid。

### 15.4 Squid 认证失败

处理：

- 区分 407、连接超时、TLS 目标错误。
- 不在日志打印账号密码。
- 提示轮换或同步凭证。
- 不自动重新部署 ECS。

### 15.5 运行中 SSH 断开

处理：

- 当前 Codex 请求可能失败。
- 不自动重放模型请求。
- SSH 可由 LaunchAgent 自动重启。
- 用户重新运行 Codex 包装器时重新选路。
- 如果需要恢复当前任务，由 Codex 自身的 resume 能力处理，而不是网络层重放。

### 15.6 Host Key 变化

默认视为安全事件：

- 禁止自动删除旧 key。
- 禁止使用 `StrictHostKeyChecking=no`。
- 停止 SOCKS5 启动。
- 可检查 Squid 作为临时兼容路径。
- 通过 ECS 控制台核验新指纹后人工更新。

## 16. 性能设计

### 16.1 预期

对 Codex 的流式请求：

- 网络 RTT 是主要延迟来源。
- SSH 加密开销通常很小。
- SOCKS5 协议协商开销很小。
- 长连接建立后，差异主要体现在网络稳定性。

### 16.2 SSH 单连接复用

多个 SOCKS5 Channel 会复用同一个 SSH TCP 连接。

优点：

- 连接建立成本低。
- SSH 保活和身份管理简单。

缺点：

- 丢包时可能出现 TCP 队头阻塞。
- 一条 SSH master 断开会影响所有逻辑连接。

对于低带宽 Agent 请求通常可以接受。

### 16.3 基准指标

对每条链路至少测量：

- TCP/代理连接时间。
- 首字节时间。
- 请求总时间。
- 20 次中位数。
- 20 次 P95。
- 30 分钟 WebSocket 稳定性。
- 2 小时 Codex 长任务稳定性。
- 断网恢复时间。

curl 示例：

```bash
curl \
  --proxy socks5h://127.0.0.1:1080 \
  --output /dev/null \
  --silent \
  --write-out \
  'connect=%{time_connect} start=%{time_starttransfer} total=%{time_total}\n' \
  https://chatgpt.com/
```

Squid 基准不得把密码写进报告。

## 17. 自动化测试

### 17.1 静态检查

必须通过：

```text
bash -n
ShellCheck
shfmt check
Markdownlint
Secret scanner
```

### 17.2 Bats 单元测试

使用 stub 替代：

- `ssh`
- `curl`
- `nc`
- `lsof`
- `networksetup`
- `security`

必须覆盖：

#### 隧道测试

- 首次启动成功。
- 重复启动幂等。
- Control Socket 健康。
- Socket 失效后安全清理。
- 本地端口冲突。
- SSH 认证失败。
- Host Key 校验失败。
- SSH 超时。
- 精确停止。
- 不误杀无关进程。

#### 路由测试

- Codex + SOCKS 健康 → SOCKS。
- Codex + SOCKS 失败 + Squid 健康 → Squid。
- Codex + 两者失败 → 非零退出。
- `unknown` 客户端 → Squid。
- `http-only` 客户端 → Squid。
- `--route squid` → 不启动 SSH。
- `--route socks5 --no-fallback` → SOCKS 失败即终止。
- `--dry-run` → 不启动客户端。

#### 环境测试

- SOCKS 模式清除 HTTP Proxy。
- SOCKS 模式设置大小写 `ALL_PROXY`。
- Squid 模式清除 `ALL_PROXY`。
- Squid 模式设置大小写 HTTP Proxy。
- 环境中已有代理变量时不会污染选路。
- 参数中含空格时正确透传。
- 目标程序退出码正确返回。

#### 安全测试

- 日志不包含密码。
- `--dry-run` 不包含密码。
- 配置权限过宽时拒绝或警告。
- 非法配置键被拒绝。
- 配置中的 shell 语法不会执行。
- SOCKS Bind 非 loopback 时拒绝。

### 17.3 集成测试

在真实 ECS 上验证：

1. SSH SOCKS5 访问普通 HTTPS。
2. SSH SOCKS5 访问 ChatGPT。
3. SOCKS5 远端 DNS。
4. Codex Doctor HTTP。
5. Codex Doctor WebSocket。
6. Codex 交互式流式响应。
7. SSH 隧道关闭时降级到 Squid。
8. Squid 停止时 SOCKS5 正常。
9. 两者停止时启动失败。
10. 网络从 Wi-Fi 切换到其他接口后的表现。

### 17.4 故障注入

建议测试：

- 临时阻断 ECS SSH 端口。
- 临时停止 Squid。
- 临时停止 sshd 前必须保留云控制台恢复路径。
- 占用本地 1080。
- 修改错误 Host Key 测试文件。
- 使用错误 Squid 密码。
- 模拟 DNS 失败。
- 模拟目标服务返回 403/429/5xx。

必须区分：

- 网络不可达。
- 代理不可达。
- 认证失败。
- 目标业务错误。

429 不应被判断为代理链路失败。

## 18. 实施 Roadmap

### Phase 0：安全基线与备份

预计工作量：0.5 天

任务：

1. 备份现有 Squid 配置。
2. 记录当前云安全组规则。
3. 记录当前 macOS PAC 设置。
4. 轮换 Squid 密码。
5. 删除文档中的真实凭证。
6. 收紧 `.keys` 和生成文件权限。
7. 创建并验证 SSH `known_hosts`。
8. 保留现有 Squid 可用性。

交付物：

- 脱敏配置备份。
- 已验证的 Host Key。
- 权限检查结果。
- 当前 Squid 健康基线。

退出条件：

- Squid 仍可用。
- 新凭证未出现在 Git 或日志中。
- SSH 可以在严格 Host Key 模式连接。

### Phase 1：SOCKS5 手工原型

预计工作量：0.5 天

任务：

1. 使用现有 SSH 账户手工运行 `ssh -D`。
2. 使用 curl 验证 `socks5h`。
3. 使用 Codex Doctor 验证 HTTP 和 WebSocket。
4. 运行至少一次 Codex 交互任务。
5. 记录延迟、错误和稳定性。

交付物：

- 手工验证命令。
- Codex 兼容性结果。
- SOCKS5 与 Squid 基线数据。

退出条件：

- Codex HTTP 可用。
- WebSocket 可用，或明确验证 HTTPS fallback。
- DNS 确认由 ECS 侧解析。
- Squid 保持可用。

### Phase 2：隧道生命周期脚本

预计工作量：1 天

任务：

1. 新增公共配置和校验库。
2. 实现 `tunnel-start.sh`。
3. 实现 `tunnel-stop.sh`。
4. 实现 `tunnel-status.sh`。
5. 使用 Control Socket。
6. 加入并发锁、端口冲突和严格 Host Key 检查。
7. 增加 L1-L3 健康检查。

交付物：

- 幂等的隧道启停脚本。
- 脱敏状态输出。
- 单元测试。

退出条件：

- 连续启动两次不会创建重复隧道。
- 停止不会误杀其他进程。
- 端口冲突会明确失败。
- SSH 失败能够返回非零退出码。

### Phase 3：双链路启动器

预计工作量：1 天

任务：

1. 实现 `run.sh`。
2. 实现客户端能力矩阵。
3. 实现 SOCKS5 优先选路。
4. 实现 Squid 降级。
5. 实现 `--route`、`--no-fallback`、`--dry-run`。
6. 正确清理和注入代理环境变量。
7. 记录最后选路，不记录秘密。

交付物：

- `bash scripts/run.sh -- codex`。
- Codex 启动包装器。
- 路由状态机测试。

退出条件：

- SOCKS 健康时选择 SOCKS。
- SSH 被阻断时自动选择 Squid。
- 两者失败时拒绝启动。
- 当前 shell 中已有代理变量不影响选路。

### Phase 4：Squid 兼容链路加固

预计工作量：0.5-1 天

任务：

1. 修复 Squid ACL 绕过。
2. 修复 `setup-ecs.sh` 的错误吞噬。
3. 远程脚本启用严格模式。
4. 修复变量传递和引用。
5. 限制云安全组来源。
6. 确认未认证请求被拒绝。
7. 确认非安全端口 CONNECT 被拒绝。

交付物：

- 可重复执行的部署脚本。
- Squid 安全测试结果。

退出条件：

- `squid -k parse` 失败时部署必定失败。
- Squid 服务未启动时脚本必定失败。
- 未认证、错误端口和未授权来源都被拒绝。

### Phase 5：统一状态与诊断

预计工作量：0.5 天

任务：

1. 重构 `status.sh`。
2. 展示 SOCKS、SSH、Squid 和 Codex 状态。
3. 支持 `--deep`。
4. 支持脱敏 JSON。
5. 删除硬编码 Wi-Fi。
6. 区分警告和失败。

退出条件：

- 普通检查在数秒内完成。
- 深度检查能定位 HTTP/WebSocket 问题。
- 输出不含任何密码或 Token。

### Phase 6：稳定性观察

预计周期：7 天

观察要求：

- 日常 Codex 全部通过 `run.sh` 启动。
- Squid 保持在线但不作为默认路径。
- 记录每次启动选择的路径。
- 记录 SOCKS 降级次数和原因。
- 至少完成一个两小时以上长任务。
- 至少完成一次网络切换测试。
- 至少完成一次 SSH 故障降级演练。

成功标准：

- SOCKS5 选用率不低于 95%。
- 无凭证泄漏。
- 无静默直连。
- 无无法恢复的系统代理修改。
- Squid 降级可重复工作。

### Phase 7：后台自动化，可选

预计工作量：1 天

只有 Phase 6 证明链路稳定后才实施。

任务：

- 使用 macOS LaunchAgent 管理 SSH master。
- 登录后自动启动。
- 异常退出自动重启。
- 网络变化后自动恢复。
- 保持本地绑定为 loopback。
- 保留手工启动和停止命令。

不要求安装 `autossh`。

退出条件：

- 登录后隧道自动可用。
- SSH 进程崩溃后可恢复。
- 不产生重启风暴。
- 用户可明确关闭自动启动。

### Phase 8：后续决策

观察 2-4 周后评估：

1. 是否仍有客户端依赖 Squid。
2. Squid 实际降级次数。
3. SOCKS5 的稳定性和性能。
4. 是否值得继续承担公网 Squid 风险。

可能结果：

- 保留双链路。
- Squid 改为仅紧急手工启用。
- 关闭公网 Squid。
- 完全卸载 Squid。

本阶段不预设必须删除 Squid。

## 19. 上线步骤

推荐顺序：

```text
1. 完成安全备份
2. 不动现有 Squid
3. 新增 SSH SOCKS5
4. 手工验证
5. 新增 run.sh
6. 默认使用 SOCKS5
7. 演练 Squid 降级
8. 观察七天
9. 再决定是否自动启动
```

上线当天检查：

- SSH Host Key 已核验。
- SSH Key 权限正确。
- 本地 1080 未被其他程序占用。
- Squid 仍然健康。
- Codex Doctor 通过 SOCKS5。
- `run.sh --dry-run` 显示 SOCKS5。
- 主动阻断 SSH 后，`run.sh --dry-run` 显示 Squid。
- 恢复 SSH 后，再次显示 SOCKS5。

## 20. 回滚方案

### 20.1 应用级回滚

强制 Squid：

```bash
bash scripts/run.sh --route squid -- codex
```

### 20.2 停止 SOCKS5

```bash
bash scripts/tunnel-stop.sh
```

不会修改：

- ECS Squid 配置。
- macOS 系统 PAC。
- 现有 Squid 密码。

### 20.3 完整回滚

如果新方案需要整体撤回：

1. 停止 SSH master。
2. 将 Codex alias 指回原 Squid 启动方式。
3. 保留新脚本但不调用。
4. 不删除测试和诊断结果。
5. 记录失败原因后再修复。

回滚不需要重启 ECS。

## 21. 验收标准

### 功能验收

- [ ] `tunnel-start.sh` 可以建立 SOCKS5。
- [ ] 重复启动幂等。
- [ ] `tunnel-stop.sh` 精确停止。
- [ ] Codex HTTP 通过 SOCKS5。
- [ ] Codex WebSocket 通过 SOCKS5。
- [ ] SOCKS5 使用远端 DNS。
- [ ] SOCKS5 失败时自动选择 Squid。
- [ ] Squid 失败时 SOCKS5 不受影响。
- [ ] 两者失败时目标程序不会启动。
- [ ] 用户可强制指定任一路径。

### 安全验收

- [ ] 本地 SOCKS 只绑定 loopback。
- [ ] SSH Host Key 严格校验。
- [ ] 私钥和秘密配置权限为 600。
- [ ] 日志不含密码和 Token。
- [ ] 文档不含真实凭证。
- [ ] Squid ACL 无端口绕过。
- [ ] Squid 安全组限制来源。
- [ ] 未出现静默直连。

### 可靠性验收

- [ ] SSH 断开能够被检测。
- [ ] 端口冲突不会误杀进程。
- [ ] 目标服务 429 不被误判为代理故障。
- [ ] Wi-Fi 切换后可以恢复。
- [ ] 两小时 Codex 会话稳定。
- [ ] 所有脚本通过静态检查和测试。

## 22. 运维手册摘要

日常启动：

```bash
bash scripts/run.sh -- codex
```

查看状态：

```bash
bash scripts/status.sh
```

深度诊断：

```bash
bash scripts/status.sh --deep
```

强制 Squid：

```bash
bash scripts/run.sh --route squid -- codex
```

强制 SOCKS5，不允许降级：

```bash
bash scripts/run.sh --route socks5 --no-fallback -- codex
```

停止 SOCKS5：

```bash
bash scripts/tunnel-stop.sh
```

## 23. 最终推荐

实施时不要一次性替换全部现有能力。

推荐演进状态：

```text
第一周：
  SOCKS5 默认
  Squid 自动降级
  Squid 公网端口限制来源

稳定后：
  SOCKS5 默认
  Squid 只作为兼容和应急链路
  评估是否进一步缩短 Squid 开放时间
```

这套双链路设计的核心价值是：

- SOCKS5 提供更安全、更简单的默认路径。
- Squid 保留现有 HTTP Proxy 兼容性。
- 不依赖额外账号或第三方控制服务。
- 迁移失败可以立即回滚。
- 用户始终明确知道实际选择了哪条链路。

在当前仓库范围内，这是比继续单独使用公网 Squid 更安全，同时比引入完整 VPN 更容易实施的折中方案。
