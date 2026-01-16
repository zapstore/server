package relay

import "github.com/pippellia-btc/rely"

const (
	KindAppSet              = 30267
	KindSoftwareApplication = 32267
	KindSoftwareRelease     = 30063
	KindSoftwareAsset       = 3063
	KindFileMetadata        = 1063
)

func Setup(c Config) (*rely.Relay, error) {
	relay := rely.NewRelay(
		rely.WithDomain(c.Domain),
		rely.WithInfo(c.NIP11()),
	)
	return relay, nil
}
