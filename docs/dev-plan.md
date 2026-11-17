# 轻码 IDE · 开发计划

> 依据：[技术栈评估](mygo-ide-eval.md)（原生 UI 主路线）与 [Mayoi 参考笔记](mayoi-reference.md)。
> 设计规格：画布项目「轻码 IDE · 界面设计」16 张稿（`http://127.0.0.1:8788`），设计体系 `ide`（主色 `#4E8DF5`）。
> 代码位置：`E:\codes\go_projects\ada_case\lightcode`，模块名 `lightcode`。

## 0. 目标与范围

**一句话**：用 Go + MyGo 原生 UI 做一个「够用的轻量 IDE」—— VS Code 的骨架与手感，裁剪掉远程开发、插件市场、PR/代码评审、多主题，AI 助手以「只产出 diff、人工确认」的方式做成一等公民。

### v1 范围内

| # | 能力 | 验收口径 |
|---|---|---|
| F1 | 打开文件夹、资源管理器树、懒加载、文件图标 | 30k 文件仓库可展开、可打开、滚动不卡 |
| F2 | 多标签编辑器（脏标记、关闭确认、预览标签） | 打开 20 个文件，切换 < 50 ms |
| F3 | 编辑：语法高亮、多光标、折叠、查找替换、跳转到行、面包屑 | 与 `ui/editor` 能力对齐，不另造 |
| F4 | 快速打开（`Ctrl+P` 模糊）、命令面板（`Ctrl+Shift+P`） | 30k 文件首屏 < 120 ms |
| F5 | 跨文件搜索（ripgrep，缺失时 Go 遍历） | 30k 仓库首条结果 < 800 ms |
| F6 | SCM：三组状态、提交框、逐 hunk 暂存/丢弃、diff 页、分支与历史 | hunk 暂存成功率 100%，status < 1 s |
| F7 | 合并与冲突：三态提示、三方只读视图、冲突解决并标记 resolved | 冲突文件可完成合并并提交 |
| F8 | AI 面板：停靠 / 浮标 / 悬浮三态，流式回复，diff 卡片，应用需确认 | 流式期间编辑器可交互，取消 < 100 ms |
| F9 | 终端：Ghostty 分栏，`path:line:col` 链接回编辑器 | 全屏程序正确，键位归属正确 |
| F10 | 设置（JSON + 表单）、状态持久化、能力面板（扩展页） | 重启恢复布局/会话，schema 可迁移 |
| F11 | 状态栏：分支、诊断数、位置、编码、行尾、语言、AI 状态 | 与设计稿分段一致 |

### 明确不做（v1）

- 远程开发 / SSH / 容器、GitHub 集成、PR 评审、账号体系。
- 用户可安装的插件运行时（扩展页只做能力开关 + 只读 provider 列表）。
- Vim 模式、多主题、Markdown Live、多根工作区、LSP 之外的代码智能（重构/重命名 v2 再说）。
- **AI 内联建议**：不做 ghost text；AI 只产出 diff，应用前必须确认。

## 1. 架构

```
cmd/lightcode            # 装配：App.Run、窗口、菜单、打开目录参数
internal/
  app/                   # 视图：chrome + 各面板（唯一持有 UI 状态的一层）
    app.go               # App 结构、Update 规则、快捷键注册
    layout.go            # 主窗骨架：标题栏/活动栏/侧栏/编辑区/状态栏
    titlebar.go          # 40 px 自绘标题栏 + 窗口控制 + 命令中心
    activity.go          # 48 px 活动栏（5 图标 + 徽标）
    explorer.go          # 资源管理器树（懒加载 + 图标 + 折叠）
    tabs.go              # 标签条（脏点、关闭、溢出、预览标签）
    breadcrumb.go        # 28 px 面包屑
    statusbar.go         # 24 px 状态栏（分段 + 悬停）
    palette.go           # 命令面板 / 快速打开
    searchpane.go        # 搜索面板
    scm.go               # 源代码管理面板 + 提交框
    diffpage.go          # diff 页（统一/并排）
    ai.go                # AI 三态：停靠 380 / 浮标 48×172 / 悬浮 380×640
    terminal.go          # 底部终端分栏
    settings.go          # 设置表单
    capabilities.go      # 扩展（能力面板）
  chrome/                # 无业务的外壳原语：IconButton、ListRow、SectionHeader、Split
  theme/                 # 设计 token → *ui.Theme；编辑器 *editor.Theme
  command/               # 命令注册表 + 键位表（菜单/palette/测试共用）
  keymap/                # VS Code 风格键位解析与优先级
  document/              # Document / Editor / Column 三层；磁盘状态；保存与外部改动
  workspace/             # 文件夹索引、懒加载目录、模糊匹配、文件操作、watcher
  git/                   # CLI 封装：status/log/diff/hunk 暂存/分支/合并/冲突
  search/                # ripgrep 或 Go 回退
  ai/                    # provider 适配、流式、上下文预算、diff 生成
  settings/              # settings.json / state.json、schema、迁移
  lsp/                   # 语言服务进程与 JSON-RPC（M5）
  icons/                 # 内嵌 SVG（不用 emoji，不用字符当图标）
```

