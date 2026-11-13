# 插件市场与宿主服务 v1

本页记录当前实现。设计取舍与后续能力见 [插件市场与宿主接口方案](插件市场与宿主接口方案.md)。

## 市场与资源

设置中的“插件市场”提供发现、已安装、可更新、搜索、详情和本地 ZIP / 目录安装。启用插件、参与普通搜索使用原生开关。运行状态、错误、stderr 与重启入口放在详情的诊断折叠区。

官方安装按下载字节显示进度，随后显示校验和安装阶段。提交替换前可取消；开始替换后禁用取消。成功反馈保留至关闭，提供立即使用和快捷键入口；更新保留原启用状态与配置。包安装成功但设置保存失败时单独提示，不宣称启用成功。

Manifest 新增可选 `icon`、`author`、`homepage`、`usage`；原有 `description` 继续使用。`icon` 为包内 PNG 路径，例如 `assets/icon.png`，推荐 256×256，当前解码上限 512×512、文件上限 1 MiB。越出包目录、指向外部的符号链接和非 PNG 格式不作为图标加载；失败使用通用图标。

签名目录 schema 1 增加可选作者、三语完整介绍、使用说明、版本说明、主页、宿主服务声明与图标资源。图标资源沿用 `{url, sha256, size}`，必须位于同一插件仓库的固定 Release；缓存按哈希命名，每次读取重新验证。插件包与目录资源由同一份图标生成，安装前也能显示。

旧插件没有新元数据时继续可用。新目录的可选展示字段不改变旧宿主的下载校验。新宿主服务的强依赖另有运行期检查，不能靠旧客户端会忽略的字段保护兼容性。

## 接入与协商

帧协议仍为四字节大端长度加 UTF-8 JSON-RPC 2.0，单帧上限 4 MiB。旧 `query` / `execute`、主题、命令和活动状态保持兼容。

新插件在 Manifest 声明需要使用的服务，并通过 SDK 的可选 `HostAware` 接入。SDK 在 initialize 响应声明 `hostServices.v1`；新宿主只向声明该能力的插件发送 `configureHost`。其中包含 API 版本、宿主版本、语言、插件数据目录、缓存目录及可用服务的名称和版本。

```json
{
  "hostServices": ["ui.notify", "settings.get", "settings.set", "context.get"],
  "requiredHostServices": [{"name": "settings.get", "version": 1}],
  "settings": [
    {"id": "prefix", "title": "文本前缀", "type": "text", "default": ""},
    {"id": "preview", "title": "显示预览", "type": "bool", "default": true}
  ]
}
```

未列入 `requiredHostServices` 的服务是可选能力。SDK 在旧宿主下不收到 `SetHost`，插件应保留业务降级路径。存在强依赖时，`ServeFromManifest` 会在业务方法前拒绝未完成协商的请求，提示升级；新宿主也会在安装与启动时检查要求。

服务版本是向后兼容的能力等级，插件通过 `Supports(name, version)` 检查最低要求。改变既有语义时必须使用新的方法名，不能让旧调用静默获得另一种行为。

## Go SDK

```go
type HostAware interface { SetHost(Host) }

type Host interface {
    Supports(service string, version int) bool
    Configuration() HostConfiguration
    Call(ctx context.Context, method string, params any, result any) error
}
```

`Call` 的 `method` 使用下表名称，不带 `host.`；SDK 在网络方法名前自动添加前缀。结果通过传入指针解码，空结果可传 nil。SDK 提供 `Notify`、`ReadClipboardText`、`WriteClipboardText`、`ShowQuery` 常用包装，其他服务直接使用 `Call`。

`SetHost`、查询、动作、配置变更可能来自不同 goroutine；插件必须同步自己保留的状态。宿主不会传递 MyGo Context、Element、Window 或 Windows 句柄。SDK 仍仅依赖 Go 标准库。

## 当前服务

