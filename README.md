# ada_case

轻码 IDE（LightCode）的仓库：一个用 Go + [MyGo](https://mygo.run/zh/docs) 原生 UI 写的
轻量代码编辑器，一个独立的 AI 客户端项目，以及并入的进程间库。

<img src="docs/logo.svg" width="80" align="right" alt="LightCode 的 LOGO：深色圆角方底上一对折角与一根蓝紫斜杠">

```
lightcode/     编辑器：窗口、侧栏、编辑、Git、终端、AI 面板
lightcode-ai/  AI 客户端：独立项目、独立二进制（protocol / provider / serve）
ada_link/      进程间库（并入）：link 的消息层被编辑器与插件共用
docs/          设计与计划（dev-plan、mygo-ide-eval、ai-plugin …）
```

一体两面的设计目标是**进程边界清晰**：编辑器不碰网络，插件不碰编辑器，
两者只认 `lightcode-ai/protocol` 这一份契约，消息层由 `ada_link/link` 提供。

## 快速开始

```powershell
# 1. AI 插件（独立项目，先构建它）
cd lightcode-ai
go build -o ..\lightcode\lightcode-ai.exe .

# 2. 编辑器
cd ..\lightcode
go run ./cmd/lightcode
```

编辑器需要本地 MyGo 检出（`lightcode/go.mod` 的 `replace` 指向 `../../mygo`），
Go 1.27+，Windows 10 1809+ / macOS 13+ / Linux（GTK 3）。

## 文档

| 文档 | 内容 |
|---|---|
| [docs/dev-plan.md](docs/dev-plan.md) | 里程碑、验收口径、WBS、进度日志 |
| [docs/ai-plugin.md](docs/ai-plugin.md) | AI 进程插件：`ada_link` 取舍、协议、生命周期、解耦规则 |
| [docs/mygo-ide-eval.md](docs/mygo-ide-eval.md) | MyGo 做 IDE 的技术评估 |
| [lightcode/README.md](lightcode/README.md) | 编辑器的功能、结构、约定与测试 |
| [lightcode-ai/README.md](lightcode-ai/README.md) | 插件项目的构建与协议摘要 |

## 测试

```powershell
cd lightcode     ; go test ./...   # 编辑器：108 个
cd ..\lightcode-ai; go test ./...   # 插件：9 个
cd ..\ada_link  ; go test ./...     # 并入库：link / mygoembed / panels
```

![主窗](_shots/lightcode-m0.png)
