package webhooktesting

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func EncodeObject(t *testing.T, object any) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	err := json.NewEncoder(&buf).Encode(&object)
	require.NoError(t, err)

	return &buf
}