### 1.1 分层与数据流

- **Document**：路径、`*editor.Buffer`、磁盘 mtime/hash、编码、行尾、只读标记。**唯一**拥有文本与 undo。
- **Editor**：视图状态（光标、滚动、折叠、选区、查找栏），一个 Document 可被多个 Editor（分屏）共享。
- **Column**：一个编辑列（标签条 + 面包屑 + Editor），决定当前标签。
- 视图**只读快照**：`buffer.Snapshot()`；慢操作（打开目录、git、搜索、AI）在 goroutine，完成后 `win.Update(func(){ ... })` 回写。
- 视图里禁止 I/O；禁止在 `Update` 里长跑。
- 帧预算：空闲 < 2 ms、滚动 < 5 ms、打字 < 4 ms（`go test ./internal/app -run Budget`）。

### 1.2 命令与键位（先建地基）

```go
type Command struct {
    ID    string   // "file.save"
    Title string   // "保存文件"
    Key   string   // "CmdOrCtrl+S"
    When  string   // "editorFocus && dirty"（简化版 when 表达式）
    Run   func(*app.App)
}
```

- 菜单、命令面板、快捷键、测试全部由 `command.Registry` 生成，禁止各面板私接快捷键。
- 优先级：**终端 > 编辑器 > 命令面板/浮层 > 外壳默认**；谁先吃键谁 `take`，未处理才下沉。
- 键位文件用 VS Code 的 `keybindings.json` 格式（`key` / `command` / `when`），M1 支持用户在设置里追加。

## 2. 里程碑

| 里程碑 | 内容 | 预估 | 验收（可执行） |
|---|---|---|---|
| **M0 地基与空壳** | 模块、token 主题、命令表、键位、标题栏/活动栏/侧栏/标签/编辑器/状态栏、打开文件夹、保存、命令面板 | 8 人日 | `go build` 通过；冷启 < 400 ms；空闲 < 90 MB；`CapturePage` 出图与主窗稿并排 ≥ 85% |
| **M1 编辑体验** | 多标签与脏状态、关闭确认、面包屑、查找/跳转、IME、缩放、会话恢复、外部改动、编码 | 8 人日 | 20 标签切换 < 50 ms；中文不丢字；重开恢复布局 |
| **M2 工作区** | 懒加载树 + 文件图标、`Ctrl+P` 模糊、搜索面板、大纲、新建/重命名/删除、watcher 刷新 | 10 人日 | 30k 仓库展开 < 300 ms；首条搜索结果 < 800 ms；外部改动 < 500 ms 可见 |
| **M3 Git** | status 三组、提交、diff 页（统一/并排）、逐 hunk 暂存/丢弃、分支、历史、pull/push（不自动）、合并与冲突 | 18 人日 | status < 1 s（30k）；hunk 暂存 100%；冲突可完成并提交 |
| **M4 AI 面板** | 三态切换与浮标、流式 markdown、diff 卡片、应用确认、取消/重试、Key 存 `mygo.Secrets`、上下文预算 | 10 人日 | 流式不卡；取消 < 100 ms；失败不留半成品 |
| **M5 终端 / 设置 / 能力面板** | Ghostty 分栏与键位、设置表单与 schema、扩展能力开关、i18n | 8 人日 | 全屏程序正确；设置可迁移；中英文案齐 |
| **M6 打磨与发布** | 预算测试、截图回归、a11y、打包（msi/dmg/AppImage）、崩溃恢复 | 8 人日 | CI 绿；干净机器可装可升 |

