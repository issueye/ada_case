# 作为库使用（ada_link）

`ada_link` 是一个 Go 模块：把一个 MyGo 应用的 native UI 嵌入另一个 MyGo 应用的窗口，
两个进程之间带类型化调用、事件、帧环与输入转发。演示程序（`cmd/`、`examples/`）只是它的
使用者。

- 模块路径：`github.com/learningdog1/ada_link`
- 依赖：`github.com/egoist/mygo`（本仓库开发时用 `replace ../mygo`）
- Go：1.27+
- 平台：Windows / macOS / Linux（安装器仅 Windows）

## 1. 引入

发布后：

```sh
go get github.com/learningdog1/ada_link@latest
```

在同级目录开发（像本仓库这样）：

```go
// go.mod
require github.com/learningdog1/ada_link v0.0.0

replace github.com/learningdog1/ada_link => ../ada_link
```

本仓库的 `go.mod` 里还有 `replace github.com/egoist/mygo => ../mygo`，用于对着本地
MyGo checkout 开发；**replace 不会被下游继承**。所以使用本库的项目要自己决定 MyGo 用哪个
版本：

```go
require github.com/egoist/mygo v0.7.4   // 发布版
// 或对着本地 checkout：
replace github.com/egoist/mygo => ../mygo
```

## 2. 包结构

| 导入路径 | 用途 | 依赖 |
|---|---|---|
| `github.com/learningdog1/ada_link` | 便捷入口：`Spawn`/`Attach`/`Serve` 与常用类型别名 | MyGo + ui |
| `.../link` | 传输、协议、帧环、输入、调用/事件；不含 MyGo 类型 | 纯 Go |
| `.../mygoembed` | MyGo 接入：宿主视图与子进程离屏渲染 | MyGo + ui |
| `.../panels` | 多面板注册表、事件命名空间与广播 | MyGo + ui |
| `.../install` | Windows 按用户安装/卸载 | 纯 Go（`x/sys`） |

只做进程间 IPC、不需要 UI 的项目只导入 `link`；嵌入 native UI 时导入根包（或
`mygoembed`）即可。

```go
import (
    adalink "github.com/learningdog1/ada_link"
    "github.com/learningdog1/ada_link/link" // 需要更细的 API 时
)
```

## 3. 最小宿主

```go
sess, err := adalink.Spawn(adalink.SpawnOptions{
    Command: []string{childExe},   // 子进程可执行文件与参数
    W: 720, H: 520, Scale: 2,      // 初始 DIP 尺寸与设备像素/DIP
    Title: "Child",
})
if err != nil {
    log.Fatal(err)
}
defer sess.Close()

var panel *adalink.Host
var win *mygo.Window
mygo.App.WhenReady(func() {
    win = mygo.NewWindow(mygo.WindowOptions{
        Title: "Shell", Width: 1080, Height: 700,
        Content: ui.View(func(c *ui.Context) {
            ui.Row(c).Fill().Children(func() {
                sidebar(c)       // 宿主自己的控件
                panel.View(c)    // 子进程的画面
            })
        }),
    })
    panel = adalink.Attach(win, sess)
})
if err := mygo.App.Run(); err != nil {
    log.Fatal(err)
}
```

`Attach` 会：

- 接管子进程的帧：主线程合并，只保留最新一帧，写进 `ui.Bitmap`；
- 把窗口的指针、滚轮、键盘、文本、IME 组合、焦点、拖放转给子进程；
- 按元素的实际尺寸与 `Painter.Scale()` 请求子进程重新渲染（不是拉伸位图）；
- 在窗口关闭时关闭会话。

宿主窗口**必须**显示 native UI（`WindowOptions.Content`）；纯 web 页面窗口没有放原生视图
的那一层。

## 4. 最小子进程

```go
func main() {
    app := &myApp{}
    if os.Getenv(link.EnvSocket) != "" {
        // 由宿主启动：离屏渲染，把帧与输入接进 link。
        if err := adalink.Serve(adalink.ServeOptions{
            View:    app.view,     // 与 WindowOptions.Content 相同的 ui.View
            Service: app.call,     // 宿主调用；在 UI goroutine 上执行
            Title:   "Panel",
        }); err != nil {
            log.Fatal(err)
        }
        return
    }
    // 单独启动：普通窗口，同一份 UI。
    mygo.App.WhenReady(func() {
        mygo.NewWindow(mygo.WindowOptions{Title: "Panel", Content: ui.View(app.view)})
    })
    mygo.App.Run()
}
```

`Serve` 读取宿主在环境里放的东西（socket、token、目录、尺寸），所以子进程侧通常只调用
`Serve`。同一个二进制可以同时是"可嵌入的子进程"和"能单独运行的应用"，演示里的
`AdaPanel.exe` 就是如此。

