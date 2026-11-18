# MyGo 跨进程界面嵌入（ada_link）设计方案

> 目标：**宿主 MyGo 应用在自己的 native UI 窗口里，嵌入另一个 MyGo 应用的 native UI，
> 两者在不同进程，走一条可控的进程间通道。**
>
> 本文先分析 MyGo（`github.com/egoist/mygo` v0.7.4，commit `5b33c407`）上与这件事相关
> 的机制，再给出方案、协议、取舍与分期。协议规范见 [docs/protocol.md](protocol.md)，
> 已实现并验证的原型见本仓库源码与 [README](../README.md)。
>
> 库入口：`github.com/learningdog1/ada_link`（根包 `adalink`）；使用指南见
> [docs/library.md](docs/library.md)。

---

## 0. 摘要

| 结论 | 内容 |
|---|---|
| 主方案 | **像素中继（pixel relay）**：子进程离屏渲染自己的 `ui` 视图，帧写进两个进程共享的帧环；宿主把帧当作纹理合成进自己的 native UI，并把输入转发回去 |
| 控制通道 | Unix domain socket + 长度前缀 JSON；复用 MyGo 页面 IPC 的 `call`/`reply`/`event` 词汇 |
| 像素通道 | 文件支撑的 3 槽帧环（premultiplied RGBA8888）+ 累计确认；大块数据不进 socket |
| 宿主 API | `adalink.Attach(win, session)`（即 `mygoembed.Attach`）返回一个 `Host`，其 `View` 构建 `ui` 视图 |
| 子进程 API | `adalink.Serve(ServeOptions{View, Service})` 把普通 `ui.View` 跑在离屏渲染器里 |
| 已验证 | 无窗口端到端（帧/点击/RPC）、真实 MyGo 窗口截图、按用户安装/卸载、`-race` 通过、帧性能测量 |
| 明确不做 | 窗口重父化（跨平台不可行）、v1 不传无障碍树/GPU 纹理 |
| 后续 | P2 把离屏表面、原始帧捕获、输入注入做成 MyGo 公共 API；P3 显示列表中继替代像素中继 |

为什么不是"再开一个窗口"或"把子进程窗口塞进宿主窗口"：MyGo 的 native UI 是每个窗口
各自的 GPU 场景（`docs/architecture.md` 的 Native UI 一节），macOS 没有跨进程 NSView 重父化，
Windows 的 `SetParent` 在 WebView2、焦点、DPI 与合成上都有坑。像素中继是 macOS、
Linux、Windows 上都能用纯 Go 实现、且对子进程渲染方式没有要求的直接路径。

---

## 1. 需求与约束

**功能**

1. 子进程拥有自己的 Go 运行时、服务与插件；崩溃不拖垮宿主。
2. 子进程的 native UI 出现在宿主窗口的一个矩形里：随布局缩放，被宿主的 clip、圆角、
   菜单、对话框、弹出层正确覆盖。
3. 指针、滚轮、键盘、文本、IME 组合、焦点、拖放从宿主流到子进程。
4. 子进程的状态变化在 1 帧内到达宿主（60 Hz 下 ≤ 16.7 ms）。
5. 除界面外还要有类型化的双向调用与事件（宿主调用子进程服务、子进程调用宿主服务）。
6. 会话可启动、可关闭、可检测崩溃；开发模式可热重启。
7. macOS / Linux / Windows 同一套代码；**不引入 cgo**；不要求 Bun 运行时。

**非功能**

- 宿主与子进程都是单二进制 MyGo 应用；不用宿主的体积为不使用的功能买单。
- 空闲时不产生帧、不占 CPU。
- 安全边界：只有本次会话的两个进程能读写帧与消息；会话之间互不可见。

**非目标（v1）**

- 嵌入非 MyGo 的原生界面（任意窗口/任意进程）。
- 宿主窗口是纯 web 页面（`WindowOptions.URL`）时嵌入 native UI：页面窗口没有放原生视图
  的那一层；这种宿主只能用 `ui.WebView` / 自定义 scheme 嵌**网页**。
- 跨机器、跨用户的远程界面。
- 让宿主的辅助功能（屏幕阅读器）完整读到子进程的控件。
- 子进程使用宿主进程的 GPU 上下文或纹理句柄。

---

## 2. MyGo 现状分析

### 2.1 一个窗口可以有两种界面

MyGo 的窗口要么显示系统 webview 里的网页（WKWebView / WebKitGTK / WebView2），要么显示
`ui` 包自绘的 native UI（`WindowOptions.Content`，见 `window.go:61`、`docs/architecture.md`
的 "Native UI"）。同一个应用的多个窗口可以混用，同一窗口里也能用 `ui.WebView` 放一个
webview（`webview.go:12-54`、`ui/webview.go:39-47`）。但 **native UI 的 `ui` 视图只能属于
创建它的进程**：它是该窗口 GPU 场景的一部分，没有跨进程句柄。

### 2.2 现有 IPC 只面向"页面"

页面 IPC 是 MyGo 最成熟的部分：

