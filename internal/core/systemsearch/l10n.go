package systemsearch

import (
	"fluentlauncher/locales"
)

// DisplayTitle returns the localized title of e for the current locale. The
// catalog's Title field always carries the zh-Hans source text; translations
// leave through this function only, so matching, pinyin initials and tests
// keep operating on the original Chinese titles.
func DisplayTitle(e Entry) string {
	if lookup, ok := titleGetters[e.Key]; ok {
		return lookup()
	}
	return e.Title
}

// DisplayCategory returns the localized category of e for the current locale.
func DisplayCategory(e Entry) string {
	if lookup, ok := categoryGetters[e.Category]; ok {
		return lookup()
	}
	return e.Category
}

// titleGetters maps each catalog Key to the locales.Sys* getter that renders
// its translated title.
var titleGetters = map[string]func() string{
	"sysAboutYourPC":             locales.SysAboutYourPC,
	"sysSystemProperties":        locales.SysSystemProperties,
	"sysSystemInformation":       locales.SysSystemInformation,
	"sysNotificationsAndActions": locales.SysNotificationsAndActions,
	"sysDisplay":                 locales.SysDisplay,
	"sysAdvancedDisplay":         locales.SysAdvancedDisplay,
	"sysNightLight":              locales.SysNightLight,
	"sysGraphicsSettings":        locales.SysGraphicsSettings,
	"sysColorFilters":            locales.SysColorFilters,
	"sysSound":                   locales.SysSound,
	"sysOutputDevices":           locales.SysOutputDevices,
	"sysAppVolume":               locales.SysAppVolume,
	"sysSoundControlPanel":       locales.SysSoundControlPanel,
	"sysSpeech":                  locales.SysSpeech,
	"sysNetworkStatus":           locales.SysNetworkStatus,
	"sysWiFi":                    locales.SysWiFi,
	"sysEthernet":                locales.SysEthernet,
	"sysNetworkConnections":      locales.SysNetworkConnections,
	"sysVPN":                     locales.SysVPN,
	"sysProxy":                   locales.SysProxy,
	"sysBluetooth":               locales.SysBluetooth,
	"sysConnectedDevices":        locales.SysConnectedDevices,
	"sysUSBSettings":             locales.SysUSBSettings,
	"sysPenAndInk":               locales.SysPenAndInk,
	"sysDeviceManager":           locales.SysDeviceManager,
	"sysPrintersAndScanners":     locales.SysPrintersAndScanners,
	"sysDevicesAndPrinters":      locales.SysDevicesAndPrinters,
	"sysFaxAndScan":              locales.SysFaxAndScan,
	"sysPowerAndBattery":         locales.SysPowerAndBattery,
	"sysPowerOptions":            locales.SysPowerOptions,
	"sysBatterySaver":            locales.SysBatterySaver,
	"sysWindowsUpdate":           locales.SysWindowsUpdate,
	"sysUpdateHistory":           locales.SysUpdateHistory,
	"sysWindowsSecurity":         locales.SysWindowsSecurity,
	"sysFirewall":                locales.SysFirewall,
	"sysRecovery":                locales.SysRecovery,
	"sysWindowsBackup":           locales.SysWindowsBackup,
	"sysYourInfo":                locales.SysYourInfo,
	"sysSignInOptions":           locales.SysSignInOptions,
	"sysOtherUsers":              locales.SysOtherUsers,
	"sysEmailAndAccounts":        locales.SysEmailAndAccounts,
	"sysSyncSettings":            locales.SysSyncSettings,
	"sysDateAndTime":             locales.SysDateAndTime,
	"sysDateAndTimeControlPanel": locales.SysDateAndTimeControlPanel,
	"sysLanguageAndRegion":       locales.SysLanguageAndRegion,
	"sysRegionFormatting":        locales.SysRegionFormatting,
	"sysInstalledApps":           locales.SysInstalledApps,
	"sysDefaultApps":             locales.SysDefaultApps,
	"sysStartupApps":             locales.SysStartupApps,
	"sysOptionalFeatures":        locales.SysOptionalFeatures,
	"sysProgramsAndFeatures":     locales.SysProgramsAndFeatures,
	"sysMouse":                   locales.SysMouse,
	"sysMouseProperties":         locales.SysMouseProperties,
	"sysKeyboard":                locales.SysKeyboard,
	"sysTyping":                  locales.SysTyping,
	"sysTouchpad":                locales.SysTouchpad,
	"sysStorageSense":            locales.SysStorageSense,
	"sysDisksAndVolumes":         locales.SysDisksAndVolumes,
	"sysSaveLocations":           locales.SysSaveLocations,
	"sysDiskManagement":          locales.SysDiskManagement,
	"sysPrivacyAndSecurity":      locales.SysPrivacyAndSecurity,
	"sysLocationServices":        locales.SysLocationServices,
	"sysCamera":                  locales.SysCamera,
	"sysMicrophone":              locales.SysMicrophone,
	"sysDiagnosticsAndFeedback":  locales.SysDiagnosticsAndFeedback,
	"sysForDevelopers":           locales.SysForDevelopers,
	"sysEnvironmentVariables":    locales.SysEnvironmentVariables,
	"sysSystemConfiguration":     locales.SysSystemConfiguration,
	"sysServices":                locales.SysServices,
	"sysEventViewer":             locales.SysEventViewer,
	"sysComputerManagement":      locales.SysComputerManagement,
	"sysTaskManager":             locales.SysTaskManager,
	"sysResourceMonitor":         locales.SysResourceMonitor,
	"sysRegistryEditor":          locales.SysRegistryEditor,
	"sysControlPanel":            locales.SysControlPanel,
	"sysAdministrativeTools":     locales.SysAdministrativeTools,
	"sysCredentialManager":       locales.SysCredentialManager,
	"sysFonts":                   locales.SysFonts,
	"sysInternetOptions":         locales.SysInternetOptions,
}

// categoryGetters maps each catalog Category value to the locales.SysCat*
// getter that renders its translated name.
var categoryGetters = map[string]func() string{
	"系统":     locales.SysCatSystem,
	"显示":     locales.SysCatDisplay,
	"声音":     locales.SysCatSound,
	"网络":     locales.SysCatNetwork,
	"蓝牙与设备":  locales.SysCatBluetoothAndDevices,
	"打印机":    locales.SysCatPrinters,
	"电源":     locales.SysCatPower,
	"更新与安全":  locales.SysCatUpdateAndSecurity,
	"账户":     locales.SysCatAccounts,
	"时间与语言":  locales.SysCatTimeAndLanguage,
	"应用与卸载":  locales.SysCatAppsAndUninstall,
	"鼠标键盘":   locales.SysCatMouseAndKeyboard,
	"存储":     locales.SysCatStorage,
	"隐私":     locales.SysCatPrivacy,
	"开发者选项":  locales.SysCatDeveloperOptions,
	"服务":     locales.SysCatServices,
	"任务管理器":  locales.SysCatTaskManager,
	"注册表编辑器": locales.SysCatRegistryEditor,
	"控制面板":   locales.SysCatControlPanel,
}
