package granter_test

import (
	"testing"

	"github.com/jlevesy/kudo/granter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticFactory_Get(t *testing.T) {
	factory := granter.StaticFactory{
		"Test": func() (granter.Granter, error) { return fakeGranter{}, nil },
	}

	_, err := factory.Get("BadKind")
	require.Error(t, err)

	granter, err := factory.Get("Test")
	assert.NoError(t, err)
	assert.Equal(t, fakeGranter{}, granter)
}

type fakeGranter struct {
	granter.Granter
}
