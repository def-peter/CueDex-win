# CueDex for Windows — 交接文档

写给接手开发的 AI。读完全文再动代码，尤其是「不可推翻的决策」和「禁止事项」
两节：这里的每个结论都是带着代价算出来的，推翻它之前先看代价。

## 1. 这是什么

CueDex 是给编码助手用的注意力路由器：Codex 或 OpenCode 的主会话回复结束时，
用屏幕边缘发光、声音或语音提示用户回来。不落盘、不上传 prompt 与回复内容。

macOS 版（`D:\code\me\go\CueDex`，Swift）已上线。**本仓库是 Windows 11 的独立
实现，与 macOS 版不共享代码。**

## 2. 不可推翻的决策

| 决策 | 结论 | 理由 |
| --- | --- | --- |
| 语言 / 框架 | Go 1.27.3 + MyGo v0.7.4，纯原生 UI | 见下 |
| 平台 | 只做 Windows 11（x64、arm64） | 用户明确要求 |
| 项目结构 | 与 macOS 版分成两个仓库、两套发布 | 用户明确要求 |
| 产品名 | `CueDex`；identifier `com.peter.cuedex`；二进制 `CueDex.exe` | 产品就叫 CueDex；数据目录由 MyGo 的 `PathUserData` 决定，Windows 惯例是 `%APPDATA%\<Name>` |
| module | `github.com/def-peter/cuedex-win` | `cuedex` 被现有仓库大小写冲突占用，已无法使用 |

**为什么不是 Wails v3**：Wails 每个窗口是一个独立 WebView2 进程。发光覆盖层
需要每块显示器一个全屏窗口，三屏就是 240–450MB 常驻内存，而且为了让 3 秒的
提示立刻出现，这些窗口必须预热常驻。一个常驻托盘工具不能是这个形状。

**为什么不是 C#**：Windows-only 项目用 C# 本身没错，但本机当时只有 .NET
运行时没有 SDK，且用户已明确要求 Go。不要再提。

**为什么不是 cgo / C++**：MyGo 全平台 `CGO_ENABLED=0`，交叉编译干净，别破坏它。

## 3. 只做功能对齐，不做平台行为对齐

**必须和 mac 版一致的（功能语义）**

- 主会话成功回复结束才提示；子会话、失败、取消、等待输入都不提示
- 每个 source（codex / opencode）有独立的提示配置
- 暂停、免打扰时段、开机自启、单次 3 秒提示
- 集成必须显式启用，卸载时只删自己写的部分，保留用户其他 hook / 插件
- 60 秒新鲜度；`"<source>:<eventID>"` 去重；每个 source 只留一条待播
- 绝不持久化或外发 prompt / 回复内容

**必须按 Windows 习惯做，不要照抄 mac 的（实现与交互）**

- 系统音：mac 的 `Glass / Ping / Pop / Submarine / Hero / Funk` 在 Windows 不存在。
  用自带 WAV（`audio` 包），从 mac 已带的 4 个人声文件起步，不要伪造同名系统音。
- 设置界面：一个设置窗口（参考 `examples/menubar-native` 的托盘附着面板），
  **不要**复刻 mac 的 5 个 Settings 页签结构。
- 更新：NSIS 安装包 + `plugins/updater`。**不要**复刻 mac 的「下载 DMG + hdiutil 挂载 +
  手动拖拽」流程。
- 快捷键：Ctrl 系，不是 Cmd 系。
- 唤醒：单实例 + 二次启动转发 `--cue`，不是 `open -gj`。
- 失败提示：Windows 通知 / toast，不是 NSAlert。
- 语言：`i18n` + `locales/zh-Hans`，跟随系统并可运行时切换。
- 首启与签名：未签名 exe 面对的是 SmartScreen，不是 Gatekeeper。README 里
  mac 那段 Gatekeeper 说明不能照搬，要按 Windows 重写。

## 4. 当前状态（已验证，可直接信任）

```
go build ./...        exit 0
go test ./...         ok  1.4s（ui.NewTester 无窗口测试）
go vet / gofmt        干净
go tool mygo build    CueDex.exe 9.5 MB + NSIS 安装包 3.3 MB，18.3 s
```

NSIS 由 CLI 自动下载，包管理器 / 签名 / 更新都还没碰。

仓库：`main` 分支，首个提交 `3447d6f`，**尚未添加 git 远程**。
`main.go` 目前还是模板的计数器界面，等着被替换。

## 5. 现成的参考实现（比文档可靠）

MyGo 源码在本地 module cache，先定位：

```powershell
$d = (go list -m -f '{{.Dir}}' github.com/egoist/mygo)
```

| 示例 | 为什么值得读 |
| --- | --- |
| `examples/menubar-native` | 托盘 + 隐藏 frameless 面板 + `SkipTaskbar` + `OnBlur` 隐藏 + 右键 `PopUpMenu` 的完整骨架。**本项目的窗口结构照抄它。** |
| `examples/chat-glow` | 用 `ui` 画边缘辉光：`Box(c).Absolute().PassThrough()` 盖满窗口，`BorderConicGradient` 出渐层、`Blur` 出柔边，动画只重绘不重建 view。**本项目辉光的技术起点。** |
| `examples/confetti` | 「poses move in frames that only paint」——动画只重绘不重建 view，几百个元素也不卡。发光动画用同一手法。 |
| `examples/launcher` | `Panel: true` + `Transparent: true` 的浮层窗口。 |
| `examples/frameless` | `Transparent: true` 的最小用法。 |