合计约 **70 人日**（3 人并行 ≈ 5–6 周），与评估里 12–18 人周的口径一致（余量留给未知）。

### 2.1 本轮（M0）任务清单

| # | 任务 | 交付物 | 状态 |
|---|---|---|---|
| M0-1 | 模块与依赖 | `lightcode/go.mod`（`replace` 指本地 MyGo 仓库）、`cmd/lightcode/main.go` | ✅ |
| M0-2 | 主题 token | `internal/theme/theme.go`（`#0f1216` / `#14181e` / `#5b9cff` / `#a78bfa`、圆角 4/6/8、13 px 正文） | ✅ |
| M0-3 | 命令注册表 | `internal/command/registry.go` + `keymap.go`（ID/标题/键位/Run） | ✅ |
| M0-4 | 窗口与标题栏 | 隐藏系统标题栏 + 自绘 40 px（拖拽、最小化/最大化/关闭、命令中心） | ✅ |
| M0-5 | 活动栏 + 侧栏 | 48 px 活动栏 5 图标；240 px 资源管理器（真实目录、懒加载） | ✅ |
| M0-6 | 标签条 + 编辑器 | 36 px 标签（脏点/关闭）、28 px 面包屑、`editor.View` | ✅ |
| M0-7 | 状态栏 | 24 px 分段（分支/诊断/位置/编码/行尾/语言） | ✅ |
| M0-8 | 命令面板 | `Ctrl+P` 文件、`Ctrl+Shift+P` 命令，模糊匹配 | ✅ |
| M0-9 | 文件读写 | 打开/保存、`Ctrl+S`、外部改动检测（mtime，重开时自动重载或提示） | ✅ |
| M0-10 | 验证 | `go build ./...`、`go vet`、`go test ./...`、启动/内存/帧预算测量、截图 | ✅ |

#### M0 验收结果（2026-11-17，Windows 11 / Go 1.27.1）

| 指标 | 目标 | 实测 | 证据 |
|---|---|---|---|
| 冷启动到窗 | < 400 ms | **78 ms** | `Start-Process` + 轮询 `MainWindowHandle` |
| 工作集 / 私有内存 | < 90 MB | **63.4 MB / 76.4 MB** | 窗口空转 3 s 后 |
| 帧时间（2,000 行 Go 文件） | 空闲 < 2 ms | **0.29 ms**（20 帧均值） | `internal/app/budget_test.go` |
| 构建 / 静态检查 / 测试 | 全绿 | `go build`、`go vet`、`go test ./...` 全绿 | 6 个包 28 个用例（M1 后） |
| 界面 | 与主窗稿并排 ≥ 85% | 标题栏/活动栏/侧栏/标签/面包屑/编辑器/状态栏/AI 停靠均已就位 | `_shots/lightcode-m0.png`、`_shots/lightcode-ui-*.png` |

已知差距（M1、M2 已完成，M3 核心完成，M4 完成）：M1-1～M1-6、M1-8 与 M2-1～M2-5 已落地并测试；M1-7 的中文输入在无头测试通过，候选窗与真机 watcher 仍需人工确认。M3 的 status/SCM/diff/逐块暂存/提交/历史已完成；冲突三方视图、逐块丢弃、分支切换、pull/push 仍待做。M4 的流式客户端、三态面板、diff 卡片与确认应用已完成；真实模型调用未验证（本机无 Key），历史持久化与浮标拖拽未做。扩展页仍是占位。

> 状态在每轮结束时更新；未完成的项记 ⬜，完成的记 ✅。

### 2.2 M1 开工清单（进度）

