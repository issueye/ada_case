# AI 进程插件（ai-provider）

> 把助手的网络调用移出编辑器进程：LightCode 的窗口里只留对话与 diff 卡片，
> 真正读 SSE 的是一根标准输入 / 标准输出管道另一端的 `lightcode ai-provider`。
> 本文先分析 `ada_link` 的进程间做法，再写本项目的协议、生命周期、安全与验证。

## 1. 为什么做

M4 的助手是进程内的：`internal/ai` 直接在窗口的 goroutine 里建 HTTP 请求、读
SSE。它有三个问题：

| 问题 | 现象 |
|---|---|
| 网络在窗口进程里 | 端点卡住时，那个 goroutine 挂着；插件式的可替换性也无从谈起 |
| 代码互相牵连 | 只要编译编辑器，就必须编译整个 OpenAI 客户端；换一家实现要重编整个应用 |
| 故障没有边界 | 一个有问题的 provider 实现能拖垮窗口进程，而窗口本该与它无关 |

进程插件的目标很小：**一条管道、一个版本号、一次一问一答的流式通道**，
够用就好，不复制 `ada_link` 的像素中继。

## 2. `ada_link` 分析

`E:\codes\go_projects\ada_link` 是同一作者的另一个库：把**另一个 MyGo 应用的
native UI 嵌进宿主窗口**。它对本项目有参考价值的部分是进程模型与消息层，而不是
渲染。

### 2.1 它的结构

| 路径 | 内容 |
|---|---|
| `link/proto.go` | 消息定义、4 字节小端长度前缀 + JSON、`Peer`（call/reply/event）、`MaxMessage = 16 MiB`、`Proto = 1` |
| `link/spawn.go` | `Spawn`：临时目录 + Unix socket + 随机 token，子进程经环境变量拿到 socket/ token，握手 `hello`/`welcome` |
| `link/session.go` | 宿主端会话：帧环、输入转发、`Done/Err/OnClose`、关闭时先礼后兵（2 s 宽限再杀） |
| `link/typed.go` | 泛型 `Call[Req,Res]` 与 `HandleFunc`：调用双方共用一套类型 |
| `mygoembed/` | `Attach`（宿主把帧当纹理合成）、`Serve`（子进程离屏渲染同一份 `ui.View`） |
| `panels/` | 多面板注册表、事件命名空间与广播 |

关键协议语义（与本文直接相关）：

- 每次调用有 `ID`，回复带 `OK`/`Value`/`Error`；调用方可带 context 先放弃等待，
  迟到的回复被丢弃；
- `event` 是单向通知，读循环里投递；监听者可以晚于事件到达；
- 子进程退出、连接断开都会让 `Session.Done` 关闭，并把原因留给 `Err()`；
- 传输安全靠 socket 权限 `0600` + 随机 token 的 `hello` 校验。

### 2.2 哪些能借，哪些不能

| `ada_link` 的做法 | 本项目采用 | 理由 |
|---|---|---|
| `call`/`reply`/`event` 词汇与 `ID` 语义 | 借用 | 心智模型一致，流式答复可以拆成事件 + 最终回复 |
| 4 字节长度前缀 + JSON、`MaxMessage` 上限 | 借用 | 与纯文本行协议相比能容下大段上下文，又不必自己发明编码 |
| `hello`/`welcome` + `Proto` 校验 | 借用 | 版本不匹配时应当明确报错，而不是读出一堆怪消息 |
| socket + 随机 token | **不用** | 管道是父进程私有的：没有第三方能连，token 只是多余的一层 |
| 帧环、像素中继、输入转发 | 不用 | 插件没有界面，只有文本 |
| 会话目录清理、帧确认 | 不用 | 管道随进程结束，系统回收 |
| 关闭时 2 s 宽限再杀 | 借用 | 先发 `bye`，给 500 ms 自行退出，再杀 |
| 泛型 `Call[Req,Res]` | 部分借用 | 协议层保留裸 JSON；上层用 `StreamRequest`/`StreamResult` 直接编解码 |

