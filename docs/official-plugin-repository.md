# 官方插件独立仓库与按需下载

官方插件不嵌入启动器，点击安装时下载，主程序与插件独立发版。源码与发布位于 [FluentLauncher/FluentLauncher-plugins](https://github.com/FluentLauncher/FluentLauncher-plugins)，独立本地仓库位于 `D:\project\go_project\FluentLauncher-plugins`。

## 仓库边界

公开仓库名为 `FluentLauncher-plugins`，默认发布源为 `FluentLauncher/FluentLauncher-plugins`，可在启动器构建变量中指定实际 owner/name。启动器仓库保留宿主、安装器、协议校验与已安装插件管理；独立仓库负责官方插件源码、SDK、示例、打包与插件目录。

```text
FluentLauncher-plugins/
  sdk/                      # 独立 Go module；只含协议与标准库
  plugins/
    echo/                   # 最小示例
    devtools/
    clipboard/
  templates/go-plugin/      # 第三方开发模板
  docs/protocol.md          # JSON-RPC、权限说明、调试步骤
  catalog.source.json       # 三语名称和介绍
  tools/package/            # 生成 ZIP、catalog.json 和 catalog.sig
  scripts/build.ps1         # 本地和 CI 打包入口
  .github/workflows/        # 编译、打包、校验并发布 Releases
```

SDK 模块路径是 `github.com/FluentLauncher/FluentLauncher-plugins/sdk`，不依赖 MyGo 和启动器宿主，只依赖标准库。插件集合的根模块通过本地 replace 开发 SDK；第三方模板引用发布标签 `sdk/v0.1.0`。启动器测试使用专用测试进程，不需要这个独立仓库、SDK 或网络。

## 安装链路

1. 本次运行首次打开设置时后台加载已验签缓存并刷新官方目录，也可点击“刷新官方目录”。先验证目录原始字节的 Ed25519 签名，再展示名称、说明、版本等元数据，不下载可执行文件。
2. 用户选择安装或更新某个插件，才下载该插件固定版本的 Windows x64 ZIP。
3. 校验目录规定的 SHA-256、下载大小、ZIP 路径、插件 ID、协议版本与可执行文件；安装器复用现有 staging 与替换逻辑。
4. 安装到 `%LOCALAPPDATA%\FluentLauncher\plugins\<id>`，成功后自动启用并作为独立进程运行。下载或校验失败保留已有版本，并允许重试。
5. 同一安装入口对已有插件的新版本显示“更新插件”，不随主程序替换。下载期间继续使用旧版本，替换期间阻止该插件的新查询/动作，等待进程退出后再替换文件；安装失败后恢复已有版本的运行能力。

启动器内置三语名称只作目录未配置/不可用时的展示，没有版本及下载地址，不能触发安装。完整版本和下载元数据由签名目录提供。缓存位于 `%LOCALAPPDATA%\FluentLauncher\cache\official-catalog.json`，读取时重新验签；刷新失败保留旧缓存。

## 签名密钥与配置

签名用于验证发布者和内容完整性，不是加密，不是 Windows Authenticode 证书，也不能把本地插件变成安全沙箱。插件目录使用与主程序更新不同的密钥；目录含每个 ZIP 的大小与 SHA-256，因此签名同时保护下载元数据。

| 仓库 | 类型 | 名称 |
| --- | --- | --- |
| 插件仓库 | Repository secret | `FLUENTLAUNCHER_PLUGIN_PRIVATE_KEY` |
| 插件仓库 | Repository variable | `FLUENTLAUNCHER_PLUGIN_PUBLIC_KEY` |
| 启动器仓库 | Repository variable | `FLUENTLAUNCHER_PLUGIN_PUBLIC_KEY`（同一公钥） |
| 启动器仓库 | Repository variable | `FLUENTLAUNCHER_PLUGIN_REPOSITORY`（实际 owner/name） |

在启动器仓库运行 `go tool mygo keygen -o <独立插件密钥目录>` 可生成兼容的 Base64 Ed25519 密钥。私钥只交给插件仓库的 Actions，公钥注入启动器；不要把私钥放进 EXE 或 Git，也不要覆盖正式密钥。主程序正式签名构建会要求提供插件目录公钥；本地未配置时显示原因，并保留 ZIP/目录安装能力。

## 发布与开发体验

每个插件自己的 `plugin.json` 决定版本；工作流构建为 `official.echo-1.0.0-windows-amd64.zip` 等资产，压缩包根目录含 `plugin.json` 与程序。每次集合发布携带完整目录与所有当前插件包，目录使用该 Release 的固定版本下载地址和哈希，避免 `latest` 指向变化导致版本与校验值不同步。

独立仓库 README 给出“复制模板 → 修改 manifest → 本地运行/调试 → 打 ZIP → 启动器安装”的完整步骤。SDK 单独以 `sdk/v0.1.0` 等标签发布，插件集合用 `v1.0.0` 等标签。手动 Actions 生成未签名本地 Artifact，版本标签生成签名目录并在全部文件上传后公开 Release。

首次上线顺序：在 FluentLauncher 组织创建两个公开仓库 → 配置独立密钥 → 发布 SDK 标签和插件 Release → 在启动器仓库配置同一插件公钥与仓库地址 → 发布主程序。主程序使用 `v0.1.0` 起始版本，SDK 使用 `sdk/v0.1.0` 标签，插件集合使用 `v1.0.0` 起始版本。

## 本地验证

- 启动器的 `CGO_ENABLED=0 go build ./...`、`go vet ./...`、`go test ./...`、`go tool mygo vet .` 和三语检查通过；界面测试覆盖安装提示、插件版本、升级按钮和重复操作禁用。
- 独立仓库根模块与 SDK 模块分别通过构建、静态检查与测试。SDK 的 `sdk/v0.1.0` 标签已由组织仓库 Actions 验证，第三方模板使用对应模块路径。
- 独立仓库使用临时测试密钥打出三个真实 ZIP、`catalog.json` 与 `catalog.sig`。启动器用本地 HTTP transport 验签、按需下载并安装三个包，通过独立 SDK 对回显和开发工具执行实际查询；不依赖 GitHub 网络。
- 测试覆盖目录/ZIP 篡改、身份与版本不符、不固定的下载 URL、缓存复验、离线刷新及取消。失败不会覆盖已验证的缓存或因下载校验失败删除已装插件。
- `go test -race` 验证目录刷新与替换串行、替换期间禁用新查询、失败恢复，以及排队查询切换到当前路径。
- 主程序签名打包已验证插件目录公钥注入。测试密钥与正式发布密钥分开；正式私钥保存在用户配置目录和对应仓库 Actions Secret，未写入 Git。
- `v1.0.0` 已由组织仓库 Actions 发布。客户端从真实 GitHub 下载并验签目录、安装三个 ZIP，通过独立 SDK 对回显和开发工具执行查询；主程序发布验证记录见 `releasing.md`。
