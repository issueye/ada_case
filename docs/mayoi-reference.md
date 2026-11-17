# Mayoi 参考笔记：一个真实跑起来的 Go + MyGo IDE

> 本地路径：`E:\codes\go_projects\Mayoi`，v0.22.2（commit `e2b49b7`，2026-11-17），MIT。
> 定位：小、快的代码编辑器，**纯原生 UI**（无 webview、无 Electron），作者 sorrycc（也是 MyGo 的 fork 维护者）。

## 0. 规模（用来校准我们的排期）

| 指标 | 值 |
|---|---|
| 打包体积 | 约 18 MB（README） |
| `cmd/mayoi` 非测试代码 | **84,636 行 / 184 个文件** |
| `cmd/mayoi` 测试代码 | 45,370 行 / 157 个文件 |
| `internal/*` 非测试代码 | 21,944 行 / 79 个文件 |
| 合计非测试 | **约 106,000 行** |
| 依赖 | `egoist/mygo`（被 `replace` 成 `sorrycc/mygo` fork）、`chroma`、`fsnotify`、`goldmark`、`moejs`、`purego` |
| 最低系统 | macOS 13（终端插件 libghostty-vt 要求） |

结论先说：**一个「VS Code 同款外壳」级别的 IDE，在 Go + MyGo 上大约就是 10 万行**。我们那 16 张稿砍掉 SSH、GitHub、插件引擎、Vim、Markdown Live、多主题之后，量级大约 2–2.5 万行，其中外壳（chrome）占大头。

## 1. 架构分区

```
cmd/mayoi/          main 包：窗口、项目、命令、视图、编辑、diff、SCM、搜索、面板、设置 UI、AI、主题
internal/
  git/              git CLI 封装（status/blame/stage/push/…）
  gitmarks/         HEAD 行级比较，产出 gutter 标记
  linediff/         Myers 行级 diff
  highlight/        chroma 后台词法（增量）
  workspace/        目录读取 + fsnotify + 文件索引 + 模糊匹配 + glob
  vfs/              本地/远程统一文件系统（os 同名函数）
  remote/           SSH：master 连接、脚本协议、inotifywait/轮询
  lsp/              JSON-RPC 语言服务器客户端
  keymap/           VS Code 格式键位解析
  settings/         settings.json + state.json + 监听外部修改
  symbols/          符号索引
  plugin/           moejs 插件运行时（清单驱动、看门狗）
  agents/ mon/ cloudsync/ usage/ cli/
```

主包内部的分层（对我们要建的 `chrome` 很有参考价值）：

| 文件 | 行数 | 职责 |
|---|---|---|
| `view.go` | 1,811 | 布局：标题栏 / 侧栏 / 列 / 面板 / 状态栏，稳定 key |
| `projects.go` | 2,010 | 窗口 ↔ 项目（project tabs 的左侧 rail） |
| `app.go` | 985 | 项目级状态 |
| `commands.go` | 844 | 命令注册表（菜单/快捷键/面板/测试共用） |
| `columns.go` | 1,200 | 列（分屏）与 tab 归属、拖拽 |
| `editor.go` | 1,104 | 编辑器视图（TextAreaBufferBase + 装饰） |
| `scm.go` / `scmpanels.go` | 2,392 / 863 | 源代码管理视图 |
| `diffview.go` / `diffpaint.go` / `diffrows.go` | 2,831 / 1,810 / 274 | 自绘 diff |
| `panel.go` | 728 | 底部面板（终端/问题/输出） |
| `sidebar.go` | 919 | 侧栏 |
| `quickopen.go` / `commandpalette.go` | 538 / 354 | 快速打开 / 命令面板 |
| `prefs*.go` | 约 4,000 | 设置窗口 |
| `theme.go` | 1,690 | 主题（多套 + 吉祥物 + 数字雨） |

## 2. 可直接照搬的 12 条

### 2.1 窗口选项照抄一份
`projects.go:80`：

