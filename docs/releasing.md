# Windows 单文件发布与更新

目标是 Windows 11 x64。分发文件为 `FluentLauncher.exe`，图标和固定版本的 SQLite 运行库嵌入程序，不需要 .NET、WebView 或完整 Visual Studio。SQLite DLL 在运行时释放到用户缓存；官方插件不嵌入主程序，其独立仓库与按需下载设计见 `official-plugin-repository.md`。

## 配置 GitHub Actions

仓库必须公开，更新客户端通过 GitHub Releases 下载文件，不包含任何 GitHub 令牌。工作流使用 `GITHUB_REPOSITORY` 注入当前仓库地址，因此 fork 可使用自己的发布源。

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
| Repository variable | `FLUENTLAUNCHER_PLUGIN_REPOSITORY` | 独立插件仓库 owner/name，默认 `mistquay/FluentLauncher-plugins` |

密钥仅生成一次。私钥要另行备份，不要提交到 Git；以后发版沿用同一对密钥，已安装版本才能验证后续更新。更新包的 Ed25519 签名与 Windows Authenticode 代码签名是两回事，目前工作流仅配置前者。

插件目录使用独立密钥，其私钥只存在插件仓库发布端，主程序只需要插件公钥。配置步骤见 `official-plugin-repository.md`；正式签名构建缺少插件公钥时会停止，避免发布无法访问官方插件的构建。

工作流需要写入 Releases 的权限（文件中已声明 `contents: write`）；组织策略如果禁止，需要在仓库或组织设置放行。

## 发布

1. 按需更新 `mygo.json` 的 `version`，方便本地开发与发版版本一致。可在 `CHANGELOG.md` 的 `## 0.2.0` 下写更新说明；文件存在时必须包含待发布版本的章节。
2. 提交代码，推送版本标签，例如 `v0.2.0`。标签是发布版本的最终来源，只接受 `v主版本.次版本.修订号` 稳定版本。
3. `.github/workflows/release.yml` 在 Windows runner 上运行构建、测试、静态检查和三语检查，再构建 EXE、签名更新包并校验归档只含主 EXE。
4. 所有检查通过后，先上传文件到草稿 Release，再公开发布。发布失败时可重跑草稿；已公开的版本禁止覆盖，修复请用新版本标签。

示例（确认远程仓库和分支正确后执行）：

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

Actions 的 **Run workflow** 可只构建、下载 Artifact，不创建 Release。默认无需密钥且不启用在线更新；勾选 signed 后生成具备更新能力的构建，但仍不公开发布。

## 客户端行为

- 正式签名发布的构建启动 10 秒后后台检查，之后运行期间每 24 小时检查一次；发现新版本打开“设置 → 其他”的软件更新卡片。
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

## 本次本地验证（2026-11-10）

- `CGO_ENABLED=0` 下 `go build ./...`、`go vet ./...`、`go test ./...`、`go tool mygo vet .`、`go tool mygo i18n check .` 通过。
- `ui.NewTester` 验证简体中文、繁体中文和英文的更新卡片，包括空闲、发现更新、下载中禁用重复操作、移除旧更新状态。
- 使用临时测试密钥完成 `v0.2.0` 签名打包；更新归档中只有 `FluentLauncher.exe`，主程序不嵌入官方插件。
- 隔离测试进程调用实际 MyGo 更新 API：篡改一个归档字节后签名校验拒绝安装，原 EXE 哈希不变；有效归档可以替换正在运行的 Windows 测试 EXE，结果哈希与发布 EXE 一致，邻近的测试文件保留。
- 同一隔离进程从内嵌数据释放 SQLite DLL，并成功执行内存数据库查询，不依赖 EXE 旁的 DLL。
- 使用本地文件源验证下载、校验和替换；未验证 GitHub Actions 线上运行、真实 GitHub 更新源及完整 UI 退出/重启。测试密钥和辅助程序均在 Git 忽略的 `build` 下，不是正式发布密钥。
- 当前工作区未配置远程地址，GitHub CLI 查询返回认证失败。正式发版仍需配置远程、恢复登录并设置仓库签名变量/Secret。
