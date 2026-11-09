package authz

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mayer-04/kenko-profile-go/internal/documents"
)

// ListAuthorizations lista las autorizaciones del usuario (más recientes primero).
type ListAuthorizations struct {
	reader AuthorizationReader
}

// NewListAuthorizations inyecta el lector.
func NewListAuthorizations(reader AuthorizationReader) *ListAuthorizations {
	return &ListAuthorizations{reader: reader}
}

// Execute devuelve las autorizaciones del userId del token.
func (u *ListAuthorizations) Execute(ctx context.Context, userID string) ([]AuthorizationWithAppointment, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: userId", documents.ErrStorage)
	}
	items, err := u.reader.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []AuthorizationWithAppointment{}, nil
	}
	return items, nil
}

// GetAuthorizationPDF genera el PDF de una autorización del usuario.
type GetAuthorizationPDF struct {
	profiles documents.ProfileReader
	reader   AuthorizationReader
	pdf      PDFGenerator
}

// NewGetAuthorizationPDF inyecta dependencias por constructor.
func NewGetAuthorizationPDF(profiles documents.ProfileReader, reader AuthorizationReader, pdf PDFGenerator) *GetAuthorizationPDF {
	return &GetAuthorizationPDF{profiles: profiles, reader: reader, pdf: pdf}
}

// Result trae los bytes del PDF y el nombre sugerido.
type Result struct {
	Bytes    []byte
	FileName string
}

// Execute obtiene la autorización (solo del propio usuario), el perfil y genera el PDF.
func (u *GetAuthorizationPDF) Execute(ctx context.Context, userID, authorizationID string) (Result, error) {
	userID = strings.TrimSpace(userID)
	authorizationID = strings.TrimSpace(authorizationID)
	if userID == "" || authorizationID == "" {
		return Result{}, fmt.Errorf("%w", documents.ErrAuthorizationNotFound)
	}
	item, err := u.reader.GetByID(ctx, userID, authorizationID)
	if err != nil {
		return Result{}, err
	}
	snapshot, err := u.profiles.GetProfile(ctx, userID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", documents.ErrProfileNotFound, err)
	}
	data, err := u.pdf.GenerateAuthorization(snapshot, item)
	if err != nil {
		return Result{}, fmt.Errorf("%w: genera pdf: %v", documents.ErrStorage, err)
	}
	short := item.Authorization.ID
	if i := strings.Index(short, "-"); i > 0 {
		short = short[:i]
	} else if len(short) > 8 {
		short = short[:8]
	}
	fileName := fmt.Sprintf("autorizacion-%s-%s.pdf", item.Authorization.Type.Slug(), short)
	return Result{Bytes: data, FileName: fileName}, nil
}