- `mygo.Bind` 反射注册服务（`ipc.go:96`），页面用生成的 TypeScript 客户端调用；
- `mygo.NewEvent[T]` 声明带类型事件（`ipc.go:462-487`），`Emit`/`EmitPage`/`Broadcast`；
- `Channel[T]` 作为方法参数流式回传（`channel.go`），1 MiB 流控窗口（`channel.go:126`）；
- 消息在 `Window.enqueue` 里按主循环轮次批量合并成一次 `__mygo.receive([...])`
  （`window.go:1637-1657`、`1659-1686`）；事件在 DOM ready 前最多保留 1024 条
  （`window.go:1631`）。

**但它绑定在 `*Window` 与页面上**：调用来自"页面的 secret + token"，通道的 id 由页面分配，
方法执行时要一个 `*Window` 来建通道（`ipc.go:317`、`channel.go:163`）；native UI 窗口
没有页面，`enqueue` 直接返回（`window.go:1638-1640`）。因此 **不能**直接把 `Bind` 拿去给
另一个进程用，这是本方案里最值得上游改造的一处（第 8 节）。

### 2.3 已有的跨进程积木

| 机制 | 位置 | 可复用什么 |
|---|---|---|
| 单实例锁 | `single_instance.go:59-113` | 证明 `net.Listen("unix")` 在三个平台都可用（Windows 10+ 的 AF_UNIX），以及"短 socket 路径 + 哈希文件名"的规避（`single_instance.go:174-185`） |
| automation 插件 | `plugins/automation/` | 换行分隔 JSON（NDJSON）、16 MiB 行上限（`protocol.go:20-30`）、请求/回复信封、把事件注入 native UI（`automation.go:174-182`，走内部的 `surface.Conn.Event`） |
| 自定义 scheme | `protocol.go:53-70` | 由 Go 提供的流式 `http.Handler`（256 KiB 块、最多 4 块等待主线程，`protocol.go:240-245`）；只适合"页面从另一个进程取 HTML"这种 Web UI 方案 |
| WebView | `webview.go:62-125` | 窗口内任意布局位置的 webview，证明"外部内容 + 宿主 chrome"可行；但它是 webview，不是 native UI |
| `Window.CapturePage` | `window.go:1497-1509`、`content.go:193` | native UI 窗口能整体截图为 PNG（automation 用它）；v1 的原始帧捕获可以由此发展，但 PNG 编码不适合 60 Hz |

**关键缺口**（相对本方案需要的）：

- 没有公共的 **native UI 原始像素 + 脏矩形** 捕获（`raster.Renderer.Render` 有 damage，
  但 `internal/raster` 不可被 app 导入，`ui.Tester` 也不暴露 damage）。
- 没有公共的 **合成输入注入**（只有内部的 `surface.Conn.Event`，automation 借此实现；
  发布版还因 `mygo_noinspector` 编译掉，`plugins/automation/off.go`）。
- 没有 **child process 抽象**（`os/exec` 只在 updater/CLI 里各自出现）。
- 没有 **共享内存/共享表面** 抽象。

### 2.4 `ui` 的离屏与宿主合成能力

这两点决定了原型可以今天就跑起来：

**子进程侧**：`ui.NewTester(view, w, h)`（`ui/api_gen.go:1458`）在内存里用软件光栅器
（`internal/raster`）渲染视图，不建窗口；`Tester` 能接受指针、键盘、文本、IME 组合、滚动、
缩放、焦点、拖放等输入（`ui/headless.go:508-800`），`Tester.Image()` 返回
premultiplied RGBA（`ui/headless.go:211-214`），`Tester.Advance` 驱动它自己的时钟
（`ui/headless.go:284-325`）。**这就是一个可被驱动、无窗口的 native UI 表面。**

**宿主侧**：`ui.NewBitmap` + `Bitmap.Update(img image.Image)`（`ui/bitmap.go:97-122`）
为视频设计——尺寸不变时复用 GPU 纹理，只上传新像素，`*image.RGBA` 连续行**零拷贝**接收；
`Window.Update(fn)`（`content.go:173-178`）把 fn 放到主线程执行并请求重绘；绘制用
`Painter.Image(bmp, r, fit)`（`ui/paint.go:1371`）。像素尺寸按 `ceil(DIP × scale)`
计算（`ui/zoom.go:72-77`），与帧环的尺寸约定一致。

### 2.5 由此得到的两条可行路线

1. **像素中继**：子进程渲染，宿主合成。渲染方式任意（CPU 光栅、GPU 离屏、甚至真实窗口的
   截图），协议只需要"帧 + 输入"。**v1 采用。**
2. **显示列表中继**：子进程把 `ui` 的绘制指令（`scene` 的 op 列表）序列化，宿主用自己的
   GPU 重放。分辨率无关、带宽极小、能继承宿主主题与缩放，但需要覆盖 `scene` 的全部图元、
   字形图集、图片、着色器（glass 等），工作量大得多。**P3。**

---

## 3. 总体架构

