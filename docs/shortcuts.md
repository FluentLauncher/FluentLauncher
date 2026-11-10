# 统一快捷键

实施清单：

- [x] 内置命令、按键规范化、作用域及旧配置迁移。
- [x] 快捷键设置页、录制、多绑定、冲突处理及动态提示。
- [x] 全局热键差异注册、失败回退与录制保护。
- [x] 插件静态命令、能力协商、后台按需调用及生命周期。
- [x] 独立 SDK、模板和官方示例同步。
- [x] 最终回归、静态检查与验证记录。

快捷键由宿主统一管理。全局作用域与各局部作用域相互冲突；搜索、动作面板、输入弹窗、布局预览的局部作用域互斥。标准文本编辑、Tab 导航和系统保留按键不提供改键。

用户覆盖写入 settings.json 的 Shortcuts。未覆盖时内置命令采用默认值，插件命令带推荐值但默认禁用。Keys 为空表示清除，Enabled=false 保留按键但不启用。

## 使用

设置侧栏「常规」后的「快捷键」页包含全局、搜索面板、动作面板、输入弹窗、布局预览及按插件分组的命令。支持按功能、插件名/ID、按键筛选。原「常规」页不再重复显示呼出热键。

列表采用 40 DIP 的紧凑单行布局：左侧为功能名称，右侧依次为可编辑的按键字段、启用开关和更多菜单。正常启用状态由开关表达；仅在冲突、不可用、未注册或实际按键不同时显示补充说明。插件说明及完整按键可悬停查看，同作用域命令集中分组。

点击按键字段（铅笔图标），或从更多菜单选择「编辑」进入录制，点击录制区域按组合键，再确认保存；一次可添加多个组合键（最多 12 个）。Esc、Enter 本身可录制，Tab 用于移到按钮。已注册的本应用全局键在录制期间只更新录制值，不执行命令。关闭设置、切换页面或窗口失去焦点时取消未确认录制。

开关停用时保留按键，更多菜单中的「清除绑定」解除按键，「恢复默认」移除用户覆盖。冲突时显示占用功能，只有点击「替换冲突绑定」才从其他功能移除冲突的那个按键；其他组合键保留。所有改动沿用设置的自动保存。

## 内置命令与配置

| 作用域 | 命令 ID | 默认键 |
| --- | --- | --- |
| global | launcher.toggle | Alt+Space |
| search | search.previous / next | Up / Down |
| search | search.execute / dismiss | Enter / Escape |
| search | search.actions | Ctrl+K、Shift+F10、Menu |
| actions | actions.previous / next | Up / Down |
| actions | actions.execute / dismiss | Enter / Escape、Ctrl+K |
| actions | actions.slot.1 ～ actions.slot.9 | Alt+1 ～ Alt+9 |
| prompt | prompt.confirm / cancel | Enter / Escape |
| preview | preview.left / right / up / down | Left / Right / Up / Down |

组合键使用 Ctrl、Alt、Shift、Win，支持字母、数字、F1–F12、导航键和常用标点键名。全局键要求 Ctrl、Alt 或 Win；系统保留键拒绝配置，其他程序占用以实际注册结果为准。窗口提示与动作右侧按键从当前配置生成。

```json
{
  "Shortcuts": {
    "launcher.toggle": {"Keys": ["Ctrl+Alt+Space"], "Enabled": true},
    "search.actions": {"Keys": ["Ctrl+J", "Menu"], "Enabled": true},
    "plugin:official.clipboard:show-history": {"Keys": ["Ctrl+Alt+V"], "Enabled": true}
  }
}
```

旧 settings.json 没有 Shortcuts 时，Hotkey 自动迁入 launcher.toggle；legacy hotkey.json 也迁移。新的 Shortcuts 一旦存在便优先，清除和恢复默认后不会再次从旧 Hotkey 恢复。

