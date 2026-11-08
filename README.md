# FluentLauncher

Windows 11 键盘优先启动器，使用 Go + [MyGo](https://github.com/egoist/mygo) 原生 UI 实现，不依赖 WebView 或 .NET 运行时。

## 功能

- 全局热键、托盘、单实例、启动隐藏、开机启动。
- 应用/文件/自定义启动项/工作区/站点搜索/计算器/已开窗口/插件统一搜索。
- 即时工具：`date`/`今天`（含 `date 2026-12-01 +3d` 日期算术）、`time`/`现在`、`timestamp`/`时间戳`、`uuid`、`days 2026-12-01`（距今天数）、`week`（ISO 周）、`rand 1-100`、`pass 16`、`10kg lb`/`32f c` 单位换算（含中文单位）、`b64`/`unb64`/`urlencode`/`urldecode`、`#3366AA`/`rgb 51,102,170` 颜色；回车即复制，Ctrl+K 动作面板给变体（UUID 大写/去横线/重新生成、日期中文格式/时间戳、颜色 rgb()/hsl()、换算带单位等），内置功能条目带专属图标。
- 系统功能搜索：输入“控制面板”“网络连接”“打印机”“电源”等关键词，直接打开对应的 Windows 设置页、控制面板项或系统工具。
- 中文全拼、首字母、别名、固定、隐藏、最近使用与稳定排序。
- 结果动作：打开、管理员运行、打开位置、复制路径/文本、插件动作。
- Acrylic / Mica / 纯色 / 渐变 / 图片背景；真透明、圆角、自定义配色与字号。
- 指定目录文件索引（SQLite，增量写入）；可选 Everything 连接器。
- 屏幕边缘滚轮调音量，带不抢焦点浮层。
- 可执行插件 + JSON-RPC over stdio；官方回显/开发工具/剪贴板历史插件。
- 设置窗口覆盖常规、外观、文件、音量、站点、启动项、插件与其他。
- 界面语言：简体中文、繁体中文、英文，默认跟随系统，可在设置中切换并立即生效。

复刻覆盖与剩余项见 `docs/port-status.md`。

## 开发

```powershell
go tool mygo dev
go test ./...
go vet ./...
go tool mygo build -skip-installer
```

界面文本在 `locales/*.json`（ARB，source 语言 zh-Hans，另有 en、zh-Hant）。改动 catalog 后运行 `go tool mygo generate` 重新生成 `locales/catalog_gen.go`（随仓库提交），并用 `go tool mygo i18n check .` 校验三语占位符与复数形式。

构建产物在 `build/windows-amd64/`。运行中的应用会锁住该目录，重新构建前先退出。

## 目录

```text
main.go                     # 入口
locales/                    # 界面语言目录（ARB）+ 生成的消息函数
internal/app/               # 生命周期、查询、动作、设置、浮层
internal/core/model/        # Entry/Profile/Result
internal/core/settings/     # settings.json 模型与校验
internal/core/search/       # 匹配、排序、查询、动作、计算器、站点
internal/core/layout/       # 弹窗定位
internal/core/volume/       # 边缘命中与滚轮累积
internal/store/             # SQLite、配置、文件索引、主题包
internal/plugin/            # 协议、客户端、进程管理、安装
pkg/pluginsdk/              # 第三方 Go 插件 SDK
plugins/echo/               # 官方回显示例
plugins/devtools/           # 官方开发工具插件
resources/examples/         # 随程序分发的插件包
```

## 数据

沿用 C# 版本的位置：

- `%LOCALAPPDATA%\FluentLauncher\settings.json`
- `%LOCALAPPDATA%\FluentLauncher\data\launcher.db`
- `%LOCALAPPDATA%\FluentLauncher\plugins`
- `%LOCALAPPDATA%\FluentLauncher\themes`

首次由新程序打开数据库时会生成 `launcher.db.bak-0.1.0` 备份。

## 插件

插件是独立可执行文件，`plugin.json` 的 `executable` 指向包内程序；stdin/stdout 使用 4 字节大端长度 + JSON-RPC 2.0。官方示例随 `resources/examples` 分发，清单在 `internal/plugin/official_index.json`（含三语名称与简介，新增官方插件只需加目录与清单项），设置页以下拉选择安装。

## 调试

```text
FL_DEBUG_SHOW=1             # 忽略“启动时隐藏”并显示面板
FL_DEBUG_QUERY=echo hello   # 启动后填入查询
FL_DEBUG_OPEN_SETTINGS=1    # 启动后打开设置窗口
FL_DEBUG_SETTINGS_PAGE=1    # 配合上一项，直接打开指定设置页（0 起）
FL_DEBUG_VOLUME=64          # 固定显示音量浮层，便于调整样式
FL_DEBUG_TRAY_MENU=1        # 启动后弹出托盘菜单，便于检查菜单项
FL_DEBUG_EXIT_MS=6000       # 启动后自动退出
```