```go
opts := mygo.WindowOptions{
    Title: "Mayoi", Width: 1280, Height: 820, MinWidth: 640, MinHeight: 420,
    StateKey:             key,                    // 窗口位置/大小记忆，按项目区分
    TitleBarStyle:        mygo.TitleBarHidden,
    TitleBarHeight:       titleBarH,              // 38
    TrafficLightPosition: &mygo.Point{X: 14, Y: 13},
    BackgroundColor:      "#131317",             // 页面未画前的底色，避免启动闪白
    Content:              ui.View(f.view),
}
```

他们的尺寸常量（`view.go:29`）：`titleBar 38 / tabStrip 35 / crumbs 24 / statusBar 24 / panelHead 34` —— 和我们的设计稿（40/36/28/24）几乎一致，说明这套密度是合理的社区实践。

### 2.2 Document / Editor / Column 三层
- **Document** = 文本 + undo 历史 + Git 标记，被多个列/编辑器共享。
- **Editor** = 某个 tab 里的视图状态（选区、滚动、绘制缓存）。
- **Column** = tab 容器（editor 或 terminal）。
- 好处：同一文件在两个分屏里，撤销历史与 Git 标记是同一份；tab 换列不丢状态。

`document.go` / `columns.go`。我们设计稿里的「标签页 + 分屏」必须照这个模型，否则 undo 会分裂。

### 2.3 预览 tab（transient）
Explorer / 搜索打开文件走 `previewFile`：复用已有 tab，否则占用「预览 tab」的位置；一旦编辑、双击 tab、拖到别的列，就转正。`recentpicker.go` 只记转正过的。设计稿里的「单击预览、双击固定」直接对应。

### 2.4 命令注册表（一份命令，五处使用）
菜单项、快捷键、命令面板、测试、`mayoi run` 全走 `commands.go` 的同一张表；命令带 VS Code 格式的 id 与标题，`runCommands` 一帧一步地串行执行（等编辑/Git 落地）。插件命令后加入同一张表。

**这是外壳的地基**：先建命令系统，再建面板。

### 2.5 键位优先级与和弦
`keymap.go`：任何自己吃键的地方（编辑器、终端 `OnKey`、Explorer、diff、图片、输入框）都先问 `takeKeys`，所以和弦的第二键不会先被输入；焦点不在任何地方时，窗口把键位注册成窗口快捷键。键位文件用 VS Code 格式（带注释），被监听、热加载。

### 2.6 Git：只调 CLI，不引库
`internal/git`：`git rev-parse` 找工作树、`status --porcelain=v2`、stage/unstage/discard/commit/blame/switch/fetch/pull/push。**所有命令都带 `GIT_TERMINAL_PROMPT=0` + SSH `BatchMode`**，需要密码时直接失败而不是挂起。

状态刷新策略（`git.go` / `scm.go`）：
- 树变化、保存后、Git 命令后、窗口聚焦、以及每 5 s 轮询；
- 相同的 status 直接丢弃（不让 UI 重建）；
- 每 1.5 s 检查 `index` / `HEAD` / `FETCH_HEAD` 的 mtime —— 终端里 `git commit` 也能被发现。

### 2.7 Hunk 暂存不拼 patch（重要，替换我们原方案）
`hunks.go` 的做法：

> 「索引可能已经和 HEAD 不同，所以不能把 HEAD 的 hunk 直接应用到索引。于是把**索引文本**和**编辑器文本**再 diff 一次，取与这次改动重叠的 hunk，应用得到结果，用 `git hash-object` + `git update-index` 写回索引。没有 patch 可拼，也就没有 fuzz 可出错。」

我们原方案是「生成 unified diff → `git apply --cached`」，在 CRLF / 上下文漂移时最容易失败。**改用 Mayoi 的算法**：读 `git show :path`（索引文本）→ 与缓冲区做行级 diff（他们已有 Myers）→ 取重叠 hunk 合并 → 写 blob、更新索引。

