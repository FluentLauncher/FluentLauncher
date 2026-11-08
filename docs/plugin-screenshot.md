# 截图插件方案（official.screenshot）

状态：**设计稿，未实现**。实现前请先过一遍「风险与决策」一节，有变化先改本文档。

## 1. 定位

独立官方插件，触发词 `截图` / `screenshot` / `snap` / `截屏`。把"截图"做成一组小功能的入口，而不是单一动作：查询出菜单行，选哪行做哪件事。插件内容文本为中文（与 echo/devtools/clipboard 一致，不进主程序 catalog）。

隐私边界（写进 manifest 描述与索引简介）：**只在用户明确选择动作时捕获屏幕，从不后台截图**；捕获结果只去剪贴板或用户指定目录，不上传、不留额外副本。

## 2. 功能清单

| # | 功能 | 查询形态 | 输出 | 阶段 |
|---|---|---|---|---|
| F1 | 全屏截图→剪贴板 | `截图` 菜单行 | PNG 进剪贴板 | M1 |
| F2 | 全屏截图→存文件 | `截图` 菜单行 | PNG 存盘（默认目录+时间戳文件名） | M1 |
| F3 | 打开截图保存目录 | `截图` 菜单行 | ShellExecute 打开目录 | M1 |
| F4 | 窗口截图 | `截图 窗口 [过滤词]` 出窗口列表行，选中即截 | PrintWindow 捕获指定窗口 | M2 |
| F5 | 区域截图 | `截图` 菜单行 → 全屏遮罩拖框 | 交互选区后复制/存盘 | M3 |
| F6 | 延时截图 | `截图 延时 3` / `截图 3s` | 倒计时后全屏捕获 | M4 |
| F7 | 滚动长截图 | — | 自动滚动+拼接 | **v2 观望**，不做进首版 |

查询路由（插件无状态，靠查询文本分阶段）：

- 空/`截图` → 菜单行（F1/F2/F3/F5/F6 入口，各带说明副标题）。
- `截图 窗口 [filter]` → 枚举可截窗口（标题+进程名），filter 做大小写不敏感子串过滤；每行一个 invoke 动作截该窗口。
- `截图 延时 N`（N=1–30 秒，缺省 3）→ 一行确认，invoke 后倒计时捕获。
- 其余文本 → 菜单行按子串过滤。

动作完成后的反馈：宿主对 invoke 成功统一显示"已执行：{动作名}"；存盘类动作插件内部再 ShellExecute 打开所在目录可选（默认**不**打开，避免打扰，副标题里写明保存位置）。

## 3. 交互设计（区域截图 F5）

1. invoke 后插件在**独立 GUI 线程**创建覆盖整个虚拟屏幕的分层窗口（`WS_EX_LAYERED|TOPMOST|TRANSPARENT`，先 `SetLayeredWindowAttributes` 半透明黑遮罩 α≈80）。
2. 鼠标拖出选区：实时画 1px 亮边矩形 + 选区外保持遮罩（四块遮罩拼出"挖洞"效果）+ 选区角标显示 `W×H`。
3. 松开/回车确认 → 按选区从屏幕 DC 抓取；Esc/右键取消 → 什么都不做。
4. 捕获完成窗口自毁，焦点尽力还原（`SetForegroundWindow` 回原前台，失败不报错）。
5. 结果默认进剪贴板（与 F1 一致）；`截图 区域 存` 变体存盘。

窗口截图 F4 不需要遮罩：直接出结果行选择，PrintWindow 离屏捕获，不抢焦点。

## 4. 技术要点（Windows）