```
┌──────────────────────────── 宿主进程 (MyGo app) ───────────────────────────┐
│  Window (native UI)                                                        │
│   ui.Row                                                                   │
│    ├── 宿主的原生控件（标题、按钮、状态……）                                │
│    └── mygoembed.Host.View(c)                                              │
│          ui.Box.Fill().Draw ── Painter.Image(bitmap, r, FillBox)           │
│          bitmap ◀── Window.Update(fn) ◀── 帧（主线程合并，只留最新）       │
│                                                                            │
│  link.Session                                                              │
│    ├─ 控制 socket（长度前缀 JSON）：hello/welcome/resize/input/frame-ack/   │
│    │                              call/reply/event/bye                     │
│    └─ 帧环文件（mmap 或页缓存）：3 × premultiplied RGBA8888 槽             │
└────────────────────────────────────────────────────────────────────────────┘
                    ▲ socket 0600 in 0700 session dir   ▲ frames
                    │ token 握手                       │
┌──────────────────────────── 子进程 (MyGo app) ────────────────────────────┐
│  mygoembed.Serve                                                           │
│    ui.Tester(View, w, h)   ← 输入（指针/键/文本/IME/焦点/拖放）            │
│    每帧：build → raster.Render → Image() → Damage → 写槽 → frame 消息       │
│    服务：在 UI goroutine 上执行宿主调用，可改 View 读的状态                 │
└────────────────────────────────────────────────────────────────────────────┘
```

**为什么控制走 UDS**：Windows 10+ 支持 AF_UNIX 且 MyGo 的单实例锁已在用；不需要 cgo；
路径权限就是用户权限；socket 上只走小消息，帧不占带宽。

**为什么像素走文件支撑的共享内存**：一个文件被两个进程打开，页缓存就是共享内存；跨平台、
可清理、崩溃后不会留下内核对象。Unix 用 `mmap(MAP_SHARED)` 零拷贝；Windows 原型走
`ReadAt/WriteAt`（页缓存仍然共享，代价是一次内核拷贝），生产实现改用
`CreateFileMapping`/`MapViewOfFile`（见 `internal/shm/shm_windows.go` 的说明）。

**为什么不做窗口重父化**：macOS 没有跨进程视图重父化；Windows 的 `SetParent` 会打乱
WebView2 的合成、焦点、DPI 与系统菜单；Wayland 不允许跨客户端重父化。像素中继绕开这些，
并且对子进程是否真的有窗口没有要求。

---

## 4. 通道设计

### 4.1 两条通道，各司其职

| 通道 | 传输 | 内容 | 节奏 |
|---|---|---|---|
| 控制 | socket，长度前缀 JSON | 握手、尺寸、输入、帧通知/确认、调用/回复/事件 | 事件驱动，小消息 |
| 像素 | 文件帧环 | 整帧或脏矩形标记 | 最多 3 帧在途 |

**帧不走 `Channel[T]`**：MyGo 的通道是 JSON、绑定页面生命周期、1 MiB 流控窗口
（`channel.go:126`），适合小数据流；1280×800 一帧 4 MiB，base64 后 5.5 MiB，且没有
"只保留最新"的语义。帧环 + 累计确认给了背压与丢帧策略。

### 4.2 握手与鉴权

宿主派生时生成 128 位 token 与短路径会话目录，子进程从环境变量拿到 socket、token、目录、
初始尺寸；子进程拨号后第一条消息是 `hello`（含 proto、token、帧环信息）。宿主校验
token（常量时间）、proto、帧环文件名与尺寸，打开帧环、**重新校验文件头**，再回 `welcome`；
子进程收到 `welcome` 才开始发帧。任何一步失败都杀子进程、删目录。

细节与消息表见 [docs/protocol.md](protocol.md)。

### 4.3 帧环与背压

- 3 个槽；子进程只写"宿主已确认"的槽；写像素 → 发 `frame(seq, slot, damage)`。
- 宿主读完槽、把像素放进自己的缓冲后发累计 `frame-ack(seq)`。
- 宿主忙时合并帧：只留最新一帧，旧帧立即 ack（`link.Frame.Release`），子进程因此可以继续
  画；子进程没有空闲槽时**丢帧**并计数，绝不阻塞输入与 RPC。
- 会话换尺寸时子进程建新环（`frames-N.bin`）、先发 `surface` 再发新环的帧；`seq` 跨环递增，
  宿主按 `seq > lastSeq` 接受。

**为什么累计 ack**：宿主合并帧是常态；累计确认让"只消费最新帧"不需要逐帧跟踪，也不会
让中间帧的槽永远占用。

### 4.4 输入模型

宿主把 `ui.InputEvent` 转成 `link.InputEvent`：坐标是**元素内容盒内的 DIP**（`ui` 交付给
`HandleInput` 时已转成本地坐标，见 `ui/handler.go:149-157`），正好等于子进程画面的坐标，
不需要换算。键盘带 `Key`/`PhysicalKey`/`Mods`，文本、IME 组合、编辑命令、焦点、拖放分别
映射（表见协议文档第 5 节）。

三个有意的取舍：

