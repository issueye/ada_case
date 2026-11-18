# AI 进程插件（lightcode-ai）

> 助手从「编辑器里的一个包」变成「仓库里独立的一个项目」：自己的 `go.mod`、
> 自己的二进制、自己的测试，和编辑器之间只有一条管道和一份协议。
> `ada_link` 的代码并入了本仓库，插件的消息层就建立在它的 `link` 包上。

## 1. 三个部分，一条依赖线

```
ada_case/
├── ada_link/          # 并入的进程间库（github.com/learningdog1/ada_link）
├── lightcode-ai/      # AI 客户端：独立项目、独立二进制 lightcode-ai(.exe)
│   ├── protocol/      #   契约：wire 类型与常量（不含任何编辑器代码）
│   ├── provider/      #   OpenAI 兼容 SSE 客户端
│   └── serve/         #   管道上的握手与 ai.stream / ai.cancel
└── lightcode/         # 编辑器
    └── internal/aiplugin/ # 只做宿主侧：拉起、流式路由、取消、重启、状态
```

依赖方向是单向的，且只有一条：

| 从 | 到 | 用来做什么 |
|---|---|---|
| `lightcode` | `lightcode-ai/protocol` | 只拿 wire 类型与常量 |
| `lightcode` | `ada_link/link` | 消息层（call/reply/event、握手、帧格式） |
| `lightcode-ai` | `ada_link/link` | 同一套消息层 |
| `lightcode-ai` | 编辑器 | **没有**：插件不 import 编辑器的任何包 |
| `lightcode` | 插件实现 | **没有**：编辑器不 import `serve`/`provider` |

`lightcode-ai` 因此可以单独 `go build`、单独测试、单独发布；编辑器的二进制里
不再有 HTTP 客户端代码。反向也成立：插件不读编辑器的文件、不读凭据库，
请求里带什么就用什么。

## 2. `ada_link` 并入了哪些，改了什么

并入的是源码（`link`、`mygoembed`、`panels`、`install`、`internal/shm`、
`examples`、`cmd`、文档），不含它原来的 `bin/` 与 `artifacts/`。模块路径保持
`github.com/learningdog1/ada_link`，同级 `replace` 指向 `../ada_link`。

为插件做的最小改动（其余原样）：

| 改动 | 原因 |
|---|---|
| `Peer` 的 `conn` 从 `net.Conn` 收紧为 `io.ReadWriteCloser` | 插件走父子进程的私有管道，不是 socket；`Peer` 只用 Read/Write/Close |
| 新增 `link.PipeConn` / `NewPipeConn(r, w)` | 把 stdin/stdout 变成 `Peer` 能读写的连接 |
| 新增 `link.NewQuietPeer` 与 `(*Peer).Start` | 先注册 `Handle`/`OnMessage`、再开始读循环，避免握手消息与处理器竞争 |

没有用的部分：帧环、像素中继、输入转发、会话目录、socket + token——插件没有
界面，只有文本；管道是父进程私有的，第三方连不进来。

## 3. 协议

信封仍然是 `link.Message`（`t` = `hello`/`welcome`/`call`/`reply`/`event`/`bye`），
4 字节小端长度 + JSON，单条上限 16 MiB。载荷是 `lightcode-ai/protocol` 的类型。

| 消息 | 方向 | 载荷 | 用途 |
|---|---|---|---|
| `hello` | 编辑器 → 插件 | `proto`、`app` | 报协议版本 |
| `welcome` | 插件 → 编辑器 | `Welcome{name, version, methods}` | 身份、版本、可用方法 |
| `call ai.stream` | 编辑器 → 插件 | `StreamRequest{requestID, config, messages}` | 问一次 |
| `event delta` | 插件 → 编辑器 | `Delta{requestID, text}` | 回答的增量 |
| `reply` | 插件 → 编辑器 | `StreamResult{requestID, text}` 或错误 | 回答结束 |
| `call ai.cancel` | 编辑器 → 插件 | `CancelRequest{requestID}` | 停止某次回答 |
| `bye` | 编辑器 → 插件 | — | 有序退出 |

`requestID` 是关键：一次 `ai.stream` 的增量和取消都带着它，编辑器只把属于
本次提问的片段交给打字机，迟到片段丢弃。

