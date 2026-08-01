package storage

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"videomesh/backend/internal/domain"
)

// Service manages storage volumes (ADR-011 / ADR-014): CRUD plus runtime
// availability probing. Volume remapping by device_id and scanning arrive in
// later milestones.
type Service struct {
	repo         domain.StorageRepo
	probeTimeout time.Duration
}

// New builds a storage volume service.
func New(repo domain.StorageRepo) *Service {
	return &Service{repo: repo, probeTimeout: 2 * time.Second}
}

// List returns all volumes with live availability.
func (s *Service) List(ctx context.Context) ([]domain.Storage, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Available = s.probe(ctx, &list[i])
	}
	return list, nil
}

// Get returns one volume with live availability.
func (s *Service) Get(ctx context.Context, id string) (domain.Storage, error) {
	st, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Storage{}, err
	}
	st.Available = s.probe(ctx, &st)
	return st, nil
}

// Create validates and persists a new volume.
func (s *Service) Create(ctx context.Context, in domain.Storage) (domain.Storage, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.RootPath) == "" {
		return domain.Storage{}, domain.ErrInvalid
	}
	if err := validateType(in.Type); err != nil {
		return domain.Storage{}, err
	}
	if in.Type == domain.StorageTypeExternal && strings.TrimSpace(in.DeviceID) == "" {
		return domain.Storage{}, domain.ErrInvalid
	}
	in.ID = ulid.Make().String()
	in.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	in.Available = s.probe(ctx, &in)
	if err := s.repo.Create(ctx, in); err != nil {
		return domain.Storage{}, err
	}
	return in, nil
}

// Patch carries the user-editable fields of a volume.
type Patch struct {
	Name     *string
	Type     *domain.StorageType
	Readonly *bool
	Enabled  *bool
}

// Update applies a patch to an existing volume.
func (s *Service) Update(ctx context.Context, id string, p Patch) (domain.Storage, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Storage{}, err
	}
	if p.Name != nil {
		existing.Name = strings.TrimSpace(*p.Name)
	}
	if p.Type != nil {
		existing.Type = *p.Type
	}
	if p.Readonly != nil {
		existing.Readonly = *p.Readonly
	}
	if p.Enabled != nil {
		existing.Enabled = *p.Enabled
	}
	if existing.Name == "" {
		return domain.Storage{}, domain.ErrInvalid
	}
	if err := validateType(existing.Type); err != nil {
		return domain.Storage{}, err
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		return domain.Storage{}, err
	}
	return existing, nil
}

// Refresh re-probes availability for one volume.
func (s *Service) Refresh(ctx context.Context, id string) (domain.Storage, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Storage{}, err
	}
	existing.Available = s.probe(ctx, &existing)
	return existing, nil
}

// Delete removes the volume configuration. Disk files are never touched
// (ADR-014); video metadata retention arrives with the scanner milestone.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func validateType(t domain.StorageType) error {
	switch t {
	case domain.StorageTypeInternal, domain.StorageTypeExternal, domain.StorageTypeNetwork:
		return nil
	}
	return domain.ErrInvalid
}

// probe reports whether the volume root is currently reachable. A short
// timeout keeps a dead network share from blocking requests.
func (s *Service) probe(_ context.Context, st *domain.Storage) bool {
	done := make(chan struct{})
	var ok bool
	go func() {
		_, err := os.Stat(st.RootPath)
		ok = err == nil
		close(done)
	}()
	select {
	case <-done:
		return ok
	case <-time.After(s.probeTimeout):
		return false
	}
}
