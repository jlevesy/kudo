package granter

import (
	"fmt"
)

// Factory provides a way to retrieve a granter based on a given granter kind.
type Factory interface {
	Get(string) (Granter, error)
}

// Static factory is an implementation based on a map.
// Adding support for a grant kind when a StaticFactory is in use in unsafe.
type StaticFactory map[string]func() (Granter, error)

// Get retrieves a granter based on its kind.
func (f StaticFactory) Get(grantKind string) (Granter, error) {
	granterInitFunc, ok := f[grantKind]
	if !ok {
		return nil, fmt.Errorf("unknown kind %s", grantKind)
	}

	return granterInitFunc()
}