| # | 任务 | 验收 | 状态 |
|---|---|---|---|
| M1-1 | 会话恢复：`state.json`（打开的文件、活动标签、侧栏显隐/视图、AI 三态、缩放） | 重启回到上次布局，损坏/旧 schema 不崩 | ✅ `internal/settings` + `session_test.go` |
| M1-2 | 外部改动：1.5 s 轮询打开的文件；干净文件自动重载，脏文件弹条「重新加载 / 保留我的」 | 终端改文件后 2 s 内可见，不误报自身保存 | ✅ `checkDisk` + `TestExternalChangeBanner` |
| M1-3 | 预览标签：树里单击打开斜体预览，替换上一个干净预览；编辑即固定，双击/面板打开为固定 | 20 个预览不撑爆标签条 | ✅ `Column.OpenPreview` + `TestPreviewTab` |
| M1-4 | 分屏：`ui.SplitPercent`（可拖分隔条）、`Ctrl+\` 切换、两列共享同一 Document | 两列共享 undo，关闭分屏不丢标签 | ✅ `split_test.go`（共享/键位/会话/折叠） |
| M1-5 | 缩放：`Ctrl+= / Ctrl+- / Ctrl+0` 走 `c.SetZoom`，持久化 | 125% 下字体与列宽正确、截图不糊 | ✅ `SetZoom` + `TestZoomCommand` |
| M1-6 | 编码：UTF-8/UTF-8 BOM/GBK 探测与回写，二进制拒绝 | 往返无损 | ✅ `decode/encode` + GBK/BOM 用例 |
| M1-7 | IME：中文/日文输入法一轮，候选窗贴光标 | 0 丢字、组合不跳 | ⚠️ 无头中文输入通过（`TestChineseTyping`）；候选窗待真机 |
| M1-8 | 截图回归基线：`tt.Image()` 与 `testdata/shots/editor.png` 比像素（1%） | CI 可判定 | ✅ `baseline_test.go` |

### 2.3 M2 开工清单（进度）

| # | 任务 | 验收 | 状态 |
|---|---|---|---|
| M2-1 | 索引与快速打开：打开文件夹时建索引（30k 37–213 ms），watch/文件操作后异步重建，`Ctrl+P` 直接用索引 | 30k 首屏 < 120 ms | ✅ `internal/workspace/index.go` |
| M2-2 | 跨文件搜索：PATH 有 `rg` 时用 ripgrep（`--vimgrep` 解析、流式），否则纯 Go 并行 + 流式（30k 首条 7.5 ms、全量 924 ms）；字面量/正则/大小写、500 上限、可取消 | 30k 首条 < 800 ms | ✅ `internal/search`（`rg.go` 解析/回退有用例；本机未装 rg，实测走 Go 路径） |
| M2-3 | 大纲：Go 用 `go/parser`，Markdown 用标题，其它语言按行首声明；侧栏下分栏（可拖），点击跳转，>240 条折叠显示 | 符号行号与跳转正确 | ✅ `internal/symbols` + `outline.go` |
| M2-4 | 文件操作：新建文件/文件夹、重命名、删除（确认、脏文件先拦）、右键菜单、操作后刷新树与索引 | 新建/改名/删除可用且不丢标签 | ✅ `fileops.go` + `TestFileOperations` |
| M2-5 | watcher：`mygo.WatchFiles`（`HintsOnly`、120/600 ms 防抖、忽略目录）刷新树并异步重建索引；`ErrWatchLimit` 或不可用时回退 1.5 s 轮询 | 外部改动 < 500 ms 可见 | ✅ `watch.go`；真机验证：应用运行中创建 `watched-new.txt`，5 s 截图里树已出现（`_shots/lightcode-watch.png`） |

> 30k 样本：`_shots/bench-repo`（100 目录 × 300 文件，生成后删除）；测法见 `internal/{workspace,search}` 的 `*LargeTree` 测试（`LIGHTCODE_BENCH=1`）。

### 2.4 M3 开工清单（进度）

| # | 任务 | 验收 | 状态 |
|---|---|---|---|
| M3-1 | Git CLI 封装：`status --porcelain=v2 -z` 解析（重命名/未跟踪/冲突/分支 ahead-behind）、diff、索引文本、log | 30k 仓库 status < 1 s | ✅ `internal/git` + 解析/暂存用例 |
| M3-2 | SCM 面板：暂存/更改/未跟踪/冲突四组、文件行状态、提交框、最近提交、右键暂存/取消暂存/丢弃 | 与设计稿三组一致 | ✅ `scm.go`、`TestSCMPanel` |
| M3-3 | diff 页：`editor.DiffState` 并排视图、上下文折叠、块卡片 | 打开/刷新正确 | ✅ `diffpage.go`、`TestDiffPageAndStageHunk` |
| M3-4 | 逐 hunk 暂存：`git diff` → `editor.ParsePatch` → 单块 `Apply` 到索引文本 → `hash-object -w` + `update-index --cacheinfo`，不拼 patch | 暂存成功率 100%、另一块不泄漏 | ✅ `TestStageHunk`（索引第 25 行变了、第 3 行没变） |
| M3-5 | 分支与历史：当前分支、ahead/behind、最近 20 提交列表 | 显示正确 | ✅ `scmHistory`、`TestCommitAndLog` |
| M3-6 | 合并与冲突：状态里的冲突组已显示；三方视图与解决标记 | 可完成合并并提交 | ⬜ |
| M3-7 | pull/push/fetch：凭证（`GIT_TERMINAL_PROMPT=0`）、被拒与分叉提示 | 不自动推送 | ⬜ |

> 丢弃整文件有二次确认；逐块丢弃尚未做。分支切换 / 检出未做。

### 2.5 M4 开工清单（进度）

| # | 任务 | 验收 | 状态 |
|---|---|---|---|
| M4-1 | OpenAI 兼容流式客户端：SSE 解析、错误体、系统代理、取消；Key 从 `mygo.Secrets` / `OPENAI_API_KEY` | 流式不卡、取消 < 100 ms | ✅ `internal/ai` + SSE 用例（含取消、HTTP 错误、请求体） |
| M4-2 | 三态面板：停靠 380 / 悬浮 380×640 / 收起 62 px 栏位卡片（悬停预览上一轮回答）；模式随会话恢复 | 三态可切、不遮编辑器 | ✅ `ai.go`、`TestAIFloatOverlay`、`TestAIRailHoverPeek`、`TestAIModePersists` |
| M4-3 | 流式 markdown + diff 卡片：`editor.ParsePatch` 解析回答，卡片展开 `DiffView`，点「应用」才写入缓冲区，可「忽略」；取消/重试 | 流式期间可交互、失败不留半成品 | ✅ `TestAIStreamsAndAppliesDiff`、`TestAIRetryResends` |
| M4-4 | 上下文：系统提示（只产 diff）、当前文件/选区、8000 字符上限、上下文 chip | 大文件不爆 token | ✅ `aiPrompt`（无精确 token 计数） |
| M4-5 | 设置：Base URL / 模型 / Key 弹窗；Key 存 Secrets，Base/模型进 `state.json`（schema 3） | 重启保留配置，Key 不进明文 | ✅ `aiSettingsOverlay`、`TestAISettingsPersist` |

> 未做：浮标拖拽与悬停展开、对话历史持久化、应用后的一键撤销（当前写入缓冲区、保持 dirty，可再用 Git 丢弃）、精确 token 预算。

## 3. WBS（按模块，含依赖）

| ID | 模块 | 任务 | 人日 | 依赖 |
|---|---|---|---|---|
| W1 | 地基 | 命令表 + 键位解析 + 菜单生成 | 4 | — |
| W2 | 地基 | Document/Editor/Column 三层 + 保存/外部改动 | 4 | W1 |
| W3 | 外壳 | 标题栏（拖拽、窗口控制、命令中心） | 3 | W1 |
| W4 | 外壳 | 活动栏 + 侧栏容器 + 树（懒加载/图标/键盘） | 5 | W2 |
| W5 | 外壳 | 标签条（脏/关闭/溢出/预览/快捷键） | 3 | W2 |
| W6 | 外壳 | 状态栏 + 面包屑 + toast/右键菜单 | 3 | W2 |
| W7 | 工作区 | 文件夹索引 + 模糊匹配 + quick open | 4 | W4 |
| W8 | 工作区 | 搜索面板（rg 打包 + Go 回退 + 取消） | 4 | W7 |
| W9 | 工作区 | 大纲 / 符号 | 2 | W4 |
| W10 | 工作区 | 文件操作 + watcher | 3 | W4 |
| W11 | Git | status/log/diff 解析与刷新节流 | 4 | W2 |
| W12 | Git | SCM 面板 + 提交框 + 分支/历史页 | 4 | W11 |
| W13 | Git | diff 页（统一/并排/EOL/二进制/大文件降级） | 4 | W11 |
| W14 | Git | hunk 暂存/丢弃（hash-object + update-index） | 3 | W13 |
| W15 | Git | 合并/冲突三方视图 + resolved 标记 | 4 | W13 |
| W16 | Git | pull/push/fetch、凭证、被拒与分叉提示 | 2 | W11 |
| W17 | AI | provider + 流式 + 上下文预算 | 4 | W2 |
| W18 | AI | 三态面板 + 浮标吸附 + 悬浮卡 | 3 | W17 |
| W19 | AI | diff 卡片 + 应用/回滚 + 取消/重试 | 3 | W18, W14 |
| W20 | 终端 | Ghostty 分栏 + 键位优先级 + 链接回编辑 | 3 | W1 |
| W21 | 设置 | settings.json + schema/迁移 + 表单 | 4 | W1 |
| W22 | 设置 | 状态持久化 + 会话恢复 + 窗口 StateKey | 2 | W21 |
| W23 | 发布 | 预算测试 + 截图回归 + CI | 4 | 全部 |
| W24 | 发布 | 打包/签名/更新 + a11y + i18n | 4 | 全部 |

## 4. 关键契约（写进代码注释，评审按此查）

1. **命令唯一入口**：任何用户动作 = 一条命令；面板只调用命令，不自己实现逻辑。
2. **文本唯一所有者**：只有 Document 持有 Buffer；Editor 只持视图状态；保存走 Document，`buffer.Modified()` 是脏的唯一判据。
3. **快照只读**：跨 goroutine 读文本用 `Snapshot()`，不读 `Buffer.String()` 之外的活对象。
4. **Git 不拼 patch**：hunk 暂存 = 索引文本与缓冲区的行级 diff → 取重叠 hunk → `git hash-object` + `git update-index`。
5. **不自动**：合并、提交、推送永不自动；危险操作二次确认（丢弃改动、强推、删除分支）。
6. **AI 只产 diff**：`FilePatch.Apply` 前必须经过确认；应用到脏缓冲区失败要整体回滚。
7. **图标只用 SVG**：`internal/icons` 内嵌，禁止 emoji 与 ▾ › ✕ 字符。
8. **主题只用 token**：颜色/圆角/间距/动效从 `internal/theme` 取，不在视图里写魔法值。

## 5. 视觉规格（来自 16 张稿）

| 区域 | 尺寸 | 备注 |
|---|---|---|
| 标题栏 | 40 | 左：应用菜单；中：命令中心；右：窗口控制 |
| 活动栏 | 48 | 5 图标（资源管理器/搜索/源代码管理/运行/扩展）+ 底部设置 |
| 侧栏 | 240（可拖 180–360） | 分节标题 28，行高 24 |
| 编辑区 | 余量（AI 收起 772→1152） | 标签 36、面包屑 28 |
| AI 停靠 | 380 | 收起为右侧 62 px 独立栏位里的 48×172 卡片（不遮挡编辑器） |
| 状态栏 | 24 | 分支/诊断/位置/编码/行尾/语言 |
| 终端 | 200（折叠 32） | 底部 |

token（深色）：`--bg #0f1216`、`--panel #14181e`、`--ac #5b9cff`、`--ai #a78bfa`、边框 `#242a33`、次要文字 `#8b95a5`；正文 13 px，标题 13 px/600，行高 1.5；编辑器 JetBrains Mono 13，行高 1.5。