### 2.8 行级 diff 复用三处
`internal/linediff`（Myers）+ `internal/gitmarks`（`git show HEAD:path` 后比较）：一次比较供 **gutter 标记**、**改动预览（peek）**、**diff tab** 三处共用。100,000 行文件比较约 0.25 ms，编辑后后台重算。

### 2.9 Diff tab 为什么自绘（对我们是个决策点）
`diffview.go` 顶部写明理由：并排视图需要在缺行的一侧留空白行，而文本区做不到，所以**自己按行绘制、只画视口内**；每侧独立高亮，词级差异只算视口内；工作区那侧可编辑，且与文件编辑器**共享同一份 undo 与保存状态**（`diffedit.go`）。

其他可抄的工程点（`diffrows.go`）：
- 一个 diff tab 装下 status 里的所有文件，**4 个文件并发读取**，只有「视口 ± 一屏」的才读；
- 折叠的空白段以**新侧起始行号**为 key，打字时保持展开状态；
- 行可能因换行占多行，所以缓存每行顶部、二分查找命中行；
- 上方文件异步读入变高时，保持当前选中行（否则顶部行）的屏幕位置。

我们的 0.9.2 有 `editor.DiffView`（声称双侧对齐、可编辑新侧），**先用它**；只有在「一个 tab 装全仓库改动 + 增量读取」这种场景才需要抄 `diffrows` 的调度。

### 2.10 高亮放后台，增量且防串色
`internal/highlight`：chroma 在独立 goroutine；整文件只 lex 6,000 行，超出部分按视口窗口；编辑后从「字符串/注释之前的安全行」重 lex，若前缀结果一致就保留，向后到「几行一致」为止 —— 6,000 行 Go 文件敲一键约 **0.7 ms**（否则 170 ms）；打字停后再全量校正；**没算完的行走「保持旧色 / 变行显示无色」**，避免把旧颜色贴到新文本上。

### 2.11 搜索复用索引 + 代次号
`search.go`：复用 quick-open 的文件索引，多 goroutine 读；**每次搜索一个序号，旧结果直接丢弃**；每 60 ms 回传一批并按路径排序；打开且脏的文档用内存文本搜；结果所在目录被 watch，磁盘变化也能更新；替换时重新定位匹配，对打开的文档做一次 undo 事务，对关闭的文件后台写。

### 2.12 设置、i18n、性能预算与「设计测试」
- `internal/settings`：`settings.json`（偏好）+ `state.json`（窗口/tab/布局）两个文件，改动后稍延迟写入；**监听文件，发现是外部改的就重新读取并应用到所有窗口**；字体每帧对比设置，变了才重新测量。
- `locales`：ICU 消息目录 → `go generate` 出类型化函数；视图每帧取文案，切语言立即生效；菜单/面板标题在语言变化时重建。
- **性能预算进 CI**（`budget_test.go`，`MAYOI_BUDGETS=1`）：空闲 2 ms、滚动 5 ms、打字 4 ms、查找打开时打字 6 ms、快速打开、面板、2000 条会话滚动… 超预算即测试失败，改预算必须同 commit 说明。
- **设计规则测试化**（`docs/DESIGN.md`）：动效时长只能来自 `motion.go` 的 token（`TestMotionUsesTokens`）、tab 标题不随 hover 位移（`TestTabTitlesHoldStill`）、状态栏数字用等宽数字（`TestStatusPositionHoldsItsWidth`）、文案规则（`TestCopyRules`）、状态画廊（`GALLERY=… go test -run TestGallery`）。

## 3. 与我们的方案对照

