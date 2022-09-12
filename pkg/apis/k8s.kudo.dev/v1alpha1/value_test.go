package v1alpha1_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jlevesy/kudo/pkg/apis/k8s.kudo.dev/v1alpha1"
)

const rawPayload = `{"kind":"something","attribute":1,"otherAttribute":"deux"}`

type data struct {
	Attribute      int    `json:"attribute"`
	OtherAttribute string `json:"otherAttribute"`
}

func TestValueWithKind_EncodeDecode(t *testing.T) {
	var subject v1alpha1.ValueWithKind

	err := json.Unmarshal([]byte(rawPayload), &subject)
	require.NoError(t, err)

	assert.Equal(t, "something", subject.Kind)

	gotData, err := v1alpha1.DecodeValueWithKind[data](subject)
	require.NoError(t, err)

	assert.Equal(t, &data{Attribute: 1, OtherAttribute: "deux"}, gotData)

	encValue, err := v1alpha1.EncodeValueWithKind(subject.Kind, gotData)
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(encValue)
	require.NoError(t, err)

	assert.Equal(t, rawPayload, string(jsonBytes))
}

func TestValueWithKind_EncodeFailsOnArrays(t *testing.T) {
	_, err := v1alpha1.EncodeValueWithKind("wrong", []int{1, 2, 3, 4})
	require.Error(t, err)
}