结论：**借消息层，弃渲染层**。这样插件协议只有约 330 行（含客户端），
不需要把 `ada_link` 作为依赖引进来（它依赖 MyGo 与 cgo-free 的 socket 路径，
对只需要文本通道的项目是多余的）。

## 3. 设计

### 3.1 进程模型

```
┌─────────────────────────────┐   stdin/stdout（私有管道）  ┌────────────────────┐
│ lightcode 窗口进程           │◀──────────────────────────▶│ lightcode ai-provider │
│  aiplugin.Client            │   长度前缀 JSON：call/reply │  aiprovider.Serve     │
│  对话、diff、打字机、UI       │   event：答案的增量          │  internal/ai → HTTP   │
└─────────────────────────────┘                            └────────────────────┘
```

- **自己拉起自己**：`aiPluginCommand()` 默认返回 `os.Executable() + "ai-provider"`。
  一个构建产物同时是编辑器与插件，开发时 `go run ./cmd/lightcode` 也一样成立。
  设置 `LIGHTCODE_AI_PLUGIN` 可以改指向单独构建的插件可执行文件。
- **懒启动**：只有第一次提问才 `exec` 子进程。一个只写代码、不问 AI 的会话
  不会为插件付内存与启动时间。
- **卡死可换**：进程死掉后，下一次 `Stream` 自动再拉一个；能力面板里的「重启」
  可以立刻换掉卡住的实现。
- **窗口关闭即退出**：`OnClosed` 里发 `bye`、关管道，超时 500 ms 再杀。

### 3.2 帧与消息

帧：`uint32` 小端长度（`<= 16 MiB`）+ UTF-8 JSON。与 `ada_link` 相同。

| `t` | 方向 | 字段 | 用途 |
|---|---|---|---|
| `hello` | 宿主 → 插件 | `proto`、`name` | 父进程先报协议版本 |
| `welcome` | 插件 → 宿主 | `proto`、`name`、`version`、`methods` | 版本、插件身份、可用方法 |
| `call` | 双向 | `id`、`m`、`a[]` | 调一个方法 |
| `reply` | 双向 | `id`、`ok`、`v` / `e` | 结果或错误 |
| `event` | 插件 → 宿主 | `n`、`p` | 流式增量 |
| `bye` | 宿主 → 插件 | — | 有序退出 |

方法：

| 方法 | 参数 | 结果 | 说明 |
|---|---|---|---|
| `ai.stream` | `StreamRequest{requestID, config, messages}` | `StreamResult{requestID, text}` | 答复结束时回复；期间以 `delta` 事件流出 |
| `ai.cancel` | `CancelRequest{requestID}` | `null` | 取消某个仍在进行的答复 |

`delta` 事件载荷：`{requestID, text}`。**每个增量都带 request ID**：宿主用它在
自己的事件路由里只把属于本次请求的片段交给打字机，迟到的片段会被丢弃。

### 3.3 一次问答的时序

```
窗口线程            aiplugin.Client               插件进程            HTTP 端点
   │ SendAI ──────────▶ ensure()：启动、hello/welcome
   │                   │ call ai.stream ──────────▶ stream()
   │                   │                           │ POST /chat/completions ─▶
   │                   │◀── event delta ───────────│◀── SSE data: 片段 ───────
   │ 每 50 ms 批量更新  │                           │
   │                   │◀── reply {text} ──────────│ 结束 / 出错
   │ ◀─ finalize、diff 卡片、打字机追平
```

取消时宿主发 `ai.cancel`；插件 `cancel()` 那个请求的 context，HTTP 请求随之中断，
回复带 `context canceled`，宿主按取消处理（保留已到文本，不弹错误）。

### 3.4 与窗口内的分工

