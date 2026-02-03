package cv

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
)

// ContentPayload represents the persisted CV content and metadata.
type ContentPayload struct {
	Content   string     `json:"content"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	IsDefault bool       `json:"is_default"`
}

// ContentRepository defines the storage behavior for CV content.
//
// Load returns the persisted content with metadata.
// Save persists the content and returns updated metadata.
type ContentRepository interface {
	Load(ctx context.Context) (ContentPayload, error)
	Save(ctx context.Context, content string) (ContentPayload, error)
}

// CompositeContentStore coordinates multiple content repositories.
//
// Primary is used first for reads and writes; secondary is used as fallback for reads
// and as an additional write target when saving.
type CompositeContentStore struct {
	primary   ContentRepository
	secondary ContentRepository
}

// NewCompositeContentStore creates a CompositeContentStore from primary and secondary repositories.
// It returns an error if both repositories are nil.
func NewCompositeContentStore(primary ContentRepository, secondary ContentRepository) (*CompositeContentStore, error) {
	if primary == nil && secondary == nil {
		return nil, errors.WithStack(errors.New("no content repositories configured"))
	}

	return &CompositeContentStore{
		primary:   primary,
		secondary: secondary,
	}, nil
}

// Load fetches CV content from the primary repository, falling back to secondary when needed.
func (s *CompositeContentStore) Load(ctx context.Context) (ContentPayload, error) {
	if s.primary != nil {
		payload, err := s.primary.Load(ctx)
		if err != nil {
			return ContentPayload{}, errors.Wrap(err, "load primary content")
		}
		if !payload.IsDefault {
			return payload, nil
		}
	}

	if s.secondary != nil {
		payload, err := s.secondary.Load(ctx)
		if err != nil {
			return ContentPayload{}, errors.Wrap(err, "load secondary content")
		}
		return payload, nil
	}

	return ContentPayload{}, errors.WithStack(errors.New("no content repository available"))
}

// Save persists CV content to the primary repository and mirrors it to the secondary when present.
func (s *CompositeContentStore) Save(ctx context.Context, content string) (ContentPayload, error) {
	var payload ContentPayload
	var saved bool
	if s.primary != nil {
		var err error
		payload, err = s.primary.Save(ctx, content)
		if err != nil {
			return ContentPayload{}, errors.Wrap(err, "save primary content")
		}
		saved = true
	}

	if s.secondary != nil {
		secondaryPayload, err := s.secondary.Save(ctx, content)
		if err != nil {
			return ContentPayload{}, errors.Wrap(err, "save secondary content")
		}
		if !saved {
			payload = secondaryPayload
			saved = true
		}
	}

	if saved {
		return payload, nil
	}

	return ContentPayload{}, errors.WithStack(errors.New("no content repository available"))
}
