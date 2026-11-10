# 平台支持与适配范围

当前状态：FluentLauncher 仅支持和发布 Windows 11 x64，尚未实现或验证 macOS / Linux 版本。

## 框架能力

项目使用 Go 1.27.3 和固定版本的 [MyGo v0.7.4](https://github.com/egoist/mygo/tree/v0.7.4)。该版本支持 Windows、macOS、Linux 的原生 UI 与平台构建。框架具备跨平台能力，但应用直接调用的 Windows API、运行库和系统功能需要分别适配。

## 本项目需要适配的部分

| 部分 | 当前实现 | 其他平台需要的工作 |
| --- | --- | --- |
| 搜索、匹配、界面 | `internal/core/search` 与 `internal/app` 的 MyGo 视图 | 复用通用逻辑与视图，调整平台按键、路径判断和文案；在目标平台验收字体、输入法与焦点 |
| 应用发现与启动 | `internal/winapi/apps`，开始菜单、`.lnk`、ShellExecute、runas | macOS 发现 `.app` 并启动应用；Linux 解析 `.desktop`；管理员操作按平台提供或隐藏 |
| 系统功能与文件搜索 | Windows 设置 URI、控制面板与 Everything | 按平台提供目录；不支持 Everything 的平台保留目录文件索引 |
| 已开窗口与材质 | EnumWindows、Windows 模糊和壁纸接口 | 提供平台实现，缺少能力时隐藏相应选项或采用明确的基础外观 |
| 屏幕边缘调音量 | Windows 鼠标钩子、Core Audio | 分别评估 macOS 权限、Linux X11 / Wayland 输入限制和音频接口 |
| 插件宿主与目录 | Windows 专用进程客户端、Job Object；目录选择固定 `windows-amd64` 包 | 通用协议可复用；适配进程启动和进程树回收，按系统和架构选择插件包；官方插件同步构建 |
| 数据与 SQLite | `%LOCALAPPDATA%`；嵌入 `mygo-sqlite3.dll` | 使用平台数据目录；按系统和架构提供 SQLite 动态库，或采用已验证的运行库加载方式 |
| 发布与更新 | `scripts/build-release.ps1` 和 Actions 固定 Windows x64 | 增加目标平台构建、包格式和更新清单；macOS 正式分发还需处理签名、公证与权限 |

`internal/app/app.go` 当前直接导入多处 `internal/winapi` 包，这些包及插件宿主带 Windows 构建约束；将构建目标改成 `darwin` / `linux` 时，应用尚无对应实现可链接。因此现在不能把现有 Windows 版本直接重新打包为 macOS / Linux 应用。

## 建议实施顺序

1. 先确定首个目标系统与 CPU 架构，列出该平台首版提供的能力。Linux 应进一步明确桌面环境与 X11 / Wayland；Windows 上的全部功能无法假设在各平台均等可用。
2. 在现有模块边界上拆分平台实现，优先覆盖数据目录、SQLite、应用发现与启动、插件进程管理，让主程序能够构建并启动。保持 `internal/core` 不依赖系统 API，不为尚未需要的能力预设复杂框架。
3. 保留搜索、文件索引、自定义启动项、站点、即时工具和受支持的插件。将设置页与动作菜单接入实际平台能力，避免展示无法执行的 Windows 专属选项。
4. 再适配材质、已开窗口、音量等系统集成，并补上目标平台插件包、安装包和更新流程。
5. 在目标系统实机验证全局热键、托盘、输入法、应用启动、插件退出与清理、数据持久化和更新；通过后再增加正式发布目标。

上述是后续适配方案，本次键盘操作改进没有引入其他平台实现或发布产物。