## 6. 测试与验收

- **单元**：已有 `internal/document`（保存/CRLF/二进制/外部改动）、`internal/workspace`（排序/懒加载/ignore/模糊匹配）、`internal/command`（键位解析/注册表）、`internal/app`（界面文本、快速打开、侧栏与 AI 切换、保存、脏关闭确认）。
- **交互**：用 `ui.NewTester` 无头跑真视图：`TestQuickOpenOpensFile`、`TestSaveDirtyDocument`、`TestCloseDirtyAsksToSave`。
- **预算**：`internal/app/budget_test.go` 打开 2,000 行文件测 20 帧，断言 < 8 ms（实测 0.29 ms/帧），收紧后再按 2/5/4 ms 分项。
- **截图回归**：`internal/app/screenshot_test.go` 用 `tt.Image()` 输出 `_shots/lightcode-ui-*.png`；像素比对基线待 M1 接入（阈值 1%）。
- **窗口截图**：`LIGHTCODE_CAPTURE=<png 路径>` 启动后 1.2 s 用 `CapturePage` 出图并退出（`_shots/lightcode-m0.png` 即由此生成）。
- **手工清单**：打开 30k 仓库 → 打开 20 文件 → 编辑保存 → 切 AI 三态 → 暂存 hunk → 冲突合并 → 终端跑 `go test`。

