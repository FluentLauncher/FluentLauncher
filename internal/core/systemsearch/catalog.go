package systemsearch

// settingsURI points at a protocol handler; Windows opens Settings pages with
// "ms-settings:" and a few tools use their own scheme.
func settingsURI(target string) Target {
	return Target{File: target, URI: true}
}

// command starts a program or module with fixed arguments.
func command(file, arguments string) Target {
	return Target{File: file, Arguments: arguments}
}

// catalog is the hand-written directory. It stays a value, not a database: the
// launcher only needs a few dozen stable entries, and a wrong target should be
// fixed in review rather than by a user.
var catalog = []Entry{
	// 系统
	{Key: "sysAboutYourPC", Title: "关于此电脑", Latin: "About your PC", Category: "系统",
		Keywords: []string{"关于", "系统信息", "版本", "激活", "产品密钥", "电脑信息", "about", "version", "activation", "winver", "關於您的電腦"},
		Target:   settingsURI("ms-settings:about")},
	{Key: "sysSystemProperties", Title: "系统属性", Latin: "System Properties", Category: "系统",
		Keywords: []string{"系统属性", "计算机名", "高级系统设置", "性能选项", "硬件", "设备安装", "system", "sysdm", "properties", "计算机", "高级", "系統內容"},
		Target:   command("control.exe", "sysdm.cpl")},
	{Key: "sysSystemInformation", Title: "系统信息", Latin: "System Information", Category: "系统",
		Keywords: []string{"系统信息", "硬件信息", "组件", "软件环境", "msinfo32", "sysinfo", "system information", "系統資訊"},
		Target:   command("msinfo32.exe", "")},
	{Key: "sysNotificationsAndActions", Title: "通知和操作", Latin: "Notifications", Category: "系统",
		Keywords: []string{"通知", "操作中心", "弹出通知", "专注助手", "免打扰", "notifications", "toast", "focus assist", "通知與動作"},
		Target:   settingsURI("ms-settings:notifications")},

	// 显示
	{Key: "sysDisplay", Title: "显示设置", Latin: "Display", Category: "显示",
		Keywords: []string{"显示", "显示器", "分辨率", "缩放", "刷新率", "亮度", "屏幕", "display", "resolution", "brightness", "scale", "顯示設定"},
		Target:   settingsURI("ms-settings:display")},
	{Key: "sysAdvancedDisplay", Title: "高级显示", Latin: "Advanced display", Category: "显示",
		Keywords: []string{"高级显示", "刷新率", "位深度", "颜色格式", "advanced display", "refresh rate", "進階顯示"},
		Target:   settingsURI("ms-settings:display-advanced")},
	{Key: "sysNightLight", Title: "夜间模式", Latin: "Night light", Category: "显示",
		Keywords: []string{"夜间模式", "护眼", "蓝光", "夜间光线", "色温", "night light", "blue light", "夜間燈光"},
		Target:   settingsURI("ms-settings:nightlight")},
	{Key: "sysGraphicsSettings", Title: "图形设置", Latin: "Graphics settings", Category: "显示",
		Keywords: []string{"图形设置", "显卡", "gpu", "硬件加速", "着色器", "graphics", "hardware accelerated", "圖形設定"},
		Target:   settingsURI("ms-settings:display-advancedgraphics")},
	{Key: "sysColorFilters", Title: "颜色筛选", Latin: "Color filters", Category: "显示",
		Keywords: []string{"颜色筛选", "色盲", "色彩滤镜", "滤镜", "color filter", "colorblind", "filter", "色彩濾鏡"},
		Target:   settingsURI("ms-settings:easeofaccess-colorfilter")},

	// 声音
	{Key: "sysSound", Title: "声音设置", Latin: "Sound", Category: "声音",
		Keywords: []string{"声音", "音量", "音频", "扬声器", "耳机", "sound", "volume", "audio", "speaker", "音效設定"},
		Target:   settingsURI("ms-settings:sound")},
	{Key: "sysOutputDevices", Title: "输出设备", Latin: "Output devices", Category: "声音",
		Keywords: []string{"输出设备", "播放设备", "扬声器", "耳机", "输出", "output", "playback", "headphones", "輸出裝置"},
		Target:   settingsURI("ms-settings:sound-devices")},
	{Key: "sysAppVolume", Title: "应用音量", Latin: "App volume", Category: "声音",
		Keywords: []string{"应用音量", "音量混合器", "单个应用", "app volume", "mixer", "應用程式音量"},
		Target:   settingsURI("ms-settings:apps-volume")},
	{Key: "sysSoundControlPanel", Title: "声音控制面板", Latin: "Sound control panel", Category: "声音",
		Keywords: []string{"声音", "播放设备", "录音设备", "声音方案", "音频设备", "旧版声音", "sound", "mmsys", "playback", "recording", "音效控制台"},
		Target:   command("control.exe", "mmsys.cpl")},
	{Key: "sysSpeech", Title: "语音", Latin: "Speech", Category: "声音",
		Keywords: []string{"语音", "语音合成", "朗读", "语音识别", "说话人", "speech", "voice", "tts", "語音"},
		Target:   settingsURI("ms-settings:speech")},

	// 网络
	{Key: "sysNetworkStatus", Title: "网络状态", Latin: "Network status", Category: "网络",
		Keywords: []string{"网络", "网络状态", "连接", "网络重置", "internet", "network", "status", "连接状态", "網路狀態"},
		Target:   settingsURI("ms-settings:network-status")},
	{Key: "sysWiFi", Title: "Wi-Fi", Latin: "Wi-Fi", Category: "网络",
		Keywords: []string{"无线网络", "无线", "局域网", "热点", "wifi", "wi-fi", "wlan", "wireless"},
		Target:   settingsURI("ms-settings:network-wifi")},
	{Key: "sysEthernet", Title: "以太网", Latin: "Ethernet", Category: "网络",
		Keywords: []string{"以太网", "有线", "本地连接", "网线", "ethernet", "lan", "有线网络", "乙太網路"},
		Target:   settingsURI("ms-settings:network-ethernet")},
	{Key: "sysNetworkConnections", Title: "网络连接", Latin: "Network Connections", Category: "网络",
		Keywords: []string{"网络连接", "适配器", "更改适配器设置", "网卡", "ncpa", "adapters", "network connections", "網路連線"},
		Target:   command("control.exe", "/name Microsoft.NetworkConnections")},
	{Key: "sysVPN", Title: "VPN", Latin: "VPN", Category: "网络",
		Keywords: []string{"虚拟专用网络", "专线", "远程访问", "vpn", "虚拟专用"},
		Target:   settingsURI("ms-settings:network-vpn")},
	{Key: "sysProxy", Title: "代理", Latin: "Proxy", Category: "网络",
		Keywords: []string{"代理服务器", "代理设置", "局域网代理", "proxy", "socks", "端口"},
		Target:   settingsURI("ms-settings:network-proxy")},

	// 蓝牙与设备
	{Key: "sysBluetooth", Title: "蓝牙", Latin: "Bluetooth", Category: "蓝牙与设备",
		Keywords: []string{"蓝牙设备", "配对", "蓝牙开关", "bluetooth", "bt", "pair", "藍牙"},
		Target:   settingsURI("ms-settings:bluetooth")},
	{Key: "sysConnectedDevices", Title: "已连接设备", Latin: "Connected devices", Category: "蓝牙与设备",
		Keywords: []string{"设备", "已连接设备", "外设", "配对设备", "添加设备", "devices", "peripherals", "已連接裝置"},
		Target:   settingsURI("ms-settings:connecteddevices")},
	{Key: "sysUSBSettings", Title: "USB 设置", Latin: "USB", Category: "蓝牙与设备",
		Keywords: []string{"usb", "u盘", "移动设备", "usb 连接通知", "USB 設定"},
		Target:   settingsURI("ms-settings:usb")},
	{Key: "sysPenAndInk", Title: "触控笔和手写", Latin: "Pen & Windows Ink", Category: "蓝牙与设备",
		Keywords: []string{"触控笔", "手写", "笔", "手写笔", "pen", "stylus", "ink", "手寫筆與 Windows Ink"},
		Target:   settingsURI("ms-settings:pen")},
	{Key: "sysDeviceManager", Title: "设备管理器", Latin: "Device Manager", Category: "蓝牙与设备",
		Keywords: []string{"设备管理器", "驱动", "更新驱动", "硬件", "devmgmt", "device manager", "driver", "裝置管理員"},
		Target:   command("devmgmt.msc", "")},

	// 打印机
	{Key: "sysPrintersAndScanners", Title: "打印机和扫描仪", Latin: "Printers & scanners", Category: "打印机",
		Keywords: []string{"打印机", "扫描仪", "添加打印机", "打印队列", "printer", "scanner", "printing", "印表機與掃描器"},
		Target:   settingsURI("ms-settings:printers")},
	{Key: "sysDevicesAndPrinters", Title: "设备和打印机", Latin: "Devices and Printers", Category: "打印机",
		Keywords: []string{"设备和打印机", "旧版打印机", "添加打印机", "printer", "devices", "默认打印机", "裝置與印表機"},
		Target:   command("control.exe", "/name Microsoft.DevicesAndPrinters")},
	{Key: "sysFaxAndScan", Title: "Windows 传真和扫描", Latin: "Windows Fax and Scan", Category: "打印机",
		Keywords: []string{"传真", "扫描", "扫描文档", "fax", "scan", "wfs", "Windows 傳真與掃描"},
		Target:   command("wfs.exe", "")},

	// 电源
	{Key: "sysPowerAndBattery", Title: "电源和电池", Latin: "Power & battery", Category: "电源",
		Keywords: []string{"电源", "电池", "睡眠", "电源模式", "电源和电池", "power", "battery", "sleep", "電源與電池"},
		Target:   settingsURI("ms-settings:powersleep")},
	{Key: "sysPowerOptions", Title: "电源选项", Latin: "Power Options", Category: "电源",
		Keywords: []string{"电源选项", "电源计划", "高性能", "电源按钮", "关闭盖子", "powercfg", "power plan", "旧版电源", "電源選項"},
		Target:   command("control.exe", "powercfg.cpl")},
	{Key: "sysBatterySaver", Title: "节电模式", Latin: "Battery saver", Category: "电源",
		Keywords: []string{"节电模式", "省电", "电池节省", "battery saver", "power saving", "省電模式"},
		Target:   settingsURI("ms-settings:batterysaver")},

	// 更新与安全
	{Key: "sysWindowsUpdate", Title: "Windows 更新", Latin: "Windows Update", Category: "更新与安全",
		Keywords: []string{"更新", "windows 更新", "检查更新", "补丁", "升级", "update", "patch", "wu", "Windows Update"},
		Target:   settingsURI("ms-settings:windowsupdate")},
	{Key: "sysUpdateHistory", Title: "更新历史记录", Latin: "Update history", Category: "更新与安全",
		Keywords: []string{"更新历史", "已安装更新", "更新记录", "update history", "补丁记录", "更新歷程記錄"},
		Target:   settingsURI("ms-settings:windowsupdate-history")},
	{Key: "sysWindowsSecurity", Title: "Windows 安全中心", Latin: "Windows Security", Category: "更新与安全",
		Keywords: []string{"安全中心", "defender", "杀毒", "病毒", "威胁防护", "security", "antivirus", "防火墙", "Windows 安全性"},
		Target:   settingsURI("ms-settings:windowsdefender")},
	{Key: "sysFirewall", Title: "防火墙", Latin: "Windows Defender Firewall", Category: "更新与安全",
		Keywords: []string{"防火墙", "入站规则", "出站规则", "端口", "firewall", "defender firewall", "防火牆"},
		Target:   command("control.exe", "firewall.cpl")},
	{Key: "sysRecovery", Title: "恢复", Latin: "Recovery", Category: "更新与安全",
		Keywords: []string{"恢复", "重置此电脑", "系统还原", "高级启动", "recovery", "reset", "winre", "還原"},
		Target:   settingsURI("ms-settings:recovery")},
	{Key: "sysWindowsBackup", Title: "Windows 备份", Latin: "Windows Backup", Category: "更新与安全",
		Keywords: []string{"备份", "还原点", "文件历史", "backup", "restore point", "file history", "Windows 備份"},
		Target:   settingsURI("ms-settings:backup")},

	// 账户
	{Key: "sysYourInfo", Title: "账户信息", Latin: "Your info", Category: "账户",
		Keywords: []string{"账户", "头像", "微软账户", "本地账户", "account", "profile", "your info", "帳戶資訊"},
		Target:   settingsURI("ms-settings:yourinfo")},
	{Key: "sysSignInOptions", Title: "登录选项", Latin: "Sign-in options", Category: "账户",
		Keywords: []string{"登录选项", "密码", "pin", "指纹", "人脸识别", "windows hello", "sign-in", "password", "fingerprint", "登入選項"},
		Target:   settingsURI("ms-settings:signinoptions")},
	{Key: "sysOtherUsers", Title: "其他用户", Latin: "Other users", Category: "账户",
		Keywords: []string{"其他用户", "添加账户", "家庭成员", "新用户", "users", "add account", "其他使用者"},
		Target:   settingsURI("ms-settings:otherusers")},
	{Key: "sysEmailAndAccounts", Title: "电子邮件和账户", Latin: "Email & accounts", Category: "账户",
		Keywords: []string{"电子邮件", "邮箱", "工作账户", "email", "accounts", "work account", "電子郵件與帳戶"},
		Target:   settingsURI("ms-settings:emailandaccounts")},
	{Key: "sysSyncSettings", Title: "同步设置", Latin: "Sync your settings", Category: "账户",
		Keywords: []string{"同步", "云同步", "漫游", "sync", "roaming", "主题同步", "同步設定"},
		Target:   settingsURI("ms-settings:sync")},

	// 时间与语言
	{Key: "sysDateAndTime", Title: "日期和时间", Latin: "Date & time", Category: "时间与语言",
		Keywords: []string{"日期", "时间", "时区", "自动设置时间", "date", "time", "timezone", "日期與時間"},
		Target:   settingsURI("ms-settings:dateandtime")},
	{Key: "sysDateAndTimeControlPanel", Title: "日期和时间控制面板", Latin: "Date and Time", Category: "时间与语言",
		Keywords: []string{"日期时间", "时区", "时间同步", "时钟", "internet 时间", "timedate", "clock", "日期與時間控制台"},
		Target:   command("control.exe", "timedate.cpl")},
	{Key: "sysLanguageAndRegion", Title: "语言和区域", Latin: "Language & region", Category: "时间与语言",
		Keywords: []string{"语言", "区域", "输入法", "添加语言", "中文", "language", "region", "locale", "ime", "語言與區域"},
		Target:   settingsURI("ms-settings:regionlanguage")},
	{Key: "sysRegionFormatting", Title: "区域格式", Latin: "Region formatting", Category: "时间与语言",
		Keywords: []string{"区域格式", "日期格式", "数字格式", "region format", "format", "格式", "區域格式"},
		Target:   settingsURI("ms-settings:regionformatting")},

	// 应用与卸载
	{Key: "sysInstalledApps", Title: "已安装的应用", Latin: "Installed apps", Category: "应用与卸载",
		Keywords: []string{"应用", "已安装应用", "应用列表", "卸载", "apps", "installed", "uninstall", "已安裝的應用程式"},
		Target:   settingsURI("ms-settings:appsfeatures")},
	{Key: "sysDefaultApps", Title: "默认应用", Latin: "Default apps", Category: "应用与卸载",
		Keywords: []string{"默认应用", "默认程序", "文件关联", "default apps", "associations", "預設應用程式"},
		Target:   settingsURI("ms-settings:defaultapps")},
	{Key: "sysStartupApps", Title: "启动应用", Latin: "Startup apps", Category: "应用与卸载",
		Keywords: []string{"启动应用", "开机启动", "启动项", "startup", "boot", "autorun", "啟動應用程式"},
		Target:   settingsURI("ms-settings:startupapps")},
	{Key: "sysOptionalFeatures", Title: "可选功能", Latin: "Optional features", Category: "应用与卸载",
		Keywords: []string{"可选功能", "添加功能", "windows 功能", "optional features", "features", "選用功能"},
		Target:   settingsURI("ms-settings:optionalfeatures")},
	{Key: "sysProgramsAndFeatures", Title: "程序和功能", Latin: "Programs and Features", Category: "应用与卸载",
		Keywords: []string{"程序和功能", "卸载程序", "卸载", "修改安装", "appwiz", "programs", "uninstall", "程式與功能"},
		Target:   command("control.exe", "/name Microsoft.ProgramsAndFeatures")},

	// 鼠标键盘
	{Key: "sysMouse", Title: "鼠标", Latin: "Mouse", Category: "鼠标键盘",
		Keywords: []string{"鼠标", "指针", "光标", "滚轮", "双击速度", "mouse", "pointer", "cursor", "滑鼠"},
		Target:   settingsURI("ms-settings:mousetouchpad")},
	{Key: "sysMouseProperties", Title: "鼠标属性", Latin: "Mouse Properties", Category: "鼠标键盘",
		Keywords: []string{"鼠标属性", "指针速度", "鼠标键", "指针方案", "main.cpl", "mouse properties", "滑鼠內容"},
		Target:   command("control.exe", "main.cpl")},
	{Key: "sysKeyboard", Title: "键盘", Latin: "Keyboard", Category: "鼠标键盘",
		Keywords: []string{"键盘", "按键", "重复延迟", "光标闪烁", "筛选键", "keyboard", "keys", "鍵盤"},
		Target:   command("control.exe", "keyboard")},
	{Key: "sysTyping", Title: "输入", Latin: "Typing", Category: "鼠标键盘",
		Keywords: []string{"输入", "拼写检查", "文本建议", "触摸键盘", "typing", "spell check", "輸入"},
		Target:   settingsURI("ms-settings:typing")},
	{Key: "sysTouchpad", Title: "触摸板", Latin: "Touchpad", Category: "鼠标键盘",
		Keywords: []string{"触摸板", "触控板", "手势", "轻触", "touchpad", "gestures", "觸控板"},
		Target:   settingsURI("ms-settings:devices-touchpad")},

	// 存储
	{Key: "sysStorageSense", Title: "存储感知", Latin: "Storage sense", Category: "存储",
		Keywords: []string{"存储", "存储感知", "磁盘空间", "清理", "临时文件", "storage", "cleanup", "disk space", "儲存空間感知"},
		Target:   settingsURI("ms-settings:storagesense")},
	{Key: "sysDisksAndVolumes", Title: "磁盘和卷", Latin: "Disks & volumes", Category: "存储",
		Keywords: []string{"磁盘", "卷", "分区", "磁盘和卷", "disks", "volumes", "partition", "磁碟與磁碟區"},
		Target:   settingsURI("ms-settings:disksandvolumes")},
	{Key: "sysSaveLocations", Title: "保存位置", Latin: "Save locations", Category: "存储",
		Keywords: []string{"保存位置", "新内容保存位置", "默认位置", "save locations", "storage", "儲存位置"},
		Target:   settingsURI("ms-settings:savelocations")},
	{Key: "sysDiskManagement", Title: "磁盘管理", Latin: "Disk Management", Category: "存储",
		Keywords: []string{"磁盘管理", "分区", "格式化", "压缩卷", "diskmgmt", "disk management", "partition", "磁碟管理"},
		Target:   command("diskmgmt.msc", "")},

	// 隐私
	{Key: "sysPrivacyAndSecurity", Title: "隐私和安全性", Latin: "Privacy & security", Category: "隐私",
		Keywords: []string{"隐私", "隐私设置", "权限", "privacy", "permissions", "安全性", "隱私與安全性"},
		Target:   settingsURI("ms-settings:privacy")},
	{Key: "sysLocationServices", Title: "位置服务", Latin: "Location", Category: "隐私",
		Keywords: []string{"位置", "定位", "地理位置", "gps", "location", "定位服务", "位置服務"},
		Target:   settingsURI("ms-settings:privacy-location")},
	{Key: "sysCamera", Title: "相机", Latin: "Camera", Category: "隐私",
		Keywords: []string{"相机", "摄像头", "相机权限", "camera", "webcam", "相機"},
		Target:   settingsURI("ms-settings:privacy-camera")},
	{Key: "sysMicrophone", Title: "麦克风", Latin: "Microphone", Category: "隐私",
		Keywords: []string{"麦克风", "话筒", "录音权限", "microphone", "mic", "麥克風"},
		Target:   settingsURI("ms-settings:privacy-microphone")},
	{Key: "sysDiagnosticsAndFeedback", Title: "诊断和反馈", Latin: "Diagnostics & feedback", Category: "隐私",
		Keywords: []string{"诊断", "反馈", "遥测", "diagnostics", "feedback", "telemetry", "診斷與意見反應"},
		Target:   settingsURI("ms-settings:privacy-feedback")},

	// 开发者选项
	{Key: "sysForDevelopers", Title: "开发者选项", Latin: "For developers", Category: "开发者选项",
		Keywords: []string{"开发者", "开发人员模式", "旁加载", "远程调试", "developer", "dev mode", "sideload", "開發者選項"},
		Target:   settingsURI("ms-settings:developers")},
	{Key: "sysEnvironmentVariables", Title: "环境变量", Latin: "Environment Variables", Category: "开发者选项",
		Keywords: []string{"环境变量", "系统变量", "用户变量", "path", "environment", "env", "環境變數"},
		Target:   command("rundll32.exe", "sysdm.cpl,EditEnvironmentVariables")},
	{Key: "sysSystemConfiguration", Title: "系统配置", Latin: "System Configuration", Category: "开发者选项",
		Keywords: []string{"系统配置", "启动项", "引导", "安全模式", "msconfig", "boot", "startup", "系統設定"},
		Target:   command("msconfig.exe", "")},

	// 服务
	{Key: "sysServices", Title: "服务", Latin: "Services", Category: "服务",
		Keywords: []string{"服务", "启动类型", "后台服务", "自动启动", "services", "service manager", "scm", "服務"},
		Target:   command("services.msc", "")},
	{Key: "sysEventViewer", Title: "事件查看器", Latin: "Event Viewer", Category: "服务",
		Keywords: []string{"事件查看器", "日志", "系统日志", "错误日志", "event viewer", "logs", "eventvwr", "事件檢視器"},
		Target:   command("eventvwr.msc", "")},
	{Key: "sysComputerManagement", Title: "计算机管理", Latin: "Computer Management", Category: "服务",
		Keywords: []string{"计算机管理", "管理工具", "本地用户和组", "compmgmt", "computer management", "電腦管理"},
		Target:   command("compmgmt.msc", "")},

	// 任务管理器
	{Key: "sysTaskManager", Title: "任务管理器", Latin: "Task Manager", Category: "任务管理器",
		Keywords: []string{"任务管理器", "进程", "结束进程", "性能", "启动影响", "taskmgr", "task manager", "process", "工作管理員"},
		Target:   command("taskmgr.exe", "")},
	{Key: "sysResourceMonitor", Title: "资源监视器", Latin: "Resource Monitor", Category: "任务管理器",
		Keywords: []string{"资源监视器", "cpu", "内存", "磁盘占用", "resmon", "resource monitor", "資源監視器"},
		Target:   command("resmon.exe", "")},

	// 注册表编辑器
	{Key: "sysRegistryEditor", Title: "注册表编辑器", Latin: "Registry Editor", Category: "注册表编辑器",
		Keywords: []string{"注册表", "注册表编辑器", "键值", "regedit", "registry", "reg", "登錄編輯程式"},
		Target:   command("regedit.exe", "")},

	// 控制面板
	{Key: "sysControlPanel", Title: "控制面板", Latin: "Control Panel", Category: "控制面板",
		Keywords: []string{"控制面板", "经典控制面板", "所有控制面板项", "control panel", "cpl", "控制台"},
		Target:   command("control.exe", "")},
	{Key: "sysAdministrativeTools", Title: "管理工具", Latin: "Administrative Tools", Category: "控制面板",
		Keywords: []string{"管理工具", "windows 工具", "系统工具", "administrative tools", "tools", "系統管理工具"},
		Target:   command("control.exe", "/name Microsoft.AdministrativeTools")},
	{Key: "sysCredentialManager", Title: "凭据管理器", Latin: "Credential Manager", Category: "控制面板",
		Keywords: []string{"凭据", "凭据管理器", "密码管理", "credential manager", "password", "windows 凭据", "認證管理員"},
		Target:   command("control.exe", "/name Microsoft.CredentialManager")},
	{Key: "sysFonts", Title: "字体", Latin: "Fonts", Category: "控制面板",
		Keywords: []string{"字体", "安装字体", "字体预览", "fonts", "font", "字型"},
		Target:   command("control.exe", "/name Microsoft.Fonts")},
	{Key: "sysInternetOptions", Title: "Internet 选项", Latin: "Internet Options", Category: "控制面板",
		Keywords: []string{"internet 选项", "ie", "浏览器设置", "代理", "高级设置", "inetcpl", "internet options", "網際網路選項"},
		Target:   command("control.exe", "inetcpl.cpl")},
}

// index is the catalog with the matcher's precomputed fields. Go orders the two
// package variables so that the index is built after the catalog.
var index = buildIndex(catalog)