- **宿主的 `Cmd` 快捷键不转发**：面板是宿主应用的一部分，`Cmd+S` 应该保存宿主，而不是
  子进程；子进程自己的快捷键用不带 `Cmd` 的键。编辑命令（`copy`/`paste`/`undo`…）仍以
  `InputCommand` 转发，所以子进程的文本框照常工作。需要"面板独占键盘"时可以让宿主在
  菜单里声明一个模式，P2 再做。
- **中键在 v1 子进程不可表达**：`ui.Tester` 只暴露主键/次键的按下释放；协议本身支持
  `b=2`，P2 的 `ui.Surface.SendInput` 直接收完整 `ui.InputEvent`。
- **IME 候选窗**：v1 宿主窗口负责候选窗位置，子进程只渲染组合串；真正的锚定要 P2。

### 4.5 调用与事件

复用 MyGo 页面 IPC 的词汇（`call`/`reply`/`event`），因为这样两边的语义、错误传播、JSON
选项和 TypeScript 客户端生成都能沿用。原型用 `HandleAny` 路由（子进程只有一张
`Service(method, args)` 表），生产实现应走第 8 节的 `ipc.Peer` 泛化。

事件在第一个监听者出现前保留（每名字 64 条）：子进程在启动时 `Emit` 初始状态，宿主稍后
`OnEvent` 注册，不会竞争丢事件（MyGo 页面 IPC 的 "DOM ready 前保留"是同一类问题）。

---

## 5. 渲染、合成与性能

### 5.1 子进程渲染

v1 用 `ui.Tester`（CPU 光栅）。它今天就能用，但**是测试取向的 API**：

- 每个视图它最多 settle 20 帧（`ui/headless.go:259-270`），动画只靠 `Advance` 推进；
- 同一父节点下重复的 `Key` 会 panic（`ui/api_gen.go:1455-1459` 的注释）；
- 不暴露无障碍树、光标形状、候选窗；
- `Image()` 把内部 BGRA 逐像素转成 RGBA 并新分配 4 MiB（`internal/raster/raster.go:38-50`）。

`Image()` 的转换是 v1 最大的单笔开销，测出来约 3.2 ms/帧（1280×800）；渲染本身约
0.3 ms。P2 的 `ui.Surface` 应当直接交 premultiplied BGRA 与脏矩形，宿主在上传时做通道
交换（或在 shader 里采样 BGRA），把这一笔省掉。

### 5.2 宿主合成

- `Bitmap.Update` 复用纹理，整帧上传；`Window.Update` 保证在主线程执行并在下一帧重绘。
- 元素 `Draw` 里每帧比较内容盒尺寸与 `Painter.Scale()`，变化时 `Session.Resize`，让子进程
  按宿主 DPI 重新渲染（`deviceSize` 用 `ceil`，与 `ui/zoom.go:72-77` 一致），因此通常
  是 1:1 像素映射，不是位图拉伸。
- 帧到达时主线程合并：`Host.frame` 只保留最新一帧并调度一次 `Window.Update`；被替换的帧
  立即 `Release`（= ack）。
- damage 在 v1 只用于确认"是否真的变了"（`link.Damage` 未变化时 2.5 µs），因为
  `Bitmap.Update` 只能整纹理上传；P2 做部分上传，P3 显示列表则根本不需要像素。

**宿主覆盖层次**：面板是普通 `ui` 元素，所以宿主的弹出菜单、`ui.Modal`、toast、tooltip、
clip 容器都自然画在它上面（和 `ui.WebView` 的 "hole" 不同，这里没有原生子视图，不存在
z-order 问题）。这是像素中继相对 webview/子窗口方案的一个额外好处。

### 5.3 实测（本机：Windows，Intel i5-10500，Go 1.27.1）

| 项目 | 结果 | 备注 |
|---|---|---|
| 子进程整帧路径（build+render+RGBA 转换+damage，1280×800） | **3.5–3.8 ms** | `BenchmarkViewFrame1280` |
| 其中 `Tester.Image()` 转换 | ~3.2 ms | 逐像素 BGRA→RGBA + 4 MiB 分配 |
| 帧环写入 1280×800（4.1 MiB） | **约 0.94 ms**（4.36 GB/s） | Windows 页缓存 I/O；Unix mmap 更快 |
| 帧环读出 | **约 0.81 ms**（5.03 GB/s） | 同上 |
| 未变化帧的 damage 检测 | **2.5 µs** | 按行比较；空闲时不渲染 |
| 点击 → 状态确认 + 新帧 | **14–16 ms** | 端到端 demo，含一次 RPC 往返 |
| 内存（1280×800，scale 1） | 帧环 3×4.1 MiB + 宿主帧池 3×4.1 MiB + 子进程图像约 8 MiB | 约 33 MiB 额外 |

**解读**：在 60 Hz 下，一帧的 CPU 预算 16.7 ms，v1 的 3.5 ms 足够跑 60 fps 的 1280×800；
4K/高 DPI 会不够（CPU 光栅与转换都按像素线性增长），这是 P2 换 GPU/真实窗口路径的主要动机。
空闲时子进程只在 ticker 上检查 `FramePending`/dirty，不建帧、不发帧。

---

## 6. 生命周期与容错

