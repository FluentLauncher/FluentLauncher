# 音量浮层跨屏定位修复

日期：2026-11-13

## 问题与原因

屏幕边缘滚轮能改变系统音量，但提示浮层可能出现在可见屏幕之外。原实现使用 MyGo 返回的目标屏幕 DIP 坐标，再调用 `Window.SetBounds`；MyGo 0.7.4 在 Windows 上按窗口当前 DPI 将这些坐标转换成物理像素。窗口当前屏幕与目标屏幕缩放不一致时，定位会产生偏移。

本机三屏环境（主屏 150%，上方与左侧屏幕 100%）已复现：左屏工作区为 `(-1919,-491)-(1,541)`，浮层按 DIP 定位后，实际物理矩形落到 `(-1618,662)-(-1258,728)`，超出了左屏底边。

## 修复

- 由 Windows 根据鼠标的物理坐标选择屏幕，直接读取该屏幕的物理工作区和 DPI。
- 将浮层宽高及底部间距转换为目标 DPI 的像素尺寸，再使用 `SetWindowPos` 定位，保留置顶、不激活与鼠标穿透行为。
- 跨屏触发 `WM_DPICHANGED` 后，重新应用已计算好的像素矩形，避免 MyGo 再次缩放窗口尺寸。
- 在同一次 UI 线程更新内完成百分比更新与浮层显示。

## 验证

- `ui.NewTester` 验证深浅主题下百分比更新、0/100 与范围限制，文字保持在胶囊内。
- 几何回归覆盖 100%、125%、150%、200% 缩放以及负坐标屏幕。
- 原生测试复用同一透明、鼠标穿透窗口，在本机三屏之间往返，检查实际物理矩形与键盘焦点。全程不移动鼠标、不改变系统音量、不读取启动器个人配置。
- `go build ./...`、`go vet ./...`、`CGO_ENABLED=0 go test ./...`、`go tool mygo vet .` 和 `go test -race ./internal/app ./internal/winapi/volume` 均通过。
- `go tool mygo build -platform windows/amd64 -skip-installer -o build/volume-overlay-fix` 已生成本地试用程序。

原生测试默认跳过；在 Windows 桌面会话内按需执行：

```powershell
$env:FL_VOLUME_NATIVE_TEST = '1'
try {
    go test ./internal/winapi/volume -run '^TestOverlayNativeCrossDPI$' -count=1 -v
} finally {
    Remove-Item Env:FL_VOLUME_NATIVE_TEST
}
```

原生定位与焦点检查已经通过；已安装程序仍需更新并重启后，才能验收真实滚轮触发、连续滚动以及 1.2 秒自动隐藏的完整流程。
