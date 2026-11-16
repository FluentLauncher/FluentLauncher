# MyGo 上游 issue 草稿：Windows 下进程退出时挂死

状态：已提交为 [egoist/mygo#1896](https://github.com/egoist/mygo/issues/1896)，由 [#1897](https://github.com/egoist/mygo/pull/1897)（2026-11-15 合并）修复，等待发布：截至 2026-11-16 最新正式版 `v0.9.1` 早于该修复，尚不包含。分隔线以下是提交时的英文正文。

本项目的规避方式见 `internal/winapi/teardown` 和 `internal/app/app.go` 里的 `OnQuit`（提交 `4a69786`）。升级到包含 #1897 的 MyGo 正式版后移除；从当前的 `v0.7.4` 跨越多个版本，升级需单独验证构建、窗口、输入法和更新后重启。

---

**Title:** Windows: process never exits when an IME destroys a child of the hidden app window during `ExitProcess`

## Summary

On Windows, an app can stay behind forever after `App.Quit()` / `App.Relaunch()`. The quit sequence completes (`OnQuit` listeners run, the relaunched instance starts), but the old process never goes away: it is left with a single thread, blocked inside `ExitProcess`.

The cause is that the hidden application window (class `MyGoApp`) is still alive when the process exits. If anything destroys a child of that window while DLLs are unloading, Windows calls the window's Go window procedure inside `ExitProcess`, and the Go runtime deadlocks.

## Environment

- MyGo v0.7.4 (the exit path in `internal/windows/app.go` is unchanged in v0.9.1)
- Go 1.27.3, windows/amd64
- Windows 11 Pro 10.0.26300
- Doubao IME 1.0.0.23 installed and active (a third-party TSF input method; `tsf-oime-core.dll`)

## What happens

1. `internal/windows/app.go` creates the app window with `createWindow(0, appClass, ...)` and never destroys it. After the loop in `Backend.Run` ends, `Application.finish()` runs and the process exits (return from `main`, or `os.Exit` in `forceStop` / `Exit`).
2. The input method creates a helper window, class `OimeTsfNotifyWindow`, as a **child of the `MyGoApp` window**.
3. `runtime.exit` on Windows takes `suspendLock` and calls `ExitProcess` without releasing it:

   ```go
   func exit(code int32) {
       lock(&suspendLock)
       atomic.Store(&exiting, 1)
       stdcall(_ExitProcess, uintptr(code))
   }
   ```

4. `ExitProcess` terminates every other thread and runs `DLL_PROCESS_DETACH`. `MSCTF.dll` tears down its thread manager, the IME's DLL calls `DestroyWindow` on its helper window, and Windows sends `WM_PARENTNOTIFY` (`WM_DESTROY`) to the parent, the `MyGoApp` window.
5. That enters `wndProc` through `runtime.callbackasm` on a thread that is already inside `runtime.exit`. The runtime throws, `fatalthrow` calls `exit` again, and `lock(&suspendLock)` waits for a lock the same thread already holds. Nothing can release it.

## Evidence

Stack of the only remaining thread (the UI thread), innermost first. System frames are from export symbols; Go frames were resolved from the binary's pclntab.

```bash
ntdll!NtWaitForSingleObject
KERNELBASE!WaitForSingleObjectEx
runtime.semasleep
runtime.lock2
runtime.exit                     <- second entry, waits for suspendLock
runtime.startpanic_m
runtime.fatalthrow
runtime.throw
runtime.callbackasm1
runtime.callbackasm              <- Go window procedure entered
USER32!CallWindowProcW           hwnd = the MyGoApp window, msg = 0x210 WM_PARENTNOTIFY,
                                 wParam = 2 (WM_DESTROY), lParam = the OimeTsfNotifyWindow child
ntdll!KiUserCallbackDispatcher
win32u!NtUserDestroyWindow
tsf-oime-core.dll                <- the input method, during detach
MSCTF.dll  (CtfImeDestroyThreadMgr ...)
ntdll!LdrShutdownProcess
ntdll!RtlExitUserProcess
KERNEL32!ExitProcess
runtime.exit                     <- first entry, holds suspendLock
```

State of the stuck process:

- 1 thread, state Waiting; `GetExitCodeProcess` already reports 0
- still owns the hidden `MyGoApp` window, with one child of class `OimeTsfNotifyWindow`
- every `MyGoWindow` was closed by the quit sequence as expected

It was found hours after an update: `Updater` installed a new version, `App.Relaunch()` started the new instance, and the old one was still listed in Task Manager. Because the old executable is still mapped, `.<name>.exe.old-<pid>` also cannot be cleaned up by `cleanUpdateLeftovers`.

## Reproduction

With Doubao IME as the active input method:

1. Start any MyGo app with a window and give it keyboard focus.
2. Check that the `MyGoApp` window has an `OimeTsfNotifyWindow` child (Spy++, or `EnumChildWindows`).
3. Quit the app.
4. The process remains with one thread.

The input method is only the trigger. Any DLL that destroys a child or owned window of the app window in `DLL_PROCESS_DETACH` leads to the same deadlock, as would any other message delivered to a MyGo window class at that point.

## Suggested fix

Do not let a window with a Go window procedure survive into `ExitProcess`. Once the event loop has stopped, on the main thread and before the process exits, destroy the app window and any other remaining window of MyGo's classes (`MyGoApp`, `MyGoWindow`, the caption and flyout shadow classes). This has to cover every exit path: the normal return from `Run`, `forceStop`, and `Application.Exit`.

Destroying the app window at that point also destroys the IME's child while the Go runtime is still healthy, so the notification is handled normally.

## Workaround

In the app, from an `OnQuit` listener (it runs on the main thread after the loop has stopped), enumerate the thread's top-level windows with `EnumThreadWindows` and call `DestroyWindow` on those whose class name starts with `MyGo`.