- **捕获**：全屏/区域用 `GetDC(0)+BitBlt`；窗口用 `PrintWindow(hwnd, hdc, PW_RENDERFULLCONTENT=2)`（能抓到 DWM/硬件加速内容，普通 BitBlt 抓窗口会是黑/残影）。
- **多显示器**：按虚拟屏幕坐标（`SM_XVIRTUALSCREEN/SM_CXVIRTUALSCREEN`）整块抓，负坐标显示器也覆盖。
- **DPI**：进程启动即 `SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2)`，否则缩放屏上坐标错位、图像被系统拉伸模糊。坐标换算用 `GetDpiForWindow`/`PhysicalToLogical`。
- **编码**：Go 标准库 `image/png`，无新依赖。
- **剪贴板**：优先 `RegisterClipboardFormatW("PNG")` 写 PNG 字节（Win10 1809+ 资源管理器/微信/浏览器都认），同时写 `CF_DIB` 兜底老应用；`GlobalAlloc(GMEM_MOVEABLE)+OpenClipboard+SetClipboardData+CloseClipboard`，失败回退 BMP。
- **存盘**：默认 `%USERPROFILE%\Pictures\FluentLauncher\`（可用 `截图 目录 <path>` 记忆？首版不做记忆，固定默认目录+副标题明示），文件名 `Screenshot_20261108_153012.png`；目录不存在则创建。
- **窗口枚举**：插件内复制一份 windowlist 的过滤逻辑（可见、非 tool、非 cloaked、非自身进程），约 60 行；不依赖主程序。
- **GUI 线程模型**：`runtime.LockOSThread()` 的专用 goroutine 跑遮罩窗口的消息循环；SDK 的 stdin 服务循环留在主 goroutine，两者用 channel 交接"开始/结果/取消"。
- **超时模型（关键）**：宿主 execute RPC 有超时（默认 1.5s，上限 30s），**交互/延时捕获必然超时**。设计为：F4/F6/F5 的 execute **立即返回"已启动"**，捕获在插件内异步完成（结果落剪贴板/磁盘）；F1/F2 全屏捕获同步完成（<100ms）。宿主侧无需任何改动。

## 5. 与宿主/SDK 的对接

- `pluginsdk.Handler`：`Query` 按 §2 路由出菜单行/窗口行；`Execute` 对即时动作同步做、对交互动作起 goroutine 后立刻返回 nil。
- manifest：`id=official.screenshot`，`executable=screenshot.exe`，`keywords=["截图","screenshot","snap","截屏"]`，`permissions=[]`（Windows 截屏无需特殊权限），描述含隐私边界。
- 分发：`resources/examples/official.screenshot/` + `official_index.json` 三语条目（截图工具 / Screenshot / 螢幕截圖）+ `scripts/build-plugins.ps1` 一行。
- 图标：宿主侧 builtin_icons 无需新增（插件行目前无图标，与其他插件一致）。

## 6. 风险与已知限制

| 项 | 说明 | 对策 |
|---|---|---|
| 独占全屏游戏 | BitBlt/PrintWindow 抓到黑屏 | 文档注明；检测失败时提示"目标可能处于独占全屏" |
| DRM/受保护内容 | 部分浏览器视频、金融软件输出黑块 | 系统行为，无法绕过，注明 |
| UAC/高完整性窗口 | 中完整性进程截不到 | 注明；窗口列表里这类窗口截出黑图时给错误提示 |
| 交互捕获超时 | 见 §4 超时模型 | execute 立即返回，异步完成 |
| 焦点抢占 | 遮罩窗口必须拿焦点收 Esc | 完成后尽力还原；F4/F6 不抢焦点 |
| 剪贴板被占用 | OpenClipboard 失败（别的应用锁着） | 重试 3 次（间隔 50ms），仍失败返回错误文本 |
| Go 窗口消息循环 | 插件进程首次做 GUI，线程模型易错 | M3 单独里程碑，先做最小遮罩验证再加选区 |

## 7. 里程碑

- **M1（管线验证）**：F1/F2/F3 + manifest/索引/构建脚本 + PNG 编码与文件名单测。实机烟雾：查询→回车→剪贴板有图/目录有文件。
- **M2（窗口截图）**：窗口枚举+过滤行+PrintWindow 捕获；单测枚举过滤逻辑（mock 不了 EnumWindows，实机冒烟）。
- **M3（区域遮罩）**：分层窗口、拖框、确认/取消、焦点还原；手动 QA 清单（单屏/多屏/125% 缩放/Esc 取消/拖出 0 尺寸）。
- **M4（延时与打磨）**：F6、CF_DIB 兜底、错误文案、多显示器坐标回归。
- **v2 观望**：滚动长截图（需向目标窗口发 WM_MOUSEWHEEL+图像拼接，失败模式多）、截图标注（画框/箭头/马赛克）、OCR 接入（依赖识别库，另行评估）。

## 8. 测试策略

- 可无头单测：查询路由/参数解析（延时秒数、过滤词）、文件名生成、PNG 编码往返、窗口行排序。
- 实机冒烟（隔离 LOCALAPPDATA 预装启用，同 clipboard 插件的做法）：F1 后 PowerShell `Get-Clipboard -Format Image` 非空；F2 后目录出现新文件；F4 截一个已知窗口比对尺寸。
- F5 遮罩无法自动化：随 M3 交付手动 QA 清单，实机走一遍。