- 窗口进程：拼上下文（系统提示、当前文件 / 选中的文字、历史轮次）、
  50 ms 批量写入、按帧揭示打字机、解析 diff、确认后写文件。
  这些都是 UI 逻辑，留在窗口进程是对的。
- 插件进程：只有「拿配置去请求、把片段流回来」一件事，不碰文件、不读凭据。

**Key 不落盘在插件里**：窗口从系统凭据库（`mygo.Secrets`）读出，随每个请求
经管道送过去，插件只放在内存里。管道是父进程私有的，不经网络、不经文件。

### 3.5 无窗口路径

测试与将来可能的 CLI 没有窗口，`SendAI` 直接调 `aiprovider.Stream`（同一份
provider 实现，进程内），因此测试快、不需要拉起进程；窗口里走插件。两条路径
共用 `aiprovider.Stream`，不会出现「测试通过、界面另一套逻辑」的分叉。

## 4. 代码地图

| 路径 | 内容 |
|---|---|
| [`lightcode/internal/aiplugin/protocol.go`](../lightcode/internal/aiplugin/protocol.go) | 协议常量、帧读写、`Config`/`ChatMessage`/请求与事件结构 |
| [`lightcode/internal/aiplugin/peer.go`](../lightcode/internal/aiplugin/peer.go) | `Peer`：call/reply/event、读循环、关闭与等待者唤醒 |
| [`lightcode/internal/aiplugin/client.go`](../lightcode/internal/aiplugin/client.go) | 宿主端：拉起进程、握手、`Stream`（增量路由 + 取消）、`Status`、重启 |
| [`lightcode/plugins/aiprovider/serve.go`](../lightcode/plugins/aiprovider/serve.go) | 插件端：`Serve`、`ai.stream`/`ai.cancel`、`Stream`（进程内共用） |
| [`lightcode/internal/app/aiclient.go`](../lightcode/internal/app/aiclient.go) | 窗口接线：命令、懒启动、关闭、重启、能力面板文案 |
| `lightcode/cmd/lightcode/main.go` | `ai-provider` 子命令分派 |
| [`lightcode/internal/ai/ai.go`](../lightcode/internal/ai/ai.go) | OpenAI 兼容 SSE 客户端，只有插件（与无窗口路径）用它 |

## 5. 测试

| 测试 | 证明什么 |
|---|---|
| `aiplugin/peer_test.go` | 帧往返、超大帧拒绝、调用/回复、错误回复、事件、关闭后的调用 |
| `aiplugin/client_test.go` | **真子进程**：握手、增量与整段一致、取消、被杀后自动换新进程、协议版本不符、启动即退出、空 Key |
| `cmd/lightcode/main_test.go` | 真实分派：argv `ai-provider` 时回答 `hello`，而不是开窗口 |
| `app/aiclient_test.go` | 默认命令是 `exe + ai-provider`、环境变量可改指、能力面板的「按需启动」与重启 |
| `app/ai_test.go`、`m4_screenshot_test.go` | 无窗口路径：问答、diff 卡片、重试、Key 缺失 |

`client_test.go` 用测试二进制自身当插件（`LIGHTCODE_AI_PLUGIN_HELPER=serve`），
所以跑的是 `aiprovider.Serve` 本体、真的 `exec`、真的管道、真的 HTTP。

## 6. 限制与后续

- **未验证真实模型**：所有测试打的是本地 httptest 端点，与 M4 的结论一致
  （没有可用的 Key）。
- **进程数固定为一个**：多窗口 / 多会话会串行共用同一个插件；目前只有单窗口。
- **没有插件清单与权限模型**：现在只信任「自己拉起的自己」；若要装第三方插件，
  需要签名、权限声明、版本兼容，这些都没做。
- **没有独立分发**：`LIGHTCODE_AI_PLUGIN` 已留出口，但还没有构建 / 打包单独
  `lightcode-ai` 的目标。