`ServeOptions.Service` 在子进程的 UI goroutine 上执行，可以安全地修改 `View` 读的状态；
返回后自动请求下一帧。

## 5. 调用与事件

子进程服务：`Service(method, args)` 是路由表，运行在 UI goroutine：

```go
func (a *app) call(method string, args []json.RawMessage) (any, error) {
    switch method {
    case "Counter.Increment":
        a.n++
    case "Counter.State":
    default:
        return nil, fmt.Errorf("no method %q", method)
    }
    return state{N: a.n}, nil
}
```

宿主调用：

```go
raw, err := sess.Call(ctx, "Counter.Increment")
// 或类型化：
res, err := link.Call[Req, Res](ctx, sess.Peer(), "Counter.Increment", req)
```

子进程调用宿主（在 `OnReady` 里或任何时候）：

```go
raw, err := c.Call(ctx, "Host.Ready")
link.HandleFunc[Req, Res](c.Peer(), "Host.DoSomething", func(ctx context.Context, req Req) (Res, error) { ... })
```

事件：

```go
sess.OnEvent("child.changed", func(p json.RawMessage) { ... })   // 宿主
c.Emit("child.changed", state{N: a.n})                           // 子进程
c.OnEvent("host.push", func(p json.RawMessage) { ... })          // 子进程
```

事件在第一个监听者出现前保留（每个名字最多 64 条），所以子进程启动时 `Emit` 的初始状态
不会因为宿主还没注册监听而丢失。

方法名约定是 `Service.Method`；参数与结果是 JSON。宿主页面调用子进程服务时，建议以命名
空间另绑（`link:child.Service.Method`）并沿用 MyGo 页面的信任检查，而不是把 `Session`
直接暴露给页面。

## 5.1 多面板：`panels`

宿主嵌入多个面板时，用 `panels.Registry` 统一管理会话、状态、窗口与事件：

```go
import "github.com/learningdog1/ada_link/panels"

reg := panels.New()
sessA, _ := adalink.Spawn(...)
panelA, _ := reg.Add("a", "Panel A", sessA)
reg.Attach("a", win)                                  // 帧放进窗口
reg.OnEvent("changed", func(e panels.Event) {          // 所有面板，现在与以后
    log.Printf("%s: %s", e.Topic, e.Payload)          // panel/a/changed
})
reg.Broadcast("theme", Theme{Dark: true})             // 发给所有 Running 面板
```

- ID 必须唯一、不含 `/` 与空白；事件 topic 为 `panel/<id>/<name>`。
- `List`/`Get`/`OnChange` 给出 `Running`/`Closed`/`Failed` 与结束原因。
- `Broadcast` 只发给 `Running`，并发发送，按面板返回成功/失败；`Include`/`Exclude` 可选。
- 事件监听器在会话读协程上执行，不能阻塞。

Registry **不是**事件总线：它不做 N×M 扇出、latest 值缓存与每订阅者队列。需要这些时，
在它之上再加一个进程内 Hub（设计取舍见 [`docs/panels.md`](panels.md) 第 3 节）。

## 6. 线程、坐标与 DPI

- 宿主侧所有 `ui` 操作在主线程；`Host` 已经把帧合并到主线程，回调里只需 `Release` 帧
  （`Host` 自己处理）。
- 子进程侧 `ui.Tester` 只由它自己的 UI goroutine 使用；服务、输入、`Refresh` 都排队到那里。
- 输入坐标是**元素内容盒内的 DIP**；宿主元素多大，子进程就渲染多大，通常是 1:1 像素。
- 元素尺寸变化会触发 `Session.Resize`；子进程会合并 80 ms 内的连续尺寸变化，避免布局抖动
  造成每帧重建帧环。宿主在窗口 resize 时强制重绘，并在新帧到达前用 `Contain` 显示旧帧，
  不会把旧尺寸的画面拉伸到新尺寸。
- 宿主的 `Cmd`/`Super` 快捷键不转发（面板属于宿主应用），但 `copy`/`paste` 等
  `InputCommand` 会转发。
- **给 native UI 宿主窗口设置不透明的 `WindowOptions.BackgroundColor`**（如 `"#ffffff"`）。
  窗口在 live resize 时，新暴露的边缘先由窗口背景填充；没有背景色时 Windows 的
  redirection bitmap 可能在那里显示黑色或上一帧。若用截图工具按**整个窗口**截图，边缘
  还会带上窗口阴影与非客户区（约 8 px 的灰边），那不是界面内容；用 `-shot` /
  `CapturePage` 只截客户区即可分辨。

