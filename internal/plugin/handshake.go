package plugin

import (
	"errors"

	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
)

// handshakeError checks the reply a plugin gave to initialize. The two
// failures it can report are the ones a plugin author meets first, and each
// names what disagreed: "握手失败" alone left them with nothing to fix.
func handshakeError(initialized protocol.InitializeResult, id string) error {
	if initialized.ProtocolVersion != protocol.ProtocolVersion {
		return errors.New(locales.PluginHandshakeProtocolVersion(initialized.ProtocolVersion, protocol.ProtocolVersion))
	}
	if initialized.PluginID != id {
		return errors.New(locales.PluginHandshakeIDMismatch(initialized.PluginID, id))
	}
	return nil
}
