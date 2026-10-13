package profilevalidation

import (
	"context"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
	"github.com/heihei0299/pi-switch/internal/profile"
)

type Validator struct{}

func (Validator) Validate(ctx context.Context, prof provider.Profile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return profile.ValidateProfile(prof)
}