- **启动**：`link.Spawn` 建会话目录（短随机名，系统临时目录，0700）→ 监听 → 派生 →
  20 s 内等 `hello` → 打开帧环 → `welcome`。失败即清理。
- **运行**：子进程退出/连接断开 → 宿主 `Session.Done()`、`Err()` 给出原因，杀进程（若还在）、
  删目录。宿主关闭 → 发 `bye`、关闭 peer、2 s 宽限后杀子进程、删目录（Windows 上必须等
  子进程关闭环文件，所以顺序是"先等退出再删"）。
- **崩溃**：宿主看 `Done()`；面板视图可以显示占位与"重启"按钮。自动重启策略留给应用
  （建议指数退避 + 状态快照），`link` 只提供 `Spawn`。
- **孤儿**：会话目录只由宿主删除；子进程拿到的环境里没有"自毁"逻辑，杀掉宿主不会留下
  socket 文件（下次同目录随机名，不会冲突；临时目录由系统清理）。
- **开发模式**：重建子进程二进制 → 杀掉旧会话 → 新 `Spawn`；宿主窗口不变，面板显示几帧
  占位，状态由应用自己恢复。

### 6.1 演示的按用户安装（包装层）

为了让"嵌入的是**另一个应用**"这件事一眼可见，演示把宿主与子进程做成两个独立的 exe，
并提供一套最小的按用户安装：

- **两个应用**：`AdaLink.exe`（宿主：窗口、chrome、安装/卸载按钮）与 `AdaPanel.exe`
  （子进程：同一份 `ui.View`，有环境变量时 `Serve`，没有时是普通窗口）。宿主优先启动
  与它同目录的 `AdaPanel.exe`，否则退回"自己 + `-child`"的单文件模式。
