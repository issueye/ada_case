# lightcode-ai

轻码 IDE 的 AI 面板，**独立项目**：自己的 `go.mod`、自己的二进制、自己的测试，
**连界面也在这里**。它用 MyGo 的 `ui` 画面板，`ada_link` 把帧送进编辑器窗口的
挂载点；它不 import 编辑器的任何包，编辑器只拿它的 `protocol` 契约。

```
assistant/  面板自己的 UI：标题头、对话、输入、diff 卡片、拖动
protocol/   契约：configure/context/answer/apply/setMode/settings 与事件
provider/   OpenAI 兼容的 SSE 客户端
theme/      面板的深/浅色板（与编辑器同一套 token，各自一份）
icons/      面板用到的 SVG
main.go     mygoembed.Serve(assistant.View)
```

## 构建

```powershell
go build -o ..\lightcode\lightcode-ai.exe .   # Windows
go build -o ../lightcode/lightcode-ai .       # macOS / Linux
go build -o lightcode-ai .                    # 只构建，放到任意目录
```

`lightcode-ai -version` 打印名称与版本。

## 协议

帧与输入走 `ada_link`（`mygoembed` 的 `Serve`/`Attach`，帧环 + 指针/键盘转发），
调用与事件走 `ada_link/link` 的消息层。`protocol` 只命名载荷：

| 方法/事件 | 方向 | 载荷 |
|---|---|---|
| `ai.configure` | 编辑器 → 面板 | `Configure{baseURL, model, apiKey, light, mode, x, y}` |
| `editor.context` | 面板 → 编辑器 | `Context{messages}`（系统提示 + 当前文件/选区） |
| `editor.answer` | 面板 → 编辑器 | `AnswerRequest{requestID, text}` → `AnswerResult{cards}` |
| `editor.apply` | 面板 → 编辑器 | `ApplyRequest{requestID, index}` → `ApplyResult{err}` |
| `editor.setMode` / `ai.settings` | 面板 → 编辑器 | 换挂载点 / 打开端点与 Key 弹窗 |
| `ai.move` / `ai.origin` | 双向事件 | 拖动与挂载点位置 |

对话、输入、打字机、diff 卡片都在本进程；编辑器只做文件与凭据。全部说明与取舍见
[`../docs/ai-plugin.md`](../docs/ai-plugin.md)。

## 依赖

标准库 + MyGo（`ui`、`ui/editor`、`ui/markdown`，画自己的界面）+ `ada_link`。
不依赖编辑器、不依赖它的凭据库或工作区；Key 随 `ai.configure` 进来，只在内存里。

代理：HTTP 客户端用默认 transport，认 `HTTP(S)_PROXY` 环境变量；不再读桌面系统的
代理设置——那需要重新引入 UI 框架，与这里的独立性相悖。

## 测试

```powershell
go test ./...
```

- `provider`：SSE 分片、请求体、HTTP 错误、取消；
- `assistant`：两种形态的界面、流式回答 + 卡片 + 应用、拖动换算、按钮走桥、缺 Key。

帧中继与输入转发的真进程端到端由 `ada_link` 的 `mygoembed` 测试与手工截图覆盖
（`_shots/lightcode-ai-panel.png` 等三张）。