## 4. 进程生命周期

- **发现**：`LIGHTCODE_AI_PLUGIN`（显式路径）→ 编辑器可执行文件同目录的
  `lightcode-ai(.exe)` → 当前目录 → `PATH`。找不到就报错并给出构建命令，
  不再回退到编辑器自己的进程。
- **懒启动**：第一次提问才 `exec`；只写代码不问 AI 的会话不付代价。
- **握手**：编辑器发 `hello`，等 `welcome`（默认 5 s）；版本不符直接失败。
- **流式**：`ai.stream` 在插件里是一个 goroutine，`delta` 事件按到达顺序写回。
- **取消**：编辑器取消 context → 发 `ai.cancel` → 插件取消该请求的 context，
  HTTP 请求中断；已到的文本保留。
- **崩溃/被杀**：下一次提问检测到连接结束，自动再拉一个进程；能力面板里的
  「重启」可手动换掉。
- **关闭**：窗口关闭时发 `bye`、关管道，500 ms 宽限后杀进程。

## 5. 安全

- Key 由编辑器从系统凭据库（`mygo.Secrets`）读出，随每个请求经管道传给插件，
  插件只放在内存里，不写盘。
- 管道是父子进程私有的标准流，不经网络、不经临时文件；没有 `ada_link` 的
  socket/token 那一层，因为不存在第三方能连上的路径。
- 插件不读编辑器的工作区、不读凭据库，也不改文件：它只发 HTTP。

## 6. 构建与运行

```powershell
# 插件：独立项目，独立构建
cd lightcode-ai
go build -o ..\lightcode\lightcode-ai.exe .   # Windows
go build -o ../lightcode/lightcode-ai .       # macOS / Linux

# 编辑器（同目录会找到插件）
cd ..\lightcode
go run ./cmd/lightcode
```

`go run` 的可执行文件在临时目录，找不到「同目录」的插件；从 `lightcode/`
目录启动时走「当前目录」规则，或显式指定：

```powershell
$env:LIGHTCODE_AI_PLUGIN = "E:\codes\go_projects\ada_case\lightcode\lightcode-ai.exe"
```

插件支持 `lightcode-ai -version`。

## 7. 测试（三块，各自独立）

| 模块 | 测试 | 证明什么 |
|---|---|---|
| `ada_link` | `link/link_test.go` 等原有测试 | 并入后消息层与帧协议未退化；`link` 在管道上仍工作 |
| `lightcode-ai` | `provider/provider_test.go` | SSE 流式、请求体、HTTP 错误、取消 |
| | `serve/serve_test.go` | 管道上握手、流式增量与整段一致、取消、空 Key |
| `lightcode` | `internal/aiplugin/client_test.go` | **真子进程**：拉起、增量/整段、取消、错误、被杀后换新、协议不符、启动即失败 |
| | `internal/app/aiclient_test.go` | 发现规则、缺插件时的报错、能力面板文案 |
| | `internal/app/ai_test.go`、`m4_screenshot_test.go` | 面板逻辑用注入的 runner 跑，不需要进程或网络 |

`internal/aiplugin` 的测试用一个测试进程扮演插件（`LIGHTCODE_AI_PLUGIN_HELPER=serve`），
它和真插件说同一份协议；真插件与编辑器的端到端则另外手动跑过：
编辑器 pid 与插件 pid 不同、`status` 里能看到 `lightcode-ai v0.2.0`。

## 8. 限制与后续

- 真实模型仍未验证（没有可用的 Key），所有测试打的是本地 httptest 端点。
- 一个编辑器会话共用一个插件进程；多窗口会串行共用，目前只有单窗口。
- 插件跟系统代理只认 `HTTP(S)_PROXY` 环境变量：M4 时用的是 MyGo 的系统代理设置，
  但那会让独立项目重新依赖 UI 框架，取舍见 §2；需要时可以把代理配置作为请求
  字段传进来。
- 没有插件清单/权限/签名：现在只信任「用户自己构建的那份」。
- 没有随编辑器打包发布：`LIGHTCODE_AI_PLUGIN` 已留出入口，打包目标未做。