- **安装位置**：`%LOCALAPPDATA%\Programs\AdaLink`，按用户、无管理员；复制两个 exe，
  建 Start Menu（Ada Link、Ada Panel）与桌面（Ada Link）快捷方式，在
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\AdaLink` 写 Settings 的卸载项
  （`DisplayName`/`DisplayVersion`/`DisplayIcon`/`UninstallString`/`EstimatedSize`…）。
- **卸载**：删除快捷方式、注册表项与目录。若卸载器正运行在安装目录里，Windows 不允许删除
  正在运行的 exe，所以退出前启动一段**无控制台的延迟清理脚本**，等进程结束后再删目录。
  （实现上 `DETACHED_PROCESS` 启动的 PowerShell 不会执行脚本，`CREATE_NO_WINDOW` 会且能
  在父进程退出后继续运行，这是一个容易踩的坑。）
- **直观的对照**：单独启动 `AdaPanel.exe` 是普通窗口；启动 `AdaLink.exe` 时同一份 UI 出现在
  宿主的窗口里，任务管理器里两个进程，PID 与父进程关系一眼可见。

安装层是**演示包装**，不是通用安装器：生产应用应当用 MyGo 自带的 `mygo build` 生成
安装包、签名与公证（`docs/distribution.md`），本仓库的 `install` 包只覆盖
"装得上、卸得干净、看得见"的范围（`install.Config` 是通用的，见 `docs/library.md`）。

### 6.2 多面板：registry、事件命名空间与广播

一个宿主嵌多个子进程时，v1 的 `Session` 已经天然隔离（一条会话一条 socket、一份帧环），
缺的是宿主的**账本**与**命名**。`panels` 包补上这两点：

- **Panel registry**：`Add(id, title, sess)` / `Attach(id, win)` / `List` / `Get` /
  `Remove` / `Close`；状态 `Running`/`Closed`/`Failed`，`OnChange` 通知增删与结束；
  失败的面板留在表里，便于 UI 显示原因并重试。
- **事件命名空间**：子进程的 `Emit("ready", …)` 在宿主侧成为
  `panel/<id>/ready`；`OnEvent` 跟随现在与将来的面板，`OnPanelEvent` 只订指定面板；
  `Topic`/`SplitTopic` 提供构造与解析。宿主侧保留 `host/<name>` 前缀，给未来 Hub 用。
- **广播策略**：`Broadcast(name, payload, opts)` 只发 `Running`（`Closed: true` 例外），
  支持 `Include`/`Exclude`，并发发送并逐面板报告结果；发给子进程的 `name` 不加命名空间
  （子进程只认识 `theme`，不认识 `panel/...`）。广播不是原子屏障：跨面板的"同时"不存在，
  需要一致性就带 epoch。
- **仍然不是总线**：不做 N×M 扇出、每订阅者有界队列、latest 缓存、宿主页面订阅桥。
  这些是 Hub 的职责；阈值与方案见 `docs/panels.md` 第 3 节。Registry 的监听器仍在会话读协程
  上执行，因此不得阻塞——Hub 接入时应先非阻塞入队，再由自己的 dispatcher 扇出。

这样多面板的常见场景（每个面板的事件只喂自己的 UI、宿主向所有面板广播主题）不需要 Hub；
只有当出现第二个消费者、动态订阅或统一背压需求时，才在 Registry 之上加 Hub。

---

## 7. 安全模型

| 关注点 | v1 的做法 |
|---|---|
| 谁能连上控制 socket | 0700 会话目录 + 0600 socket，同用户才能连；128 位 token 常量时间比较 |
| 谁能读像素 | 帧环文件 0600，在 0700 目录中 |
| 消息洪泛 | 单条 ≤ 16 MiB、帧环 ≤ 8 槽、单边 ≤ 16384 像素、每名字 64 条保留事件、3 帧流水线 |
| 恶意帧环 | 宿主重新读文件头并校验 magic/尺寸/槽数/文件长度；`frame.file` 必须是纯文件名 |
| 子进程越权 | 子进程以用户权限运行，能读自己的文件/网络/剪贴板；**不能**读宿主内存（只有帧环与 socket）。要更强隔离需要 OS 沙箱（macOS App Sandbox / Windows AppContainer / Linux seccomp），不在 v1 范围 |
| 子进程代码来源 | 谁启动谁负责；下载/插件市场的子进程应由宿主校验签名后再启动 |

宿主的页面（webview）与子进程是**两条独立的信任边界**：页面通过 MyGo 的 `isTrusted`
（`window.go:1342-1370`）受控；子进程只拿到 link 能力。不要因为"子进程是自家代码"就把它
的方法直接暴露给任意页面——如果要做，应当把子进程方法以命名空间（如 `link:child.Service.Method`）
另行绑定，并沿用页面的信任检查。

---

## 8. 与 MyGo 现有 IPC 的统一（建议的上游改造）

目标：**`mygo.Bind` / `NewEvent[T]` / `Channel[T]` 原样工作，只把"页面"换成"对端"**。

现状的耦合点：`method.call` 需要 `*Window` 来建通道并 `enqueue` 回复（`ipc.go:282-354`、
`ipc.go:317`）；页面身份是 `*Window` + page token；`enqueue` 对 native UI 直接丢弃
（`window.go:1638-1640`）。

建议引入一个小接口（示意，名字与签名以实现为准）：

```go
// package mygo (internal/ipc)
type Peer interface {
    // Context gives the call's context, canceled when the peer goes away.
    Context() context.Context
    // NewChannel creates the peer's side of a channel id it sent.
    NewChannel(id int64, ctx context.Context) (*channel, error)
    // Enqueue sends a reply, event or channel value to the peer.
    Enqueue(m message)
    // Trusted reports whether the peer may call Go at all.
    Trusted() bool
}
```

- `*Window` 实现它（行为不变，`handleCall` 改为 `method.call(peer, args)`）。
- `link.Session` 实现它：`call` 消息来自子进程时走同一套反射与校验；回复编码成
  `reply` 消息；`Channel` 的 id 由子进程分配，宿主侧 ack 走 `chan-ack` 消息。
- 事件：`Event[T].Emit` 增加 `EmitPeer(peer, payload)`，`Broadcast` 不再只遍历
  `pageWindows()`。
- 信任：`link.Session` 的 `Trusted()` 由宿主在 `Spawn` 时决定（默认 true；子进程 UI
  来自不可信来源时为 false，`call` 直接被拒绝）。
- TypeScript 生成不变：绑定表在编译期就是程序里的服务，与传输无关。

这样"两个进程的应用"与"应用 + 页面"共享一套 IPC 语义，`channel` 流控、panic 恢复、
JSON 选项、错误文本都只有一份实现。**这也是把本方案做成 `plugins/link` 或上游包的前提。**

**如果还要让宿主的 webview 页面调用子进程服务**：不要直接把 `link.Session` 暴露给页面。
把子进程方法以命名空间绑进页面的方法表（如 `link.child.Order.Create`），在绑定层做白名单
与参数校验，并沿用页面的 `isTrusted` 检查（`window.go:1342-1370`）；页面永远拿不到 socket、
token 与帧环。这和第 7 节的信任边界是同一件事：**子进程是资源，不是页面的能力**。

---

## 9. 分期路线图

### P1 — 本仓库（已实现并验证）

- `link`：UDS + 长度前缀 JSON、握手、帧环、输入、调用/事件、会话生命周期。
- `mygoembed`：`Attach`（宿主视图 + 输入 + 主线程合并）与 `Serve`（`ui.Tester` 离屏 + 服务）。
- 库化：模块路径 `github.com/learningdog1/ada_link`；根包 `adalink` 提供
  `Spawn`/`Attach`/`Serve` 与类型别名；公开 `link`（纯 IPC）、`mygoembed`（MyGo 接入）、
  `panels`（多面板注册表、命名空间事件与广播）、`install`（Windows 安装）；
  `examples/minimal` 与 `examples/multi` 是最小示例；使用指南在 `docs/library.md`、
  `docs/panels.md`。
- 验证：`go test -race ./...`；`cmd/linkdemo` 无窗口端到端；`cmd/linkapp` 真实窗口截图
  （`artifacts/window.png`）；`cmd/panel` 单独窗口与嵌入两种形态；按用户安装/卸载与
  自删除清理；`panels` 的两个真实子进程端到端（命名空间事件、广播、状态、移除），
  `examples/multi` 两个面板在同一个窗口的截图。

### P2 — 上游 MyGo API（让"生产"成立）

1. **`ui.Surface`**：公共的离屏表面。`ui.NewSurface(view, SurfaceOptions{Size, Scale})`，
   `Surface.Frame() (pix []byte /*BGRA*/, stride int, damage []image.Rectangle, ok bool)`、
   `Surface.SendInput(ui.InputEvent)`、`Surface.AccessTree()`、`Surface.SetClock(...)`。
   内部复用 `headless` + `raster.Renderer.Render` 的 damage（`internal/raster/damage.go:27-40`），
   但把 BGRA 直接交出去，避免 `Image()` 的转换。
2. **真实窗口的原始帧**：`Window.CaptureFrame() (pix []byte, w, h int, damage []image.Rectangle, err error)`，
   由 GPU 渲染器给出 BGRA + damage（现在的 `CapturePage` 是 PNG，`window.go:1497`；
   automation 已证明 native UI 可整体截图，`content.go:193`）。有了它，子进程可以是
   **普通 MyGo 窗口 + 隐藏显示**（`WindowOptions.Hidden`，`window.go:92-94`），拿到 GPU
   渲染、真实 IME、真实无障碍树；隐藏窗口的持续帧驱动需要在 P2 里验证与补足。
3. **输入注入**：把 automation 用的内部 `surface.Conn.Event`（`internal/surface/surface.go:86-89`）
   提升为公共 `Window.SendInput(platform.SurfaceEvent 的公开镜像)`，或直接
   `Window.SendInputEvent(ui.InputEvent)`。
4. **`ipc.Peer` 泛化**（第 8 节），`link` 作为 `plugins/link`（Go + npm 客户端可选）。
5. **共享内存**：Windows 的 `CreateFileMapping`/`MapViewOfFile` 与 Unix 的 `memfd`/`shm_open`
   （如果不想留文件），以及在可能时用 GPU 共享纹理（见 P4）。

### P3 — 显示列表中继

子进程把 `scene.Scene` 的 op 列表（矩形、圆角、渐变、路径、阴影、文本 run、图片、blur/glass
效果、canvas）序列化成命令流，宿主用自己的 GPU 重放。收益：

- 分辨率无关、宿主 DPI/缩放/主题天然正确；
- 带宽从 4 MiB/帧降到几 KiB～几百 KiB；
- 宿主能做局部 damage 上传、把远端控件与本地控件同一层合成；
- 不需要 CPU 光栅化，功耗低。

代价：协议大（每个图元都要规范）、字体/图集/图片资源要传或共享、着色器效果（glass、blur）
需要双方都实现、文本 shaping 结果的跨版本一致性。**只在中继层做，不改变 `ui` 的公共 API**：
`ui.Painter` 已经有完整的绘制调用面，`scene` 已经是序列化友好的结构，这是可行性的基础。

### P4 — 更远的选项

- **GPU 共享纹理**：Windows 的 D3D11 shared handle、macOS 的 IOSurface、Linux 的 DMA-BUF。
  需要 purego 与每个渲染后端配合，收益是把"每帧 4 MiB 拷贝 + 纹理上传"也省掉。
- **多面板/标签页**：一个宿主窗口里多个 `Host`，每面板一个子进程或一个进程多个表面。
- **无障碍**：把子进程的 `platform.AccessTree` 片段映射到宿主的可访问性树（`ui` 已有
  `Access`、automation 已能导出 keyed tree，`plugins/automation/automation.go:150-173`），
  这是与像素中继正交的一层。
- **工具链**：`mygo dev` 支持子进程热重启；远端 inspector（复用 native UI inspector 的树）。

---

## 10. 替代方案对比

| 方案 | 跨平台 | 隔离 | 合成/覆盖 | 复杂度 | 结论 |
|---|---|---|---|---|---|
| A. 窗口重父化（HWND/X11） | ✗ macOS 不可；Wayland 不可 | ✓ | 差（焦点、z-order、DPI、WebView2） | 中 | 否 |
| B. **像素中继**（本方案 v1） | ✓ | ✓ | 好（普通 ui 元素，可被覆盖） | 中 | **采用** |
| C. 显示列表中继（P3） | ✓ | ✓ | 最好 | 高 | 二期/三期 |
| D. WebView 代理（子进程提供 HTML，宿主 `ui.WebView` 显示） | ✓ | ✓ | 好（有 webview 的 hole 限制） | 低 | 适用于"子进程是 Web UI"，与本方案互补；不满足 native UI 嵌入 |
| E. 单进程多窗口 | ✓ | ✗ | — | 低 | 不满足隔离需求 |
| F. 远程桌面协议（RFB/VNC） | ✓ | ✓ | 中 | 中 | 通用但重（编码/延迟/无类型化 RPC），本地场景不划算 |
| G. GPU 共享纹理 | 分平台 | ✓ | 最好 | 高 | P4 优化，不是方案骨架 |

**为什么 B 先做**：它把问题拆成两个可独立验证的部分（协议/帧，和渲染），对子进程渲染方式
没有假设，能今天就端到端跑通；C 是把 B 的像素层换掉，协议与生命周期层可以完全保留。

---

## 11. 风险与未决问题

1. **`ui.Tester` 不是生产 API**：20 帧上限、strict key、无无障碍/光标/候选窗。P1 的演示
   能跑，但 P2 必须提供 `ui.Surface`，否则子进程会继承测试工具的边界。
2. **CPU 光栅化的成本**：3.2 ms 的转换 + 0.3 ms 渲染（1280×800）在 4K 上会变成 ~40 ms/帧。
   P2 的 GPU/真实窗口路径是必需的，不是优化。
3. **IME 与键盘语义**：`Cmd` 快捷键的归属、候选窗位置、死键、组合取消、Tab 焦点遍历
   （`ui` 的 `webViewTabOut` 那类"面板把 Tab 让出去"的行为在像素中继里也要实现）。
4. **高 DPI 与分数缩放**：`ceil(DIP×scale)` 让边缘偶尔多 1 像素；宿主元素尺寸可能每帧微变，
   需要死区（本原型按整数 DIP 比较，已避免抖动）。
5. **纹理上传**：`Bitmap.Update` 整帧上传，1280×800@60 是 ~250 MiB/s；GPU 带宽不是瓶颈，
   但移动端/集显要测。P2 做部分上传。
6. **无障碍**：v1 的宿主元素对屏幕阅读器是空白。需要 P4 的树中继，且 `ui.Tester` 的树
   目前不公开。
7. **协议与键值 ABI**：键值是 `ui.Key` 的数值，跨 MyGo 版本会漂移。P2 把键名放进协议，
   并校验 MyGo 版本。
8. **子进程安全**：见第 7 节；沙箱化是另一个课题。
9. **调试体验**：native UI 没有 DevTools。建议 P2 给子进程加 inspector/automation 出口
   （生产构建用构建 tag 关掉），宿主能读子进程的树。

---

## 12. 验收标准

| 指标 | 目标 | P1 实测 |
|---|---|---|
| 输入 → 画面更新 | ≤ 1 帧（16.7 ms @60 Hz） | 14–16 ms（含一次 RPC） |
| 稳态帧率（1280×800） | 60 fps 不丢帧 | 单帧 3.5 ms，余量充足 |
| 空闲开销 | 0 帧、无持续渲染 | 未变化帧检测 2.5 µs，不建帧 |
| 崩溃检测 | 子进程退出后 ≤ 1 s 宿主感知 | `Session.Done()` 即时 |
| 清理 | 会话目录/socket/环文件不残留 | 已测（`%TEMP%` 无 `adalink-*`） |
| 数据竞争 | `go test -race` 通过 | 通过 |
| 内存 | 额外 ≤ 40 MiB（1280×800） | 约 33 MiB |
| 跨平台 | Windows 实机；Linux/macOS 编译 | Windows 实机验证；`internal/shm` 有 unix 实现，待 CI 验证 |

> 说明：本原型在 Windows 实机完成了 UI、帧、点击、RPC、窗口截图与 `-race`；Linux/macOS
> 的 AF_UNIX 与 mmap 路径与 MyGo 单实例锁同源，仍应在 CI 的 Xvfb/桌面会话里补一轮。

---

## 13. API 草案

**宿主**（本仓库已实现，`adalink` 是根包的便捷入口）：

```go
import adalink "github.com/learningdog1/ada_link"

