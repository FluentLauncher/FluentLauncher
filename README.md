# FluentLauncher

Windows 11 键盘优先启动器，使用 Go + [MyGo](https://github.com/egoist/mygo) 原生 UI 实现，不依赖 WebView 或 .NET 运行时。

## 功能

- 全局热键、托盘、单实例、启动隐藏、开机启动。
- 应用/文件/自定义启动项/工作区/站点搜索/计算器/已开窗口/插件统一搜索。
- 中文全拼、首字母、别名、固定、隐藏、最近使用与稳定排序。
- 结果动作：打开、管理员运行、打开位置、复制路径/文本、插件动作。
- Acrylic / Mica / 纯色 / 渐变 / 图片背景；真透明、圆角、自定义配色与字号。
- 指定目录文件索引（SQLite，增量写入）；可选 Everything 连接器。
- 屏幕边缘滚轮调音量，带不抢焦点浮层。
- 可执行插件 + JSON-RPC over stdio；官方回显/开发工具插件。
- 设置窗口覆盖常规、外观、文件、音量、站点、启动项、插件与其他。

复刻覆盖与剩余项见 `docs/port-status.md`。

## 开发

```powershell
go tool mygo dev
go test ./...
go vet ./...
go tool mygo build -skip-installer
```

构建产物在 `build/windows-amd64/`。运行中的应用会锁住该目录，重新构建前先退出。

## 目录

```text
main.go                     # 入口
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

插件是独立可执行文件，`plugin.json` 的 `executable` 指向包内程序；stdin/stdout 使用 4 字节大端长度 + JSON-RPC 2.0。官方示例随 `resources/examples` 分发，可在设置页安装。

## 调试

```text
FL_DEBUG_QUERY=echo hello   # 启动后填入查询
FL_DEBUG_EXIT_MS=6000       # 启动后自动退出
```