可直接用的包：`audio`（WAV）、`plugins/watch`（目录监视，**不需要引入 fsnotify**）、
`i18n` + `locales/zh-Hans`、`plugins/updater`、`ui/shader`（`.kage` 着色器，可选）。

## 6. 第一步：发光覆盖层 spike（先做这个，别先写业务）

它决定产品在 Windows 上是否成立。目标程序 `cmd/glowspike`：

- 枚举显示器，每块屏一个全屏透明窗口
- 画四边带辉光，实现 `breathing` 与 `two-color flash` 两种动画
- 鼠标穿透、不进任务栏、不进 Alt+Tab
- 触发一次 3 秒后退出

窗口选项起点：

```go
mygo.WindowOptions{
    Transparent: true, Frameless: true, AlwaysOnTop: true,
    SkipTaskbar: true, DisableResize: true, DisableMove: true,
    DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
    Content: ui.View(glowView),
}
// 创建后：win.SetIgnoreMouseEvents(true)
```

**验收标准，逐条打勾**

1. 盖在普通窗口、最大化窗口、**无边框全屏**的终端/浏览器上可见
   （独占全屏可以不可见，属于可接受折损）
2. 点击、滚轮、悬停完全穿透到下层
3. 双屏 + 125% / 150% 缩放，位置与带宽正确；运行中改缩放不崩
4. 无提示时 RSS ≤ 25 MB，CPU 0%
5. 3 秒动画期间单核 ≤ 3%，RSS 增量 ≤ 10 MB
6. 从触发到首帧 ≤ 100 ms

**退路**（仅当 1 或 3 不达标）：改用 Win32 分层窗口
（`WS_EX_LAYERED|TRANSPARENT|TOOLWINDOW|NOACTIVATE`），只画四条边带，
预渲染渐变位图、每帧只改 alpha。成本约 300–400 行，已论证可行。

## 7. 待验证的未知（不要用 mac 经验假设）

1. **Codex 在 Windows 上如何执行 `hooks.json` 的 command** —— cmd、PowerShell
   还是直接 exec。这决定 `cuedex-notify` 写成 exe 还是脚本。必须真机验证。
2. Codex hooks 文件路径与 `CODEX_HOME` 在 Windows 的语义。
3. OpenCode 在 Windows 的全局插件目录是否仍是 `~/.config/opencode/plugins/`，
   还是 `%APPDATA%\opencode`；Node 下 XDG 的解析结果。
4. SAPI 语音枚举：用 `go-ole` 调 `SpObjectTokenCategory`，中文语音是否可用。
5. `App.SetOpenAtLogin` 写 Run 键时是否带后台参数。
6. `plugins/updater` 的实际能力：签名、静默安装、回滚。
7. 未签名 exe 的 SmartScreen 实际拦截面。

## 8. 跨项目契约

`PROTOCOL.md` 是唯一的共享面：事件目录、两种文件名、payload 四个字段、
16 KiB 上限、60 秒新鲜度、去重键、每个 source 一条待播、legacy 空文件规则。
**改事件格式先改它**，然后 mac 仓库同步。

## 9. 工程约定

- `gofmt`、`go vet` 必须干净；提交前跑 `go test ./...`
- 提交信息用 Conventional Commits
- view 用 `ui.NewTester` 做无窗口测试，参考现有 `main_test.go`
- 事件、偏好、播放编排这类纯逻辑要能脱离窗口测试
- 不要动 `D:\code\me\go\CueDex`（macOS 仓库）

## 10. 禁止事项

- 不引入任何 webview（Wails / Tauri / WebView2）
- 不引入 cgo
- 不为了「和 mac 一致」去复制 mac 的 UI 结构、系统音名、更新流程
- 不把 mac 的 Swift 文件逐行翻译成 Go，只读它的功能语义
- 不新增第二个语言或构建系统

## 11. mac 版哪些文件在功能上值得读

只读语义，不搬实现：

| 文件 | 读什么 |
| --- | --- |
| `CueEvent.swift`、`CueEventMonitor.swift` | 已经是 `PROTOCOL.md` 的来源，核对即可 |
| `AppModel.swift`、`CuePlaybackController.swift` | 队列与播放编排：一次一条、播完再下一条 |
| `CuePreferences.swift`、`PreferencesStore.swift` | 偏好字段清单：哪些全局、哪些每 source |
| `CodexIntegrationInstaller.swift` | hooks.json 的增删幂等性、保留用户其他 hook |
| `Resources/cuedex-opencode.js` | 插件的事件状态机（execution/step 判定），逻辑跨平台，可照着重写 |

**不要读 / 不要搬**：`ScreenGlowController.swift`（AppKit）、
`SoundController.swift`（NSSound）、`DmgUpdateDownloader.swift`（hdiutil）。
