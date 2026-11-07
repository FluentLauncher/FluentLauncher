# 工程协作约定

- 默认使用简体中文交流，标识符、命令和错误保留原文。
- 文本文件统一 UTF-8 无 BOM；遵守 `.editorconfig`。
- 目标为 Windows 11 x64，C# / .NET 10 / WinUI 3；先读 `docs/project-plan.md` 和 `docs/TODO.md`。
- 用户选择轻量开发环境，默认 VS Code + .NET CLI；不将完整 Visual Studio 列为必装。winapp CLI 为可选的公开预览工具，不自动引入预览版项目依赖。
- 保持 KISS / YAGNI。Core 不引用 WinUI 或 Windows API；Windows 集成放在 Launcher.Windows。
- 不在 UI 线程扫描目录、读取图标或等待插件；不在搜索输入链路中执行数据库全表扫描。
- 插件为受信任的本地代码，进程隔离不等同于安全沙箱。
- 依赖使用中央版本管理，不使用浮动版本或预览版；更新后验证 SDK 和发布兼容性。
- 完成改动后运行适用检查。缺少 SDK 时明确记录未验证项，不声称编译或运行通过。
- 本地数据写入 `%LOCALAPPDATA%\FluentLauncher`，不向仓库写入个人历史、凭据或本机配置。
- 保留无关改动；完成任务和允许的验证后创建本地 Git 提交，未经授权不推送。