## 7. 生命周期

- `Session.Done()` 在会话结束时关闭；`Err()` 给出原因；`OnClose(fn)` 注册回调。
- 子进程退出、socket 断开、宿主 `Close()` 都终结会话；宿主会杀掉还在运行的子进程，
  删除会话目录。
- 崩溃恢复交给使用方：看 `Done()`，给面板显示占位与"重启"，指数退避后再 `Spawn`。
  会话目录是随机短名，不会与旧会话冲突。

## 8. 安装（Windows，可选）

```go
cfg := install.Config{
    Name:      "My App",
    Version:   "1.0.0",
    Publisher: "me",
    Exes: map[string]string{
        "MyApp.exe":  exe,                       // target -> source
        "Helper.exe": filepath.Join(dir, "Helper.exe"),
    },
    Host: "MyApp.exe",
    Shortcuts: []install.Shortcut{
        {Name: "My App", Exe: "MyApp.exe", Desktop: true},
    },
    UninstallArgs:      []string{"--uninstall"},
    QuietUninstallArgs: []string{"--uninstall", "--yes"},
}

res, err := install.Install(cfg)          // 文件、Start Menu/桌面快捷方式、Settings 卸载项
needQuit, err := install.Uninstall(cfg)   // 运行在安装目录里时 needQuit 为 true：退出后删目录
installed, dir := install.Status(cfg)
```

宿主程序需要处理 `--uninstall`（以及 `--yes`/`--quiet`）参数。`install` 在非 Windows 上
返回 `install.ErrUnsupported`，其他功能仍可编译。正式发布仍应使用 MyGo 自带的
`mygo build` 安装包与签名；`install` 覆盖的只是"按用户、无管理员、装得上卸得干净"。

## 9. 版本与兼容

- `adalink.Version` 是库版本。
- `link.Proto` 是线协议版本（v1）；两端握手校验，不匹配立即断开。
- 键值（`ui.Key`/`ui.Modifiers` 的数值）与 MyGo 版本绑定，两端必须由兼容的 MyGo 构建。
  协议文档第 5 节说明了这一点。
- 升级本库时同时检查 `DESIGN.md` 的 P2 路线：`ui.Surface`、原始帧捕获、`ipc.Peer`
  泛化会替换 v1 的 `ui.Tester` 路径。

## 10. API 一览

| 符号 | 说明 |
|---|---|
| `adalink.Spawn` / `link.Spawn` | 启动子进程并握手 |
| `adalink.Attach` / `mygoembed.Attach` | 把会话接进 native UI 窗口 |
| `adalink.Serve` / `mygoembed.Serve` | 在子进程里跑 `ui.View` |
| `Session.Call` / `Call[T]` | 调用、类型化调用 |
| `Session.Handle` / `HandleFunc[T]` | 服务宿主/子进程的调用 |
| `Session.Emit` / `OnEvent` | 事件 |
| `Session.Resize` / `Surface` / `Stats` | 尺寸、表面与统计 |
| `Child.Emit` / `Call` / `OnEvent` / `Refresh` | 子进程侧 |
| `Host.View` / `Host.Ready` / `Host.Session` | 宿主侧 |
| `panels.Registry` / `Panel` / `Event` / `Change` | 多面板注册、命名空间事件与广播 |
| `install.Install` / `Uninstall` / `Status` | Windows 安装 |

完整注释见 `go doc github.com/learningdog1/ada_link`、`go doc .../link`、
`go doc .../mygoembed`、`go doc .../install`。

## 11. 仍然存在的限制

v1 的子进程用 `ui.Tester`（测试取向的 API）：CPU 光栅、无公共无障碍树/光标/真实 IME
候选窗、20 帧 settle 上限。`DESIGN.md` 的 P2 给出生产路径（`ui.Surface`、
`Window.CaptureFrame`、`Window.SendInput`、`ipc.Peer` 泛化）。在这些落地之前，本库适合
"进程隔离的 native UI 面板"，不适合 4K 全屏视频级渲染。

## 12. 示例与测试

- [`examples/minimal`](../examples/minimal)：最小宿主 + 子进程，一个文件跑两边。
- [`examples/multi`](../examples/multi)：两个面板、一个 `panels.Registry`、命名空间事件与广播。
- [`cmd/linkdemo`](../cmd/linkdemo)：无窗口端到端，打印耗时并写 PNG。
- [`cmd/linkapp`](../cmd/linkapp) / [`cmd/panel`](../cmd/panel)：完整演示（安装、窗口、
  单独启动的子进程）。
- `go test ./...` 与 `go test -race ./...` 覆盖传输、帧环、输入、调用、resize 与真实子进程。