全局键逐项差异注册。用户修改时注册失败则尽力恢复旧注册并回退草稿；失败不会导致每次自动保存重试。配置写盘失败也恢复原注册。启动时采用尽力注册，只有启动器呼出功能允许尝试 Alt+Shift+Space / Ctrl+Alt+Space 备用键，并显示实际生效键。已有备用呼出键不会因为修改局部快捷键而反复注销。

## 插件命令

插件 manifest 可选声明 commands，命令 ID 必须在插件升级中保持稳定。配置使用 plugin:<插件ID>:<命令ID>，禁用、暂停、卸载或替换期间不分发；用户覆盖保留，重新启用和重装后重新检查冲突。安装或升级遇到已有绑定冲突时，后加入的命令不会抢占原命令。

```json
{
  "commands": [
    {
      "id": "show-history",
      "title": "打开剪贴板历史",
      "scope": "global",
      "defaultShortcuts": ["Ctrl+Alt+V"],
      "activation": "showQuery",
      "query": "clip "
    },
    {
      "id": "capture-region",
      "title": "区域截图",
      "scope": "global",
      "activation": "invoke"
    }
  ]
}
```

scope 支持 global 和 search。showQuery 由宿主显示并填入查询，不需要新 SDK 接口；invoke 使用新的 executeCommand RPC，参数为 {"commandId":"capture-region"}。插件只有在初始化 capabilities 声明 executeCommand 后才会收到调用。原 execute(resultId, actionId) 与协议版本 1 保留，旧插件继续可用。

SDK 提供可选 CommandHandler.ExecuteCommand(context.Context, ExecuteCommandRequest)，不修改原 Handler。宿主在后台按需启动插件，复用进程预算、暂停、超时、空闲回收和更新替换机制；同一命令执行期间忽略重复触发。长时间交互应在插件启动任务后立即返回。后台失败通过通知与页面状态反馈。

独立 FluentLauncher-plugins 仓库已同步 SDK、协议文档、模板，以及剪贴板/开发工具的 showQuery 示例（两个官方插件的 manifest 版本为 1.0.1）。新版 SDK 和插件包尚需单独发布；本地构建不会改变用户已安装的插件。

## 验证记录（2026-11-10）

- 主仓库在 CGO_ENABLED=0 下通过 go build ./...、go vet ./...、go test ./...，并通过 go tool mygo vet . 和三语检查（639 条消息）。
- 快捷键测试覆盖旧配置迁移、显式清除/恢复默认、多作用域冲突、替换时保留其他按键、录制 Esc/Enter、三语与 820/1180 宽度构建、输入框/列表焦点下新键生效和旧键停用，以及热键失败恢复与备用呼出键保留。
- 插件测试覆盖未启动进程的直接命令、旧插件未声明能力、禁用/暂停/进程上限、重复触发去重、更新替换期间阻止执行与失败恢复；go test -race ./internal/plugin ./internal/core/shortcuts 通过。
- 独立插件根模块与 SDK 模块分别通过构建、静态检查和测试。本地打包成功，ZIP 内保留 commands（clipboard: show-history，devtools: show-tools）。
- Windows 原生隔离测试实际注册了临时热键，验证存在竞争注册时失败、旧注册恢复、成功改键后释放旧键，以及最终注销。
- 已检查快捷键列表和录制页的 MyGo 深浅色渲染预览；这属于渲染器检查，未声称人工桌面键盘端到端验收完成。
- go tool mygo build -skip-installer -o build/shortcut-work/app 通过，生成本地 Windows x64 EXE。未推送、未发布、未覆盖运行中的启动器或用户插件。
- 紧凑布局调整后重新通过完整构建、静态检查、测试、MyGo 检查及三语检查（642 条消息）。布局回归约束正常行间距为 32–48 DIP，实测为 40 DIP；补充验证了更多菜单的清除/恢复、Esc 关闭与焦点返回、Space 开关切换，以及 820 DIP 窗口和 150% 缩放的深浅色渲染。新版独立构建位于 build/shortcut-compact/app/windows-amd64/FluentLauncher.exe。