sess, err := adalink.Spawn(adalink.SpawnOptions{
    Command: []string{exe, "-child"}, // 子进程可执行文件与参数
    W: 720, H: 520, Scale: 2,        // 初始 DIP 尺寸与缩放
    Stderr: os.Stderr,
})
defer sess.Close()

panel := adalink.Attach(win, sess)   // 设 OnFrame/OnFocus/OnFileDrop/OnClose
ui.Row(c).Fill().Children(func() {
    sidebar(c)
    panel.View(c)                       // 面板 = 子进程的帧 + 输入转发 + resize
})

raw, err := sess.Call(ctx, "Child.Do", args...)   // 调子进程服务
sess.OnEvent("child.changed", func(p json.RawMessage) { ... })
```

**子进程**（本仓库已实现）：

```go
app := &myApp{}
err := adalink.Serve(adalink.ServeOptions{
    View:    app.view,      // 与 WindowOptions.Content 完全相同的 ui.View
    Service: app.call,      // 在 UI goroutine 上执行，可改 View 读的状态
    OnReady: func(c *adalink.Child) { _ = c.Emit("child.changed", app.state()) },
})
```

**上游后的理想形态**（P2，建议）：

```go
// ui: 不再依赖 Tester
surface := ui.NewSurface(app.view, ui.SurfaceOptions{Size: image.Pt(720, 520), Scale: 2})
surface.SendInput(ev)                 // 完整 ui.InputEvent
pix, stride, damage := surface.Frame() // premultiplied BGRA + 脏矩形
// mygo: 真实窗口的原始帧与注入
png/raw, damage := win.CaptureFrame()  // GPU 渲染器给 BGRA + damage
win.SendInput(ev)
// mygo: 跨进程 Bind，语义与页面一致
mygo.BindPeer(session, appServices, mygo.PeerOptions{Trusted: true})
```
