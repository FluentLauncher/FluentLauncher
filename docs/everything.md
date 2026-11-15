# Everything 连接与故障处理

Everything 连接器通过客户端的 Unicode `WM_COPYDATA` IPC 查询索引，不需要 SDK DLL。只运行 Everything 服务不能提供查询；目前支持默认实例，不支持命名实例。

## 使用与重试

在「设置 → 文件」启用连接器。程序位置留空时从安装信息、应用发现结果和已知位置检测；便携版可以手动选择 `Everything.exe`。

「后台启动并连接」会在需要时以 `-startup` 启动客户端，并发送一次最多返回一条结果的查询。收到合法回复后才显示「已连接」，仅发现进程窗口不会判定成功。

查询失败后暂停自动重试 15 秒，保留错误状态。本地应用和本地文件索引继续搜索。暂停到期后，下次搜索重新尝试；手动连接或修改程序位置可以立即重试。正常输入引起的查询取消不会记录为连接故障。

### 自动启动与 UAC

Everything 设置了「以管理员身份运行」（`Everything.ini` 中 `run_as_admin=1`）时，客户端会自行提权重启并弹出 UAC，启动器无法绕过。因此：

- 自动启动前读取 `Everything.ini`：便携版在程序目录；本地文件不存在或 `app_data=1` 时读 `%APPDATA%\Everything\Everything.ini`。检测到 `run_as_admin=1` 时跳过自动启动并在设置页说明原因，只有用户点击「后台启动并连接」才会启动（并弹一次 UAC）。
- 自动启动失败或超时后，本次运行不再随输入自动重试启动；手动连接、修改程序位置或一次成功查询后恢复。客户端已在运行时不受影响，仍正常连接。
- 推荐配置：在 Everything「选项 → 常规」中取消「以管理员身份运行」，勾选「Everything 服务」，并让 Everything 开机启动。这样任何方式启动都不再需要授权。

## 结果排序与过滤

Everything 的 IPC 按索引顺序返回结果，不按相关度排序，所以启动器会多取一些候选，再在本地重新排序：