| 主题 | Mayoi 的选择 | 我们的选择 | 说明 |
|---|---|---|---|
| 编辑器 | **不用 `ui/editor`**，用 `TextAreaBufferBase` + 自绘装饰 + chroma | 先用 `ui/editor` | 他们需要共享 undo、Vim、装饰、Markdown Live、分屏；我们不需要。若将来要这些，再迁移 |
| Diff | 自绘行（不用文本区） | 先用 `editor.DiffView` | 我们的冲突/合并视图可能需要三方组合，先小范围试 |
| Hunk 暂存 | `hash-object` + `update-index` | ★ 改用这个 | 比 `git apply --cached` 稳 |
| Git 状态 | CLI + 5 s 轮询 + mtime 探针 + 去重 | 同 | 直接抄 |
| 键位 | VS Code JSON + 集中注册表 + takeKeys | 同 | 越早越好 |
| 外壳 | 10 万行量级的单包 | 目标 2–2.5 万行 | 我们需要克制功能：不做 SSH/插件/多主题 |
| 框架版本 | **fork**（`replace sorrycc/mygo`） | 上游 0.9.2 | 见风险：上游缺口要留 patch 通道 |
| 测试 | 预算测试 + 设计测试 + 状态画廊 | 至少截图回归 + 预算测试 | 他们的做法值得整套搬 |

## 4. 对我们排期的修正

- 上一版评估里「外壳 6–10 人周」偏乐观。按 Mayoi 的实际体量（10 万行、功能集是我们的 3–4 倍）折算，**我们这套 16 张稿的 MVP 外壳 + 服务大约 2–2.5 万行、12–18 人周**（含测试与设计迭代），其中：
  - 命令/键位/状态/窗口：3 人周
  - 侧栏/树/Explorer/搜索 UI：2–3 人周
  - SCM + diff + 暂存 + 历史：4 人周
  - 冲突/合并视图：2 人周（最不确定）
  - AI 面板与三态浮标：2 人周
  - 终端面板 + 设置 UI：2 人周
  - 打磨与截图回归：2 人周
- 编辑器与 diff 查看器直接用 `ui/editor`，能省 4–5 人周（Mayoi 为此写了约 5,000 行自绘代码）。

## 5. 可借用的文件清单（按性价比排序）

1. `hunks.go`（314 行）— 暂存/丢弃/更正索引的算法，直接改写成我们的 `git.StageHunk`。
2. `diffrows.go` + `diffpaint.go` 的调度思路 — 若要做「一个 tab 看全部改动」。
3. `internal/keymap`（约 1.9k 行）— VS Code 键位解析 + takeKeys 优先级。
4. `internal/workspace`（约 1.6k 行）— 懒加载目录 + fsnotify 批处理 + 索引 + 模糊匹配 + glob。
5. `search.go` 的代次号/批量/脏文档策略。
6. `internal/highlight` — 增量 lex 与防串色（若我们不用 `editor` 的 tokenizer）。
7. `budget_test.go` 的测量方法（medianFrame + 命名预算）。
8. `docs/DESIGN.md` — 动效 token、稳定性规则、文案规则，可直接作为我们的设计验收清单。

## 6. 不采纳 / 暂不做

- 远程 SSH（`internal/remote`，23 个文件）— 我们 v1 不做，但 **`vfs` 的路径抽象值得预留**。
- GitHub 集成、Issue/PR 视图、插件引擎（moejs）、Vim 模式、多主题与吉祥物、Markdown Live Preview、Sessions、cloud sync、usage stats。

## 7. 需要警惕

1. **fork 依赖**：Mayoi 用 `replace github.com/egoist/mygo => github.com/sorrycc/mygo v0.4.9-…`，说明上游有缺口（如 `TextBlocks` 在上游 `ui` 里找不到）。我们若跟进，需要：vendor + 固定 commit + 自己的 patch 清单，别和上游 0.9.x 混用。
2. **单包 8.5 万行**：主包太大，构建与重构都慢。我们不能照抄单体，建议 `cmd/lightcode` 只留装配，UI 分到 `internal/ui/{chrome,editor,scm,diff,ai,settings}`。
3. **他们不用 `ui/editor`**：若我们依赖 `ui/editor`，要接受它的限制（无装饰 API、无 Vim、tokenizer 固定），并把迁移成本记在风险表里。
4. **性能只在 M 系 Mac 上测**：Windows 的帧成本与启动差异要自己建基线（我们已有示例的 272 ms / 55.8 MB）。
