# AI 面板（lightcode-ai）——界面也在插件里

> 助手是仓库里独立的一个项目：它用 MyGo 的 `ui` 自己画面板，离屏渲染成帧，
> 由 `ada_link` 的像素中继送进编辑器窗口的一个矩形里；编辑器只提供挂载点、
> 转发输入、代它读文件与存 Key。

## 1. 三个部分，一条依赖线

```
ada_case/
├── ada_link/            # 并入的进程间库：link（IPC）+ mygoembed（帧中继/输入转发）
├── lightcode-ai/        # AI 面板：独立项目、独立二进制 lightcode-ai(.exe)
│   ├── assistant/       #   面板自己的 UI：标题头、对话、输入、diff 卡片、卡片态
│   ├── protocol/        #   契约：configure/context/answer/apply/setMode/settings
│   ├── provider/        #   OpenAI 兼容 SSE 客户端
│   ├── theme/ icons/    #   面板自己的色板与 SVG（不 import 编辑器）
│   └── main.go          #   mygoembed.Serve(state.View)
└── lightcode/           # 编辑器
    └── internal/app/aiembed.go   # 挂载矩形、帧宿主、输入转发、文件服务
```

依赖方向单向，只有两条：

| 从 | 到 | 用途 |
|---|---|---|
| `lightcode` | `lightcode-ai/protocol` | 只拿消息类型 |
| `lightcode` | `ada_link/{link,mygoembed}` | 会话、帧、输入 |
| `lightcode-ai` | `ada_link/{link,mygoembed}` + MyGo `ui` | 画自己的界面 |
| `lightcode-ai` | 编辑器 | **没有** |

`go list -deps` 可以验证：面板的依赖里不出现 `lightcode/internal/...`，编辑器只
出现 `lightcode.ai/client/protocol`。

## 2. 界面怎么进的编辑器

- 面板调用 `mygoembed.Serve({View: state.View})`：它在自己的进程里用 `ui.Tester`
  离屏渲染，把帧写进两个进程共享的帧环，并把 `link` 的调用/事件接上。
- 编辑器调用 `link.Spawn` 拉起面板、`mygoembed.Attach(win, sess)` 得到 `Host`；
  `Host.View(c)` 画最新帧、按元素矩形向子进程请求同尺寸的帧、并把指针/键盘/文本/
  输入法组合/拖放转发过去。
- 编辑器给出三处**挂载点**，面板按 `Configure.Mode` 画对应形态：

| 形态 | 矩形 | 谁画 |
|---|---|---|
| 停靠 `ModeDock` | 右侧 380 宽，满高 | 面板画标题头、对话、输入 |
| 悬浮 `ModeFloat` | 380×640，位置随 `settings.json` | 同上；标题头抓手可拖动 |
| 收起 `ModeRail` | 48×172 | 面板画收起卡片（图标/展开/新对话） |

拖动由面板发起：抓手把指针位置换算成窗口坐标（`Origin` + 本地坐标），发
`ai.move` 事件；编辑器夹到窗口内、写回 `settings.json`，再回一个 `ai.origin`，
面板据此修正下一帧。

## 3. 消息

信封仍是 `link.Message`（call/reply/event），握手、帧与输入由 `mygoembed` 负责；
`protocol` 只命名载荷：

| 方法/事件 | 方向 | 载荷 | 用途 |
|---|---|---|---|
| `ai.configure` | 编辑器 → 面板 | `Configure{baseURL, model, apiKey, light, mode, x, y}` | 端点、Key、色板、挂载形态与位置 |
| `editor.context` | 面板 → 编辑器 | `Context{messages}` | 系统提示 + 当前文件/选区（≤8000 字符） |
| `editor.answer` | 面板 → 编辑器 | `AnswerRequest{requestID, text}` → `AnswerResult{cards}` | 编辑器解析 diff、读文件，回卡片（旧文/新文） |
| `editor.apply` | 面板 → 编辑器 | `ApplyRequest{requestID, index}` → `ApplyResult{err}` | 把某张卡片写进文件（对当前缓冲区重新应用） |
| `editor.setMode` | 面板 → 编辑器 | `ModeRequest{mode}` | 换挂载点 |
| `ai.settings` | 面板 → 编辑器 | — | 打开编辑器里的端点/Key 弹窗 |
| `ai.move` | 面板 → 编辑器 | `MoveRequest{target, x, y}` | 拖动悬浮/收起卡片 |
| `ai.origin` | 编辑器 → 面板 | `Origin{x, y}` | 挂载点当前位置 |

