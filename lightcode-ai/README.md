# lightcode-ai

轻码 IDE 的 AI 客户端，**独立项目**：自己的 `go.mod`、自己的二进制、自己的测试。
它不 import 编辑器的任何包；编辑器只 import 它的 `protocol` 契约包。

```
protocol/   线上类型与常量（hello/welcome、ai.stream、ai.cancel、delta）
provider/   OpenAI 兼容的 SSE 客户端
serve/      管道上的握手与方法实现
```

## 构建

```powershell
go build -o ..\lightcode\lightcode-ai.exe .   # Windows
go build -o ../lightcode/lightcode-ai .       # macOS / Linux
go build -o lightcode-ai .                    # 只构建，放到任意目录
```

`lightcode-ai -version` 打印名称与版本。

## 协议

消息层来自并入本仓库的 `ada_link/link`：4 字节小端长度 + JSON，
`hello` / `welcome` / `call` / `reply` / `event` / `bye`。载荷：

| 方法/事件 | 载荷 |
|---|---|
| `ai.stream` | `StreamRequest{requestID, config, messages}` → `StreamResult{requestID, text}` |
| `delta` 事件 | `Delta{requestID, text}` |
| `ai.cancel` | `CancelRequest{requestID}` |

编辑器发 `hello`（带协议版本），本进程回 `welcome{name, version, methods}`；
版本不符即拒绝。全部说明与取舍见 [`../docs/ai-plugin.md`](../docs/ai-plugin.md)。

## 依赖

只有两样：标准库 + `ada_link/link`。不依赖 MyGo、不依赖编辑器、不依赖编辑器的
凭据库或工作区。请求里带 `config`（BaseURL、模型、Key），Key 只在内存里。

代理：HTTP 客户端用默认 transport，认 `HTTP(S)_PROXY` 环境变量；不再读桌面系统的
代理设置——那需要重新引入 UI 框架，与这里的独立性相悖。

## 测试

```powershell
go test ./...
```

- `provider`：SSE 分片、请求体、HTTP 错误、取消；
- `serve`：内存管道上的握手、流式增量与整段一致、取消、空 Key。

真进程的端到端由编辑器的 `internal/aiplugin` 测试覆盖（它 spawn 一个测试进程扮演
插件，协议与本项目一致）。
