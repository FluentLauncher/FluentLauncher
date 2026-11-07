# 工程协作约定

- 默认使用简体中文交流，标识符、命令和错误保留原文。
- 文本文件统一 UTF-8 无 BOM；遵守 `.editorconfig`。
- 目标为 Windows 11 x64，Go 1.27 + MyGo 原生 UI；先读 `docs/port-status.md` 和 `README.md`。
- 用户选择轻量开发环境，默认 VS Code + Go 工具链；不引入需要完整 Visual Studio 的依赖。
- 保持 KISS / YAGNI。`internal/core` 不引用 MyGo 或 Windows API；Windows 集成放在 `internal/winapi`。
- 不在 UI 线程扫描目录、读取图标或等待插件；不在搜索输入链路中执行数据库全表扫描。
- 插件为受信任的本地代码，进程隔离不等同于安全沙箱。
- 依赖使用中央版本管理（`go.mod`），不使用浮动版本或预览版；升级后验证构建和发布。
- 完成改动后运行适用检查：`go build ./...`、`go vet ./...`、`go test ./...`、`go tool mygo vet .`。缺少环境时明确记录未验证项，不声称编译或运行通过。
- 本地数据写入 `%LOCALAPPDATA%\FluentLauncher`，不向仓库写入个人历史、凭据或本机配置。
- 保留无关改动；完成任务和允许的验证后创建本地 Git 提交，未经授权不推送。

## 界面与主题约定

所有窗口与视图都必须跟随主题，禁止写死颜色、字号或材质：

- 视图开头统一 `theme := *c.Theme(); a.applyUITheme(&theme); c.SetTheme(&theme)`，之后只从 `theme` 取色、字号、圆角和间距。
- 窗口背景统一走 `a.drawShell`（图片 / 蒙版 / 着色三层，固定顺序），不要再用绝对定位的背景盒。
- 控件的表面色用 `theme.Surface` / `theme.SurfaceHover` / `theme.Border`；需要与主题反差的浮层（提示、HUD）用 `theme.Inverse` / `theme.InverseText`，不要自定义深色。
- 亚克力与云母等材质由 `blursBehind` + `applyBackdrop` 统一开关；新增材质必须同时实现模糊、着色和设置页说明。
- 设置窗口的界面字号固定为 `settingsFontSize`；主题字号只作用于搜索面板。
- 不透明度滑块是窗口背景透明的唯一控制：背景色/gradient 的 alpha 一律忽略（`parseOpaqueColor`），图片、纯色、渐变、材质都按同一个 `Opacity` 计算。
- 新增外观项时必须同步四处：`settings.ThemeSettings`（含默认值与校验）、设置页控件、`drawBackground`/视图、以及 `panelSizePreview` 的缩略图，否则预览与真实窗口会不一致。
- 布局尺寸相关的改动要在 `ui.NewTester` 里补构建测试，并尽量用 `FL_DEBUG_*` 开关实机截图确认。