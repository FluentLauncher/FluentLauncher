# Windows 单文件发布与更新

目标是 Windows 11 x64。分发文件为 `FluentLauncher.exe`，图标和固定版本的 SQLite 运行库嵌入程序，不需要 .NET、WebView 或完整 Visual Studio。SQLite DLL 在运行时释放到用户缓存；官方插件不嵌入主程序，其独立仓库与按需下载设计见 `official-plugin-repository.md`。

## 配置 GitHub Actions

仓库必须公开，更新客户端通过 GitHub Releases 下载文件，不包含任何 GitHub 令牌。工作流使用 `GITHUB_REPOSITORY` 注入当前仓库地址，因此 fork 可使用自己的发布源。

正式仓库为 [FluentLauncher/FluentLauncher](https://github.com/FluentLauncher/FluentLauncher)，插件与 SDK 位于 [FluentLauncher/FluentLauncher-plugins](https://github.com/FluentLauncher/FluentLauncher-plugins)。原 `mistquay/FluentLauncher` 的 .NET/WinUI 仓库保留，当前组织的版本独立发布。

首次发布前，在本机生成一次更新签名密钥：

```powershell
go tool mygo keygen
```

命令输出密钥的保存位置，默认位于用户配置目录的 `mygo/update-keys` 下。将下列值填入 GitHub 仓库的 **Settings → Secrets and variables → Actions**：

| 类型 | 名称 | 值 |
| --- | --- | --- |
| Repository variable | `MYGO_UPDATER_PUBLIC_KEY` | `mygo-update.pub` 的完整内容 |
| Repository secret | `MYGO_UPDATER_PRIVATE_KEY` | `mygo-update.key` 的完整内容 |
| Repository variable | `FLUENTLAUNCHER_PLUGIN_PUBLIC_KEY` | 独立插件仓库目录签名的公钥，必须与插件仓库一致 |
| Repository variable | `FLUENTLAUNCHER_PLUGIN_REPOSITORY` | 独立插件仓库 owner/name，默认 `FluentLauncher/FluentLauncher-plugins` |

密钥仅生成一次。私钥要另行备份，不要提交到 Git；以后发版沿用同一对密钥，已安装版本才能验证后续更新。更新包的 Ed25519 签名与 Windows Authenticode 代码签名是两回事，目前工作流仅配置前者。

插件目录使用独立密钥，其私钥只存在插件仓库发布端，主程序只需要插件公钥。配置步骤见 `official-plugin-repository.md`；正式签名构建缺少插件公钥时会停止，避免发布无法访问官方插件的构建。

工作流需要写入 Releases 的权限（文件中已声明 `contents: write`）；组织策略如果禁止，需要在仓库或组织设置放行。

## 自动发布 nightly

向 `main` 推送代码后，`.github/workflows/release.yml` 自动运行构建、测试、静态检查和三语检查，通过后生成签名单文件包并公开 GitHub Pre-release。无需修改 `mygo.json`，也无需手工打标签。Nightly 不标记为 Latest，README 的稳定版下载链接和旧客户端的稳定更新源继续指向正式版。

版本以最近已公开的正式版本为基准，修订号加一，再附加 UTC 日期、Actions run number 和 run attempt。例如正式版为 `0.1.0` 时：

```text
v0.1.1-nightly.20261110.123.1
v0.1.1-nightly.20261110.124.1
v0.1.1
v0.1.2-nightly.20261111.126.1
```

每次 nightly 使用独立标签、更新清单和归档，不覆盖旧资产。重跑 nightly 使用新的 attempt 后缀；只有当前 `main` 提交会发布，已经被新推送取代的排队构建会跳过。Nightly 暂不自动清理。

## 一键发布正式版

在 GitHub **Actions → Windows release → Run workflow** 中选择 `main`，再选择 `action`：

| action | 行为 |
| --- | --- |
| `build`（默认） | 只构建并上传 Artifact，不发布；可勾选 `signed` 验证签名构建 |
| `nightly` | 手动发布当前 main 的 nightly |
| `patch` | 自动增加修订号，例如 `0.1.0 → 0.1.1`，发布正式版 |
| `minor` | 自动增加次版本并清零修订号，例如 `0.1.1 → 0.2.0` |
| `major` | 自动增加主版本并清零其他位，例如 `0.2.0 → 1.0.0` |

自动发版仅允许从 `main` 运行，所有公开发布均强制签名，不受 `signed` 复选框影响。工作流从 GitHub 已公开、非预发布的 `vX.Y.Z` 版本中取最大值，nightly 与 SDK 标签不参与递增；没有正式发布记录时，以 `mygo.json` 为初始基准。GitHub 查询失败时停止，不猜测版本。

检查和构建全部通过后，工作流为实际构建的提交自动创建标签与草稿 Release，上传完整附件后公开，并将正式版设为 Latest。`GITHUB_TOKEN` 创建的标签不会递归触发另一轮标签工作流。

版本与更新源只在构建期间注入，结束后恢复 `mygo.json`，不产生单独的版本号提交。更新说明优先使用 `CHANGELOG.md` 中对应的 `## 版本号` 章节；没有该章节时，自动取自上一个正式版以来最近 50 条非合并提交，并附上构建提交链接。Release 页面和客户端更新卡片使用同一份说明。

仍可手工推送稳定版本标签，版本必须高于最近正式版。例如（确认远程仓库和分支正确后执行）：

```powershell
git tag v0.2.0
git push origin HEAD
git push origin v0.2.0
```

Release 包含以下文件：

| 文件 | 用途 |
| --- | --- |
| `FluentLauncher.exe` | 用户下载后直接运行，文件名保持不变 |
| `fluent-launcher-0.2.0-windows-amd64.tar.gz` | 签名校验覆盖的更新包，只有一个 EXE |
| `update-windows-amd64.json` | 版本、更新说明、下载地址、大小与签名 |
| `SHA256SUMS` | 手工核对下载文件的 SHA-256 |

所有发布任务使用同一个并发组，避免同时分配同一正式版本。上传失败时保留草稿，可以重跑同一提交；草稿标签指向其他提交时拒绝继续。已公开的 Release 禁止覆盖。若要改用新提交重试同一正式版本，应先人工处理失败草稿及对应标签，或改用更高的手工版本标签。

## 客户端行为

- “设置 → 其他 → 软件更新”提供稳定版和预览版 / Nightly 通道。稳定版只检查正式发布；预览版接收 nightly 以及版本更高的正式版。
- 更新区域按版本、更新内容、更新通道分行显示，检查与安装按钮位于右侧。更新内容通过独立弹层阅读，支持 Markdown 标题、列表和链接；短说明按内容收缩，长说明可滚动，操作按钮固定在底部。点击“稍后”、按 Esc 或点击遮罩均可关闭。
- 通道使用可通过方向键切换的分段控件。下载时显示进度，并禁用重复检查、安装与通道切换；更新说明仍可阅读或关闭。
- 没有保存通道选择时，第一次启动根据构建选择默认值并保存：正式构建为稳定版，nightly 为预览版。之后保留用户选择，预览用户升级到正式版后仍继续接收预览更新。旧配置缺少该字段时同样处理。
- 切换通道后清除先前发现的更新；检查或安装期间禁用通道选择，切换尚未保存时禁用检查。切回稳定版不会降级，等待更高正式版，例如 `0.1.1-nightly.… → 0.1.1`。
- 已安装的旧正式版可继续通过原更新源升级；需要先升级到带通道设置的新正式版，才能在设置中选择预览更新。
- 正式签名发布的构建启动 10 秒后后台检查，之后运行期间每 24 小时检查一次；发现新版本时仅更新状态，不弹出设置窗口、不切换设置页或抢占焦点。用户主动打开“设置 → 其他”后可查看新版本并更新。
- 托盘的“检查更新”和设置页的同名按钮均可手动检查，失败时显示原因并支持重试。
- 用户点击“更新并重启”后下载更新包，显示进度，校验大小和 Ed25519 签名，成功后原地替换 EXE 并按正常退出流程重启。
- 替换前的下载或校验失败不会替换程序；MyGo 替换过程中失败会尝试回滚。运行中的旧 EXE 会临时改名，下次启动清理。
- 配置、数据库、主题与已安装插件留在 `%LOCALAPPDATA%\FluentLauncher`，更新包不包含这些目录。主程序更新不更新插件；官方插件由独立仓库提供签名目录，用户选择安装或更新时才下载对应 ZIP。
- 保持 `FluentLauncher.exe` 文件名；程序所在目录及其父目录应可写（更新会在相邻目录暂存）。没有权限时不提权，界面说明当前构建无法自行更新。
- 普通 `go build` 与未签名的本地构建不配置更新源，设置页会说明不支持在线更新。

## 本地构建

```powershell
./scripts/build-release.ps1
```

输出：`build/release/windows-amd64/FluentLauncher.exe`。只有这个 EXE 是运行所必需的文件。

验证签名发布链路时，先从本机密钥文件读取变量，再执行：

```powershell
$env:MYGO_UPDATER_PUBLIC_KEY = Get-Content -Raw '<公钥文件路径>'
$env:MYGO_UPDATER_PRIVATE_KEY = Get-Content -Raw '<私钥文件路径>'
$env:FLUENTLAUNCHER_PLUGIN_PUBLIC_KEY = Get-Content -Raw '<独立插件公钥路径>'
$env:FLUENTLAUNCHER_PLUGIN_REPOSITORY = 'owner/FluentLauncher-plugins'
./scripts/build-release.ps1 -Signed -Tag v0.2.0 -Repository owner/FluentLauncher
Remove-Item Env:MYGO_UPDATER_PRIVATE_KEY
```

脚本临时注入版本、仓库地址和公钥，结束后恢复 `mygo.json`，私钥仅通过环境变量传给构建工具。初始实现禁用增量包，始终下载完整签名归档，减少发布链路复杂度。

本地验证 nightly 时，`-Tag` 也接受 `v0.1.1-nightly.20261110.123.1`。若没有对应更新记录，给 `-NotesFile` 传入包含 `## 0.1.1-nightly.20261110.123.1` 章节的临时 Markdown 文件；自动发布工作流会生成该文件。使用独立测试密钥，不要覆盖正式密钥。

版本规划与发布保护可离线检查：

```powershell
./scripts/test-release.ps1
```

该检查使用模拟的 GitHub/git 命令，覆盖版本排序、自动递增、nightly 编号、更新说明回退、公开版本不可覆盖、草稿提交一致性、上传失败保护和 stable/nightly 发布标记，不访问网络或修改远程仓库。

## Nightly 与更新通道本地验收（2026-11-10）

- `CGO_ENABLED=0` 下 `go build ./...`、`go vet ./...`、`go test ./...`、`go tool mygo vet .`、`go tool mygo i18n check .` 通过；发布工作流通过固定版本 actionlint 静态检查。
- 离线 PowerShell 测试覆盖正式版本数值排序与递增、nightly 重跑编号、main 限制、过时 main 构建跳过、查询失败停止、自动创建标签、草稿提交校验与公开版本不可覆盖。
- `ui.NewTester` 验证三语通道选择、方向键切换、切换后清除旧更新、待保存时禁止检查、下载时禁止切换、日志弹层打开关闭、70 条日志的滚动区域与底部动作可见性；导出了三语、深浅主题、820/1180 宽度的设置行与日志弹层预览并检查布局。
- 使用独立测试密钥构建 `0.1.1-nightly.20261110.123.1`；更新清单的版本、固定标签下载地址和说明正确，单文件归档与 `mygo.json` 恢复检查通过。
- 可选测试 `TestSignedNightlyUpdateIntegration` 在本地 HTTPS 服务中模拟 GitHub Releases，调用实际 MyGo 检查与安装 API：稳定版排除 nightly、连续预览版更新、nightly 升正式版、切回稳定版不降级均通过；篡改包被拒绝且原 EXE 不变，有效签名包成功替换隔离 Windows 测试进程，旁边的测试文件保留。
- 可用 `FL_TEST_SIGNED_RELEASE_DIR` 指定上述签名 nightly 目录、`FL_TEST_UPDATE_PUBLIC_KEY` 指定测试公钥，运行 `go test ./internal/app -run '^TestSignedNightlyUpdateIntegration$' -count=1 -v` 重验。测试不启动用户日常使用的启动器，不访问个人配置或数据库。
- 本次新增工作流尚未推送，真实 GitHub Actions 的自动 nightly 和一键正式发版仍待推送后验收；下方首次正式发布记录对应此前流程。

## 本次本地验证（2026-11-10）

- `CGO_ENABLED=0` 下 `go build ./...`、`go vet ./...`、`go test ./...`、`go tool mygo vet .`、`go tool mygo i18n check .` 通过。
- `ui.NewTester` 验证简体中文、繁体中文和英文的更新卡片，包括空闲、发现更新、下载中禁用重复操作、移除旧更新状态。
- 使用临时测试密钥完成 `v0.2.0` 签名打包；更新归档中只有 `FluentLauncher.exe`，主程序不嵌入官方插件。
- 隔离测试进程调用实际 MyGo 更新 API：篡改一个归档字节后签名校验拒绝安装，原 EXE 哈希不变；有效归档可以替换正在运行的 Windows 测试 EXE，结果哈希与发布 EXE 一致，邻近的测试文件保留。
- 同一隔离进程从内嵌数据释放 SQLite DLL，并成功执行内存数据库查询，不依赖 EXE 旁的 DLL。
- 本地文件源验证了下载、校验和替换；真实 GitHub 更新源与 Actions 验收见下节。测试密钥和辅助程序均在 Git 忽略的 `build` 下，不是正式发布密钥。
- 正式签名密钥独立于测试密钥，位于本机用户配置目录的 `mygo/update-keys/FluentLauncher/launcher` 和 `plugins`，对应私钥只配置到各自仓库的 Actions Secret。请额外保留安全备份，切勿用测试密钥覆盖正式密钥。

## 首次正式发布验收（2026-11-10）

- [启动器 v0.1.0](https://github.com/FluentLauncher/FluentLauncher/releases/tag/v0.1.0) 已由 [Windows Actions](https://github.com/FluentLauncher/FluentLauncher/actions/runs/42751436462) 成功构建并公开发布。资产包含单文件 EXE、签名更新归档、更新清单与 SHA256SUMS。
- [官方插件 v1.0.0](https://github.com/FluentLauncher/FluentLauncher-plugins/releases/tag/v1.0.0) 已由 [插件 Actions](https://github.com/FluentLauncher/FluentLauncher-plugins/actions/runs/42750741639) 发布，含三个独立 ZIP、catalog.json 与 catalog.sig；[SDK 标签检查](https://github.com/FluentLauncher/FluentLauncher-plugins/actions/runs/42750741668) 通过。
- 第三方模板在没有本地 replace 的情况下，从公开 `sdk/v0.1.0` 标签下载并构建成功。首次使用时国内代理尚未同步，切换测试进程到 Go 官方代理后完成完整校验，未关闭校验服务或修改全局配置。
- 使用真实网络客户端从 GitHub 下载插件签名目录及三个 ZIP，完成验签、临时安装和回显/开发工具实际查询。
- 隔离测试进程使用正式主程序公钥和真实 GitHub 更新源，从低版本检查到 v0.1.0，下载并验证签名后原地替换正在运行的测试 EXE；邻近测试文件保留。替换后 EXE 的 SHA-256 为 `2538f5c422346e61459dc8df94ff2a613dd7c6ddf202faf4d0aee847c77b18fd`，与 Release 的 SHA256SUMS 一致；正式插件公钥注入也已验证。
- 测试未运行用户日常安装的启动器、未修改其个人数据库或开机启动项。上述结果验证发布资产、客户端网络和更新替换机制；完整用户环境中的界面重启仍需实机使用验收。
- 旧 `mistquay/FluentLauncher` 仍为原私有 .NET/WinUI 仓库，没有迁移、覆盖或改变可见性。
