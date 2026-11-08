package eventbridge

import (
	"context"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
	platformevents "github.com/Mayer-04/kenko-profile-go/internal/platform/events"
)

// Adapter publica CertificateGenerated con el publicador genérico.
type Adapter struct {
	inner *platformevents.Publisher
}

// NewAdapter inyecta el publicador de plataforma.
func NewAdapter(inner *platformevents.Publisher) *Adapter {
	return &Adapter{inner: inner}
}

// PublishCertificateGenerated envía el evento con detail-type fijo.
func (a *Adapter) PublishCertificateGenerated(ctx context.Context, evt documents.CertificateEvent) error {
	return a.inner.Publish(ctx, "CertificateGenerated", evt)
}
