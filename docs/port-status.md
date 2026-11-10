# WinUI → Go/MyGo 复刻状态

日期：2026-11-10

## 已复刻

新增发布能力：GitHub Actions 生成 Windows x64 单文件 EXE，官方插件不嵌入；签名正式版本支持后台检测、手动检测和原地更新后重启。配置与验证边界见 docs/releasing.md。

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
| Plugin Host/Manager | internal/plugin | 帧协议、进程、Job Object、超时/取消/回收/暂停、按需下载安装与独立升级 |
| SDK 与 Echo/DevTools/Clipboard 插件 | 独立 FluentLauncher-plugins 仓库 | 标准库 SDK；三个可执行插件与开发模板独立构建发布，剪贴板历史仅内存 50 条 |
| MainWindow / SettingsWindow | internal/app | 搜索面板、动作面板、设置页、浮层、工作区结果 |
| 窗口材质与标题栏 | internal/winapi/glass + internal/app/chrome.go | 亚克力模糊透出下层窗口；云母将壁纸降采样并漫射成空间纹理，按主题调明度与弱色彩，支持独立的背景透明度；设置与工作区窗口自绘标题栏 |
| 音量浮层 | internal/app/volume.go | 底部居中的胶囊：图标、进度条、百分比，自身带模糊 |
| 插件 | internal/plugin + internal/app/settings_view.go | 独立进程、manifest 校验、状态/耗时/重启、每次搜索查询开关；设置页按插件展示状态与诊断 |
| 系统功能搜索 | internal/core/systemsearch + internal/core/search | 79 条 Windows 设置/控制面板/系统工具目录，支持中文同义词、英文名与拼音首字母；回车经 ms-settings: 或 ShellExecute 打开 |
| 设置保存 | internal/app/settings_view.go | 无保存/取消按钮，改动 600ms 后自动应用并保存；校验失败时回退已保存值并提示 |
| 统一快捷键 | internal/core/shortcuts + internal/app/shortcuts*.go | 内置命令与插件命令统一设置、录制/多绑定/启停/冲突替换；全局差异注册与失败恢复；插件 manifest commands、可选 executeCommand 能力和后台按需调用；详情见 docs/shortcuts.md |
| 托盘菜单 | internal/app/app.go | 使用系统菜单，跟随应用深/浅色（mygo 设置首选应用模式） |
| 选中态 | internal/app/view.go | 结果行/侧栏/下拉的选中用 42% 强调色着色 + 主题文字色，不再整块不透明确色 |
| 图标 | internal/app/app.go + view.go | 结果图标按 32 DIP、以 设备像素=32×缩放 向 shell 取图，避免 1.5× 放大导致的模糊 |
| 设置窗口 | internal/app/app.go | 默认 1180×840（更宽更矮），StateKey 换新以丢弃旧的过高窗口尺寸 |
| 弹出位置与尺寸 | internal/app/size_preview.go + internal/core/layout | 拖动缩略图或面板手柄设置尺寸与相对位置（AnchorX/AnchorY），按屏幕比例记忆；搜索框高度、结果行高度可调 |
| 界面国际化 | locales/（mygo ARB catalog）+ settings.Language | 简体中文（source）、繁体中文、英文；默认跟随系统语言，设置页可切换、立即生效；窗口标题/托盘菜单经 OnLocaleChanged 重建；系统功能目录三语显示，匹配关键词保持多语言 |
| 即时工具与内置图标 | internal/core/search/utility.go + internal/app/builtin_icons.go | date（含日期算术，月移钳到月末）/time/timestamp/uuid/days/week/rand/pass/单位换算（中英单位、温度特例）/b64 与 URL 编解码/颜色（HEX⇄RGB⇄HSL）关键词出结果、回车复制；计算器/网址/设置/工作区/插件等无文件图标的条目改绘内置线性图标 |
| 官方插件清单 | internal/plugin/catalog.go | 在线签名三语目录、缓存、固定版本 ZIP 与哈希校验；内置名称仅作无下载能力的回退 |

## 仍需补齐

- Everything 协议已实现并在本机真实客户端联调（修复过查询串 NUL 导致的 StringToUTF16 panic）；客户端未运行时给出提示而非崩溃。
- 插件为新的可执行模型；旧 C# 插件不会运行（仓库会跳过 `entryAssembly` 包）。
- 为支持背景透明度，云母使用壁纸漫射模拟，未启用不透明的 Windows 原生 Mica；壁纸不可读时使用主题底色。背景不透明度与云母强度独立：前者只影响背景 alpha，后者控制主题亮度范围内的柔和光影和弱色彩，文字与控件不随背景淡出；与亚克力的实时后景模糊分开。
- 设置页的启动项/站点/工作区是基础编辑，缺少 C# 的完整校验提示与图标选择。
- 动作面板默认支持 Ctrl+K / Shift+F10 / 菜单键打开，方向键选择、回车或 Alt+1–9 执行，Esc / Ctrl+K 返回；均可在快捷键页改键，自动化回归覆盖输入框/列表焦点、旧键停用、新键生效及返回查询。无障碍、主题包预览和高对比度仍需人工验收。
- 插件为独立进程，其 manifest 名称/描述与结果文本不随界面语言翻译（属于插件内容，不在 catalog 内）；官方示例插件仍是中文。
- 繁中用户输入繁体关键词可命中系统功能目录（繁体标题已并入关键词），但应用/文件的搜索匹配数据仍是用户自己的命名。
- 截图插件（official.screenshot）方案已定于 `docs/plugin-screenshot.md`，待实现：全屏/窗口/区域/延时截图；交互与延时捕获走异步 execute 规避宿主 RPC 超时；滚动长截图与标注列 v2。取色器、汇率按用户决定不做。