## 7. 风险与对策（承接评估第 7 节）

| 风险 | 对策 |
|---|---|
| 外壳工作量被低估 | 每件外壳按 W3–W6 估算；先做 M0 骨架再谈细节 |
| 原生绘制保真 | 主题 token 集中；主窗先做 1:1；用 `Shadow`/`Blur`/`Cache` 补质感 |
| MyGo API 变动 | `replace` 锁本地 commit；升级前跑截图回归；patch 清单 |
| 三方合并需自研 | 四缓冲（base/ours/theirs/result）+ 两次 `DiffView`，先只读 |
| 大仓库 watcher 上限 | `HintsOnly` + 目录级 watcher + 定期全量对账 |
| 终端抬系统版本 | 接受 macOS 13+；低版本隐藏入口 |

## 8. 每轮工作方式

1. 从本计划取下一个未完成里程碑，拆成 ≤ 半天的任务。
2. 先写测试/验收脚本，再实现；`go build ./... && go vet ./... && go test ./...` 必须绿。
3. 每轮结束：更新 §2.1 状态、补一张截图、把「实际 vs 计划」记到 §9。
4. 计划变更（范围/顺序/估算）只改本文档，并在 §9 记一行原因。

## 9. 进度日志

| 日期 | 里程碑 | 实际 | 差异 / 备注 |
|---|---|---|---|
| 2026-11-17 | M0 | 模块、主题、命令表、窗口空壳、资源管理器、标签、编辑器、状态栏、命令面板、保存 | 与计划一致；外壳用 `ui` 原语自绘，未引入第三方组件库 |
| 2026-11-17 | M0 验收 | 冷启 78 ms；空闲 63.4 MB；帧 0.29 ms；`go build/vet/test` 全绿；出图 4 张 | 全部达到 §2.1 的目标；差距见该节末尾，M1 清单见 §2.2 |
| 2026-11-17 | M1（完成） | 会话恢复、外部改动提示、预览标签、缩放、分屏、GBK/BOM 编码、截图基线 | 新增 `internal/settings`；分屏用 `ui.SplitPercent`；测试 33 个全绿；IME 仅无头验证 |
| 2026-11-17 | M2（完成） | 索引、搜索面板（rg + Go 回退）、大纲、文件操作、watcher | 30k 实测：索引 37–213 ms、搜索首条 7.5 ms/全量 924 ms；测试 54 个全绿；真机 watcher 已验证（运行中新建文件可见） |
| 2026-11-17 | M3（核心完成） | status/SCM 面板、diff 页、逐 hunk 暂存、提交、历史 | 测试 64 个全绿；hunk 暂存用 `hash-object` + `update-index`（用例验证只动选中的块）；冲突/三方与 remote 操作待做 |
| 2026-11-17 | M4（完成） | OpenAI 流式客户端、三态 AI 面板、流式 markdown、diff 卡片与确认应用、取消/重试、Key 入 Secrets | 测试 81 个全绿；AI 客户端的 SSE/取消用 httptest 验证；应用 diff 有测试；真机调用真实模型未验证（无 Key） |
| 2026-11-17 | M1 准备 | 会话恢复、外部改动、预览标签、分屏、缩放、编码、IME、截图基线 | 下一轮按 §2.2 顺序做，先 M1-1/M1-2（状态与文件一致性最影响可信度） |


