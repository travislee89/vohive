package backend

import "github.com/travislee89/vohive/pkg/mbim"

func (b *MBIMBackend) Capability() *mbim.Capabilities {
	return b.source.Capability()
}