| 名称 | 参数 | 返回值 / 行为 |
| --- | --- | --- |
| `ui.notify` | `kind`：info / success / error，`title`，可选 `message`、`replaceId` | 来源带插件名称；同一插件的 replaceId 替换已有系统通知；每秒限一次，短时间同 ID 重复请求合并 |
| `launcher.show` | `query` | 显示搜索面板、填入查询、聚焦输入框 |
| `launcher.hide` | 无 | 隐藏搜索面板 |
| `clipboard.readText` | 无 | `{text}`；文本上限 1 MiB |
| `clipboard.writeText` | `text` | 写文本剪贴板，上限 1 MiB |
| `dialogs.open` | 可选 `title`、`defaultPath`、`directory`、`multiple`、`filters` | `{paths, cancelled}` |
| `dialogs.save` | 可选 `title`、`defaultPath`、`filters` | `{path, cancelled}` |
| `settings.get` | 无 | `{values: {<id>: <value>}}`，只读当前插件的声明字段 |
| `settings.set` | `key`、`value` | 校验类型和范围、原子保存，只写当前插件 |
| `context.get` | 无 | 与 configureHost 相同的运行环境信息 |
| `shell.open` | `target` | 以默认程序打开绝对路径或 HTTP / HTTPS URL |
| `shell.reveal` | `target` | 在文件管理器定位绝对路径 |

`filters` 为 `[{name, extensions: ["png", "jpg"]}]`，扩展名不含点。文件选择取消返回 `cancelled: true`，不是插件故障。`shell.*` 不接受命令字符串、提权参数或任意 URI 协议。

`launcher.*`、`dialogs.*`、`shell.*` 仅在宿主正在处理显式 execute / executeCommand 时可用；查询链路不能弹窗、抢焦点或打开文件。支持宿主服务的显式动作拥有独立的两分钟上限，普通查询仍使用现有毫秒级超时。异步业务任务仍应立即返回，后续任务 API 尚未开放。

## 独立配置与数据

设置类型：`text`、`bool`、`choice`、`number`。choice 声明字符串 `options`，number 可选 `min` / `max`；所有字段必须有类型正确的 `default`。最多 50 项，不支持密码 / 凭据字段。

宿主详情生成原生文本、开关、选择与数字控件，改动约 400ms 后保存。关闭设置窗口或退出前会刷新尚未落盘的编辑。过期或已取消的写入在原子替换前再次检查，防止旧输入或旧进程会话覆盖新值。

- 数据：`%LOCALAPPDATA%\FluentLauncher\plugin-data\<id>`。
- 配置：数据目录下 `config.json`，只保存当前声明的配置项。
- 缓存：`%LOCALAPPDATA%\FluentLauncher\cache\plugins\<id>`。
- 卸载插件不删除独立数据；重装 / 升级可继续使用。插件自行管理业务数据，宿主不代理整个文件系统。

SDK 可选 `SettingsHandler.SettingsChanged(context.Context) error`，初始化声明 `settings`。配置改变时宿主通知已运行进程；不会为了事件启动插件或延长空闲时间。插件通过 `settings.get` 重新读取，并把 UI 更新排入自己的线程队列。

## 生命周期、错误与边界

宿主与 SDK 每个会话各有一个读循环，按 ID 分派响应和服务请求，最多 32 个待响应调用、8 个服务处理器。服务响应不等待查询 operation gate，避免查询 / 动作调用宿主时死锁。

取消、禁用、卸载和进程退出会结束挂起服务，旧会话不能回写新进程。已经显示的原生文件选择器受当前 MyGo API 限制，可能仍需用户关闭；取消后结果会被丢弃，不会再执行插件动作。

未知方法为标准 `-32601`，参数错误为 `-32602`，宿主业务错误 `-32001` 的 `data.kind` 包括 `unsupported`、`not_declared`、`interaction_required`、`unavailable`、`cancelled`、`rate_limited`、`too_large`。

插件身份来自已验证的进程注册，不接受请求参数中的 pluginId。服务声明约束宿主代理接口并向用户解释数据访问；独立 EXE 仍是受信任代码，不是系统沙箱。

官方 echo 插件演示查询中读取配置、结果动作发送宿主通知；旧宿主仍返回普通回显和复制结果。新 SDK 接口需在发布新的 SDK 标签后供第三方引用，已发布的旧 SDK 标签不包含这些类型。
