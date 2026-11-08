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
| 窗口材质与标题栏 | internal/winapi/glass + internal/app/chrome.go | 亚克力/云母走系统模糊，真实透出下层窗口；设置与工作区窗口自绘标题栏 |
| 音量浮层 | internal/app/volume.go | 底部居中的胶囊：图标、进度条、百分比，自身带模糊 |
| 插件 | internal/plugin + internal/app/settings_view.go | 独立进程、manifest 校验、状态/耗时/重启、每次搜索查询开关；设置页按插件展示状态与诊断 |
| 系统功能搜索 | internal/core/systemsearch + internal/core/search | 79 条 Windows 设置/控制面板/系统工具目录，支持中文同义词、英文名与拼音首字母；回车经 ms-settings: 或 ShellExecute 打开 |
| 设置保存 | internal/app/settings_view.go | 无保存/取消按钮，改动 600ms 后自动应用并保存；校验失败时回退已保存值并提示 |
| 托盘菜单 | internal/app/app.go | 使用系统菜单，跟随应用深/浅色（mygo 设置首选应用模式） |
| 选中态 | internal/app/view.go | 结果行/侧栏/下拉的选中用 42% 强调色着色 + 主题文字色，不再整块不透明确色 |
| 图标 | internal/app/app.go + view.go | 结果图标按 32 DIP、以 设备像素=32×缩放 向 shell 取图，避免 1.5× 放大导致的模糊 |
| 设置窗口 | internal/app/app.go | 默认 1180×840（更宽更矮），StateKey 换新以丢弃旧的过高窗口尺寸 |
| 弹出位置与尺寸 | internal/app/size_preview.go + internal/core/layout | 拖动缩略图或面板手柄设置尺寸与相对位置（AnchorX/AnchorY），按屏幕比例记忆；搜索框高度、结果行高度可调 |
| 界面国际化 | locales/（mygo ARB catalog）+ settings.Language | 简体中文（source）、繁体中文、英文；默认跟随系统语言，设置页可切换、立即生效；窗口标题/托盘菜单经 OnLocaleChanged 重建；系统功能目录三语显示，匹配关键词保持多语言 |
| 即时工具与内置图标 | internal/core/search/utility.go + internal/app/builtin_icons.go | date（含日期算术，月移钳到月末）/time/timestamp/uuid/days/week/rand/pass/单位换算（中英单位、温度特例）/b64 与 URL 编解码/颜色（HEX⇄RGB⇄HSL）关键词出结果、回车复制；计算器/网址/设置/工作区/插件等无文件图标的条目改绘内置线性图标 |
| 官方插件清单 | internal/plugin/official_index.json（embed） | 数据驱动三语清单，设置页下拉选择安装；新增官方插件不改 Go 代码 |

## 仍需补齐

- Everything 协议已实现并在本机真实客户端联调（修复过查询串 NUL 导致的 StringToUTF16 panic）；客户端未运行时给出提示而非崩溃。
- 插件为新的可执行模型；旧 C# 插件不会运行（仓库会跳过 `entryAssembly` 包）。
- 亚克力与云母都是「系统模糊 + 主题着色」：亚克力用主题背景色，云母取壁纸平均色，均不是 Windows 原生 Mica。
- 设置页的启动项/站点/工作区是基础编辑，缺少 C# 的完整校验提示与图标选择。
- 无障碍、全键盘动作面板、主题包预览和高对比度需要人工验收。
- 插件为独立进程，其 manifest 名称/描述与结果文本不随界面语言翻译（属于插件内容，不在 catalog 内）；官方示例插件仍是中文。
- 繁中用户输入繁体关键词可命中系统功能目录（繁体标题已并入关键词），但应用/文件的搜索匹配数据仍是用户自己的命名。
