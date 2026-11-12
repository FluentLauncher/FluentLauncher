//go:build windows

// Package sysinfo exposes non-identifying Windows version information.
package sysinfo

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func Version() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}
