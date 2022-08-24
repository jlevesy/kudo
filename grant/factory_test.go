package grant_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jlevesy/kudo/grant"
)

func TestStaticFactory_Get(t *testing.T) {
	factory := grant.StaticFactory{
		"Test": func() (grant.Granter, error) { return fakeGranter{}, nil },
	}

	_, err := factory.Get("BadKind")
	require.Error(t, err)

	granter, err := factory.Get("Test")
	assert.NoError(t, err)
	assert.Equal(t, fakeGranter{}, granter)
}

type fakeGranter struct {
	grant.Granter
}
