# ada_link 传输协议（v1）

本文件是 `link` 包的规范：两个进程之间传什么、怎么传、什么时候算失败。实现见
`link/`，宿主/子进程的 MyGo 接入见 `mygoembed/`。

关键字 **必须**、**应该**、**可以** 按 RFC 2119 理解。

## 1. 角色与连接

- **宿主（host）**：拥有窗口与原生控件的 MyGo 应用。它创建会话目录，在目录里监听
  Unix domain socket，派生或连接子进程。
- **子进程（child）**：跑界面视图的进程。它拨号到宿主的 socket，创建帧环文件。

v1 只有一种方向：**宿主监听，子进程拨号**。宿主派生（`link.Spawn`）时把自己当作服务端，
所以子进程不会等宿主；`link.Dial` 供宿主之外的方式启动子进程（未来 "attach" 模式沿用同一
握手，只是角色相反）。

```
host                                      child
  │  listen(sessionDir/l.sock, 0600)         │
  │  spawn(env: SOCK, TOKEN, DIR, W, H, …)   │
  │                                          │  dial(SOCK)
  │                                          │  create(frames-1.bin), hello(token)
  │  read hello, validate token/proto        │
  │  open(frames-1.bin)                      │
  │  welcome ───────────────────────────────▶│
  │                                          │  frame loop
  │◀──────────────────────── frame(seq,slot) │
  │  copy pixels, frame-ack(seq) ───────────▶│
  │  input ─────────────────────────────────▶│
  │  call/reply/event ⇄                       │
  │  bye ───────────────────────────────────▶│
```

## 2. 帧格式

每条消息是 **u32 小端长度 + JSON 对象**。长度上限 16 MiB（`link.MaxMessage`）；
超限的连接被关闭，因为那是协议错误而不是大消息。像素**从不**走 socket：`frame` 消息只有
序号、槽位与脏矩形，像素在共享帧环里。

```
+--------+--------------------------------+
| u32 LE | JSON object (≤ 16 MiB)         |
+--------+--------------------------------+
```

JSON 字段名短，因为输入事件会频繁发送；未知字段必须忽略（向前兼容）。`t` 是消息类型。

## 3. 握手

### 3.1 hello（子进程 → 宿主，第一条消息）

```json
{"t":"hello","proto":1,"token":"<32 hex>","title":"child",
 "size":{"w":720,"h":520,"scale":1},
 "frame":{"file":"frames-1.bin","slots":3,"w":720,"h":520,"stride":2880,"scale":1}}
```

