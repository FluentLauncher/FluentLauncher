package plugin

import (
	"fmt"

	"fluentlauncher/internal/plugin/protocol"
)

// handshakeError checks the reply a plugin gave to initialize. The two
// failures it can report are the ones a plugin author meets first, and each
// names what disagreed: "握手失败" alone left them with nothing to fix.
func handshakeError(initialized protocol.InitializeResult, id string) error {
	if initialized.ProtocolVersion != protocol.ProtocolVersion {
		return fmt.Errorf("插件使用协议版本 %d，本启动器使用 %d", initialized.ProtocolVersion, protocol.ProtocolVersion)
	}
	if initialized.PluginID != id {
		return fmt.Errorf("插件声明的标识是 %q，与包内 plugin.json 的 %q 不一致", initialized.PluginID, id)
	}
	return nil
}