对话、输入、打字机、diff 卡片、错误与重试都在面板进程里；编辑器只保留
**Key 与文件操作**——Key 存在系统凭据库，只随 `ai.configure` 进面板内存。

## 4. 生命周期

- **懒启动**：第一次显示面板（停靠/悬浮/收起任一）时 `link.Spawn`；只写代码不看
  AI 的会话不付代价。
- **挂载点变化**：`ai.configure` 带 `Mode` 与 `x/y`；面板重画，帧环尺寸由宿主按
  矩形请求（`Host.requestSize`）。
- **进程退出**：`Session.IsClosed` 为真时挂载点画错误卡（「AI 插件进程已退出」）
  与「重试」；重试会重新 `Spawn`。
- **关闭窗口**：`Session.Close` 发 `bye`，宽限 500 ms 后杀。
- **找不到插件**：挂载点直接显示「找不到 AI 面板 …」与构建命令；构建命令见 §6。

## 5. 输入与焦点

- 指针、滚轮、键盘、文本、输入法组合事件都由 `mygoembed.Host` 转发，坐标是面板
  本地 DIPs。
- `Cmd`（Ctrl）组合键留在编辑器：面板是编辑器的一部分，`Ctrl+S` 等仍是编辑器的。
- `Host.View` 被**去掉了 AutoFocus**（并入库里的一处改动）：面板与编辑器共用一个
  窗口，面板出现时抢焦点会让编辑器打不了字；点一下面板才聚焦。

## 6. 构建与运行

```powershell
cd lightcode-ai
go build -o ..\lightcode\lightcode-ai.exe .     # Windows
cd ..\lightcode
go run ./cmd/lightcode
```

`go run` 的可执行文件在临时目录，找不到「同目录」的插件；从 `lightcode/` 启动时
按「当前目录」规则找，或显式指定：

```powershell
$env:LIGHTCODE_AI_PLUGIN = "E:\codes\go_projects\ada_case\lightcode\lightcode-ai.exe"
```

面板支持 `lightcode-ai -version`。

## 7. 测试

| 模块 | 测试 | 证明什么 |
|---|---|---|
| `lightcode-ai/assistant` | `TestViewIntro`、`TestViewRail` | 面板自己的 UI 在两种形态下都画得出来 |
| | `TestSendStreamsAndApplies` | 假编辑器 + httptest 端点：流式回答、取卡片、应用 |
| | `TestDragMovesTheMount` | 拖动换算成窗口坐标（含挂载原点） |
| | `TestModeButtonsAskTheEditor`、`TestNoKeyOpensSettings` | 按钮走桥，不自己改编辑器状态 |
| `lightcode-ai/provider` | SSE 流式/请求体/HTTP 错误/取消 | 网络客户端 |
| `lightcode/internal/app` | `TestAIContextCarriesFile`、`TestAICardsAndApply`、`TestAIMountNeedsAWindow`、`TestAIPanelRow` | 上下文、卡片、应用、缺窗口/缺插件的表现 |
| `ada_link` | `link`、`mygoembed` 原测试 | 帧中继与输入转发未退化 |

真机三张图（编辑器 1363×953）：`_shots/lightcode-ai-panel.png`（停靠）、
`lightcode-ai-float.png`（悬浮）、`lightcode-ai-rail.png`（收起），面板内容全部
来自子进程的帧。

## 8. 限制与后续

- **圆角是方的**：帧是不透明矩形，悬浮/收起卡片的圆角由编辑器画在背后，帧覆盖
  了它；要真正的圆角需要给帧带 alpha 或做形状裁剪。
- **没有悬停预览**：旧的「收起卡片悬停预览上一轮回答」是编辑器画的浮层，面板
  接管界面后这一项没了，需要的话由面板发事件、编辑器画预览。
- **IME**：组合事件已转发，但候选窗位置与真机中文输入仍需人工验证。
- **真实模型未验证**：所有测试打 httptest 端点（没有可用 Key）。
- **代理**：面板用默认 transport，只认 `HTTP(S)_PROXY`；不再读桌面系统代理设置。
- **对话不持久**：对话在面板进程里，进程退出即清空。
- **没有插件清单/签名/打包**：`LIGHTCODE_AI_PLUGIN` 是入口，分发目标未做。