- `proto` 必须等于 1。宿主的 `welcome` 也带 `proto`；任何一方版本不符必须断开，不回退。
- `token` 是宿主派生时写入环境 `ADALINK_TOKEN` 的 128 位随机值，常量时间比较。
- `frame.file` 必须是会话目录里的**纯文件名**（不含 `/`、`\`、`..`）。宿主对每个
  `surface` 收到的文件名都重新校验。
- `frame.w/h/stride` 是**设备像素**；`size.w/h` 是设备无关像素（DIP）。`stride` 必须等于 `4*w`。
- `scale` 是设备像素每 DIP。

### 3.2 welcome（宿主 → 子进程）

```json
{"t":"welcome","proto":1,"size":{"w":720,"h":520,"scale":1},"title":"child"}
```

宿主先打开帧环并校验头部，再回 `welcome`；子进程收到 `welcome` 后才开始发帧。
宿主读 hello 或打开帧环失败时直接关闭连接、杀掉子进程、删除会话目录。

### 3.3 超时

- 宿主 accept 与读 hello 的期限由 `SpawnOptions.Timeout`（默认 20s）约束。
- 子进程拨号与读 `welcome` 的期限是 10s/20s。
- 期限过后连接关闭。没有重试：失败的会话由调用方重新 `Spawn`。

## 4. 帧环

帧环是一个**文件支撑的环形缓冲**，两个进程各自以读写方式打开它。

### 4.1 文件头（64 字节）

| 偏移 | 类型 | 含义 |
|---|---|---|
| 0 | 8 字节 | magic `"ADLNKFG1"` |
| 8 | u32 | 槽数（v1 固定 3） |
| 12 | u32 | 宽（设备像素） |
| 16 | u32 | 高（设备像素） |
| 20 | u32 | 行跨距 bytes（必须 `4*宽`） |
| 24 | f32 | scale（设备像素/DIP） |
| 28 | 36 字节 | 保留，写零 |

### 4.2 槽

槽 `i` 从偏移 `64 + i*stride*高` 开始，长 `stride*高` 字节：

- 像素格式为 **premultiplied RGBA8888**，行自上而下，与 Go `image.RGBA.Pix` 的布局一致。
- 不做压缩、不做颜色管理；合成由宿主的 GPU 采样。
- 槽的数量 3 给出最多 3 帧的流水线深度：子进程最多领先宿主 3 帧，多了就丢帧而不是阻塞。

### 4.3 同步

1. 子进程取一个**空闲槽**：`slotSeq[i] <= lastAck` 的槽。没有空闲槽时**丢弃这一帧**
   （计入 `dropped`），不阻塞输入与 RPC。
2. 子进程写像素，然后递增 `seq`，记录 `slotSeq[slot] = seq`，再发送
   `{"t":"frame","seq":N,"slot":K,"damage":[...]}`。写像素发生在上一条消息之后、
   这一条消息之前；socket 的系统调用构成顺序屏障，文件页在进程间是共享的。
3. 宿主收到 `frame` 后，校验 `seq > lastSeq`、`0 <= slot < slots`，把整个槽读进自己的
   帧缓冲（`link.Frame`），然后**在消费完像素之后**发送累计确认
   `{"t":"frame-ack","seq":N}`：确认 N 意味着 N 及以前的所有帧都可以回收。
4. 宿主没来得及消费的帧也**必须**被确认（丢弃时立即 ack），否则子进程的槽会永久占用。
5. `frame-ack` 是累计的，因此宿主合并多帧（只留最新）时只需确认最新一帧的 seq。

`damage` 是建议性的：`[{"x":…,"y":…,"w":…,"h":…}]` 给出相对上一帧的变化包围盒，
`w/h` 缺省或为 0 表示整帧。v1 宿主仍整帧上传纹理（`ui.Bitmap.Update` 的接口如此），
damage 留给 P2/P3 的部分上传与显示列表中继。

### 4.4 尺寸变化

宿主 `{"t":"resize","size":{"w":W,"h":H,"scale":S}}` 要求新的 DIP 尺寸与缩放。
子进程可以：

- 立即重建视图尺寸，创建**新文件**（如 `frames-2.bin`），先发送
  `{"t":"surface","frame":{…},"size":{…}}`，然后才发新环里的帧；旧环由子进程删除。
- 宿主收到 `surface` 时打开新环、换掉旧环并关闭旧环的映射。消息按顺序处理，
  因此 `surface` 之前不会有新环的 `frame`，之后也不会有旧环的 `frame`。

`seq` 跨环继续递增（宿主只按 `seq > lastSeq` 接受），因此换环不会重排或吞帧。

## 5. 输入

宿主把 `ui.InputEvent` 转成下面的对象，`{"t":"input","ev":{…}}`。坐标是**子进程区域内的
DIP**，原点在子进程画面的左上角；宿主元素的内容盒就是子进程的画面。

| 字段 | 类型 | 用于 | 含义 |
|---|---|---|---|
| `k` | string | 所有 | `down` `up` `move` `scroll` `zoom` `keydown` `keyup` `text` `compose` `command` `focus` `drop` `clipboard` |
| `x`,`y` | number | 指针 | 子进程区域内的 DIP |
| `b` | int | down/up | 0 主键、1 次键、2 中键 |
| `c` | int | down | 连续点击计数（双击为 2） |
| `dx`,`dy` | number | scroll | 滚动量，DIP；正值 `dy` 向下滚 |
| `s` | number | zoom | 缩放倍数（2 = 放大一倍） |
| `p` | bool | scroll | 触控板式精确滚动 |
| `m` | bool | scroll | 惯性滚动 |
| `key` | uint16 | keydown/up | `ui.Key` 的数值（布局相关） |
| `pk` | uint16 | keydown/up | `ui.Key` 的物理键位（US 布局） |
| `mods` | uint8 | keydown/up | `ui.Modifiers` 位掩码：Shift=1、Ctrl=2、Alt=4、Super=8（与 MyGo 一致） |
| `r` | bool | keydown | 自动重复 |
| `t` | string | text/compose/command/clipboard | 文本、组合串、命令名（`copy`、`paste`、`undo`…）或剪贴板文本 |
| `caret` | int | compose | 组合串中光标位置（rune 计） |
| `f` | bool | focus | 子进程是否拥有键盘 |
| `files` | string[] | drop | 文件路径 |
| `w` | bool | clipboard | 子进程复制（true）还是请求宿主剪贴板（false） |

规则：

- 按键的数值是 **MyGo 版本的 ABI**：`ui.Key` 与 `ui.Modifiers` 的值在编译期固定，
  两端必须由同一 MyGo 版本构建（`go.mod` 的版本即握手时要校验的 ABI）。`proto` 管消息
  形状，MyGo 版本管键值；未来把键名放进协议即可解耦。
- 宿主应把不属于面板的快捷键（`ui.Cmd` 修饰的键）留给宿主自己的菜单与快捷键，
  不发给子进程：面板是宿主应用的一部分，不是独立应用。
- 中键在 v1 的子进程实现（`ui.Tester`）里不可表达；宿主发送它，子进程记录并忽略。
  这是 `ui.Tester` 的缺口，不是协议的缺口；P2 的 `ui.Surface.SendInput` 接收完整
  `ui.InputEvent`。
- IME：宿主元素的 `InputCompose` 直接转发；v1 的候选窗由宿主窗口显示（`ui.TextCaret`
  指定的位置），子进程只渲染组合串。P2 才把候选窗真正锚定到远端输入框。

## 6. 调用、回复与事件

形状与 MyGo 页面 IPC 相同（`ipc.go` 的 `call`/`reply`/`event`），只是换成进程边界：

| 消息 | 字段 | 含义 |
|---|---|---|
| call | `id`,`m`,`a` | 调用方法 `m`，参数数组 `a`；`id` 由调用方在本连接内唯一 |
| reply | `id`,`ok`,`v` 或 `e` | `ok:true` 时 `v` 为结果；否则 `e` 为错误文本 |
| event | `n`,`p` | 有名字的带类型事件 |

- 方法名是 `Service.Method`。宿主是调用方时，方法属于子进程；子进程是调用方时，方法属于
  宿主。同一个连接上两个方向都可以 `call`。
- 每个 `call` 由接收方在自己的 goroutine 里执行，可以并发；参数与结果都是 JSON。
- `id` 只在一条连接里唯一；连接关闭后未回复的调用以错误结束。
- 事件在第一个监听者出现前**保留**，每个名字最多 64 条（`maxHeldEvents`），之后丢弃最旧的。
  这让子进程启动时发出的初始状态不会和宿主的监听注册竞争。
- 通道（流）在 v1 **没有**实现。MyGo 的 `Channel[T]` 绑在页面生命周期上（`channel.go:160`），
  跨进程版本要等控制层与 `ipc.Peer` 泛化一起做（见 `DESIGN.md` 第 8 节）；
  大流量数据（视频）应当走帧环或 P3 的共享缓冲，而不是 JSON 通道。

## 7. 关闭与错误

- 任一方向发送 `{"t":"bye"}` 后关闭连接。宿主还会在宽限期（2s）后杀掉子进程。
- `{"t":"error","e":"…"}` 报告不影响连接的错误；接收方记录。
- 连接中断、子进程退出、握手失败都终结会话：宿主关闭帧环、删除会话目录，`Session.Done()`
  关闭，`Session.Err()` 给出原因。
- 宿主删除会话目录发生在子进程退出之后；Windows 上打开的文件不能删除，所以子进程必须在
  退出前关闭帧环（实现里 `Serve` 在事件循环结束后关闭）。
- socket 路径来自会话目录；Unix socket 路径有 ~100 字节上限，因此会话目录建在系统临时目录
  下并以短随机名命名（参考 MyGo 单实例锁的做法，`single_instance.go:176`）。

## 8. 安全

| 资产 | 保护 |
|---|---|
| 控制 socket | 会话目录 0700，socket 0600；只有同一用户可以连接 |
| 帧环 | 文件 0600，会话目录 0700；只有宿主与子进程能读像素 |
| 会话标识 | 128 位随机 token，握手常量时间比较；失败的 hello 立即断开 |
| 文件名 | `frame.file` 只允许纯文件名，拒绝路径分隔符与 `..` |
| 帧头 | 宿主打开环后重新读取并校验 magic、尺寸、槽数与文件长度；不信任消息里的数字 |
| 资源 | 消息 ≤ 16 MiB、槽 ≤ 8、单边 ≤ 16384 像素、每模板 64 条保留事件、3 帧流水线 |
| 信任边界 | 子进程是宿主启动的代码，但它仍是**另一个进程**：它拿到的会话只允许帧、输入与 RPC，不能读宿主内存、不能伪造别的会话 |

不做的：不加密（本地同用户 socket + 随机 token 已够；跨用户/跨机器不在本方案内），
不鉴权子进程的**代码来源**（谁启动的子进程谁负责；插件市场/下载的子进程应当由宿主签名
校验后再启动）。

## 9. 版本与演进

- `proto` 是消息形状的版本，v1 为 1。新字段只增不改；未知字段忽略；未知消息类型接收方
  记录并忽略（`bye`/`error` 除外），以允许早期灰度。
- 键值 ABI 由 Go 模块版本决定。握手校验 `proto`；`go.mod` 里 `github.com/egoist/mygo`
  的版本是隐含的 ABI 版本。生产实现应把 MyGo 版本放进 hello 并校验。
- P2 计划新增：`access`（无障碍树片段）、`cursor`（光标形状）、`clipboard`（双向文本/图片）、
  `frame-part`（damage 部分上传）、`surface-mode`（`pixels` / `displaylist`）。
- P3 的显示列表中继是**新 `t` 值**而不是新协议：`{"t":"scene",…}` 替代 `frame`，
  像素回退为可选。