- 单个普通词先发 `startwith:<词>` 取最多 300 条名称前缀匹配，再发原查询取最多 200 条；按路径去重后按名称匹配程度排序：名称或去掉扩展名后完全相同 > 前缀 > 词首包含 > 包含。可执行文件和快捷方式加分，文件夹次之，路径越深分数略低。已经使用 Everything 语法的查询（含 `:`、`\`、`*`、空格等）只发原查询。
- Everything 结果的分数固定在 100–800，低于所有本地匹配（最弱的应用副标题匹配为 1000）。应用、系统功能、本地文件索引始终排在前面，Everything 只填补剩余位置。
- 每次查询都附加「排除路径」（设置 → 文件 → Everything 连接器），生成 `!"路径"` 条件。默认排除 `%SystemRoot%` 下的 `WinSxS`、`SoftwareDistribution`、`Installer`、`servicing`、`assembly`、`Prefetch`，以及 `\$Recycle.Bin\`、`\System Volume Information\`。条目含 `\` 时按完整路径匹配，环境变量在查询时展开。

本地应用同时从 AppUserModelID 提取英文别名，例如 `Microsoft.Windows.Explorer` 得到 `Explorer`，`{…}\cmd.exe` 得到 `cmd`。这样在中文系统中输入 `explor` 也能命中「文件资源管理器」。

如果持续无法连接，先打开 Everything 确认其索引已可正常搜索，并检查是否使用默认实例。拒绝查询时显示 Windows 错误信息，便于检查权限等问题。超时会明确提示已暂停查询及手动重连入口。

## 搜索响应

- 内存来源立即查询。Everything 保留 100ms 防抖，避免每次击键都发送 IPC。
- 新查询约一帧（16ms）后即可发布空结果、清除上一条查询的结果，不必等外部来源全部完成；已有本地命中仍立即发布。Everything 的结果随后增量合并。
- 等待查询执行权的旧请求可以立即响应取消，不在互斥锁后积累。原生消息发送最多等待 100ms，并受调用方剩余时限约束；等待 IPC 回复的整体查询时限为 1.5 秒。
- 取消唤醒线程结束后才销毁查询窗口，避免向已经回收的窗口句柄发送迟到唤醒。

上述时间是调度窗口和等待上限，实际界面呈现还取决于线程调度与绘制耗时。

## 2026-11-13 排查与验证

本地诊断日志记录了两次 Everything 查询在约 1,601ms 后超时，并且同一查询的首次空结果也在约 1,601ms 后发布。代码中确认：查询失败没有退避，后续查询又会因客户端窗口存在而清除失败状态；结果收集器在所有来源为空时等待慢来源完成；旧查询等待串行互斥锁时不能响应取消。本次分别修复这些问题，并把连接按钮改为真实 IPC 探测。

验证覆盖：

- `go build ./...`、`go vet ./...`、`CGO_ENABLED=0 go test ./...`、`go tool mygo vet .` 和 `go tool mygo i18n check .`。
- 查询失败退避、手动探测失败与恢复、到期和修改路径后重试、排队取消、取消不触发故障，以及慢来源未完成时发布空结果；相关 race 检查通过。
- 本机 Everything 1.4.1.1028 x64：真实连接探测、英文/中文/无命中查询、20 次原生查询取消后的继续搜索通过。该轮直接 IPC 查询约 24–53ms，不含 100ms 防抖，不代表统计分位数。
- 临时的独立 MyGo 程序初始化真实事件循环，不打开可见窗口；30 次并发查询取消后，连续查询和无命中返回正常。临时探测源码已移除。

原截图中的单次超时没有稳定复现，因此不能确定当时客户端未回复的具体原因。没有关闭现有 Everything 客户端验证冷启动，也没有替换正在使用的 FluentLauncher；用户原窗口下的实际输入体验仍需使用更新后的程序确认。

## 2026-11-15 排序、过滤与 UAC

用户反馈两个问题：自动启动时弹出 UAC；搜 `explor` 时被 WinSxS 组件目录淹没，搜 `winscp` 时也看不到 `WinSCP.exe`。排查确认了三个原因：本机便携版 `run_as_admin=1`；Everything 结果固定 5000 分，高于只命中副标题的应用（1000 分）；只取 10 条按索引顺序返回的结果，相关项被截断。修复内容见上面两节。

验证：

- `go build ./...`、`go vet ./...`、`CGO_ENABLED=0 go test ./...`、`go tool mygo vet .`、`go tool mygo i18n check .` 通过。
- 单元测试覆盖：排序和去重、分数上限低于本地匹配、排除条件和前缀查询的拼接、`Everything.ini` 位置判断、需要提权时不自动启动、失败后不重复启动、AppUserModelID 别名，以及设置页排除路径编辑框在三种语言下的布局。
- 本机 Everything 1.4.1.1028 x64 直接调用 `Provider.Search`：`winscp` 前几条是 `WinSCP.com`、`WinSCP.exe`、桌面快捷方式和安装目录；`explor` 首条是 `C:\Windows\explorer.exe`，不再出现 WinSxS。含 100ms 防抖在内，单次耗时约 190–245ms。
- 未在完整界面中实机验收；未关闭现有客户端验证跳过自动启动后的设置页提示。

真实客户端回归可按需运行，会连接已有客户端，并可能按设置启动客户端：

```powershell
$env:FL_TEST_EVERYTHING_LIVE = '1'
$env:FL_TEST_EVERYTHING_PATH = 'C:\Program Files\Everything\Everything.exe'
go test ./internal/winapi/everything -run '^TestEverythingLiveIntegration$' -v -count=1
Remove-Item Env:FL_TEST_EVERYTHING_LIVE,Env:FL_TEST_EVERYTHING_PATH
```
