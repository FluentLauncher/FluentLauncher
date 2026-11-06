# WinUI → Go/MyGo 复刻状态

日期：2026-11-07

## 已复刻

| 原 C# 模块 | Go 落点 | 状态 |
| --- | --- | --- |
| Launcher.Core/SearchEngine | internal/core/search/engine.go | 分档、拼音/首字母/acronym、别名、固定、最近使用、隐藏、稳定排序 |
| QueryCoordinator | internal/core/search/query.go | 前缀解析、并发来源、取消、增量合并、路径去重 |
| ResultCommands | internal/core/search/commands.go | 打开、管理员、位置、复制、插件动作/预览、别名/固定/隐藏 |
| UtilityProvider / Calculator | internal/core/search/utility.go | URL、路径、网页搜索、四则/括号/百分比/幂 |
| WebSearchSite | internal/core/settings/items.go + search/sites.go | 关键词触发、URL 编码、冲突提示 |
| CustomLaunchItem / Workspace | internal/core/settings/items.go + app/actions.go | 目标/参数/工作目录/管理员、顺序执行、失败隔离 |
| ScreenEdgeVolume | internal/core/volume + winapi/volume | 边缘命中、滚轮累积、Core Audio、钩子线程 |
| LauncherDatabase | internal/store/db.go | 三张表、个人规则、文件索引、WAL、备份 |
| LauncherSettingsStore/ConfigurationFiles | internal/store/config.go | 原子写、旧配置补齐、导入导出 |
| FileIndexService | internal/store/fileindex.go | 递归扫描、排除目录、差异写入、粗粒度监听 |
| ThemePackage | internal/store/theme.go | zip 导入导出、路径/大小限制 |
| StartMenuCatalog | internal/winapi/apps | Shell 应用列表、便携目录、PATH、监听 |
| WindowsApplicationLauncher | internal/winapi/apps/launch.go | ShellExecuteEx、runas、UAC 取消、打开位置 |
| EverythingProvider | internal/winapi/everything | WM_COPYDATA Unicode 查询 |
| WindowCatalog | internal/winapi/windowlist | EnumWindows、cloaked/tool 过滤、还原置前 |
| ShortcutResolver | internal/winapi/shortcut | WScript.Shell 读取 .lnk 目标/参数/工作目录，用于去重 |
| Plugin SDK/Host/Manager | internal/plugin + pkg/pluginsdk | 帧协议、进程、Job Object、超时/取消/回收/暂停、安装升级 |
| Echo/DevTools 插件 | plugins/echo、plugins/devtools | 可执行插件，行为按 C# 版本 |
| MainWindow / SettingsWindow | internal/app | 搜索面板、动作面板、设置页、浮层、工作区结果 |

## 仍需补齐

- Everything 只做了协议实现，尚未在本机真实客户端上做联调。
- 插件为新的可执行模型；旧 C# 插件不会运行（仓库会跳过 `entryAssembly` 包）。
- 主题的 Acrylic/Mica 强度由系统决定；C# 的透明度滑杆现在只影响图片遮罩，未对系统材质着色。
- 窗口位置记忆仍使用固定 28% 顶部位置；C# 的每屏锚点尚未迁移。
- 设置页的启动项/站点/工作区是基础编辑，缺少 C# 的完整校验提示与图标选择。
- 无障碍、全键盘动作面板、主题包预览和高对比度需要人工验收。
