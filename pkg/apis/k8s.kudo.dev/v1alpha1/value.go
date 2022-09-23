package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
)

type ValueWithKind struct {
	Kind string `json:"kind"`

	payload []byte `json:"-"`
}

func (a *ValueWithKind) UnmarshalJSON(p []byte) error {
	var k struct {
		Kind string `json:"kind"`
	}

	if err := json.Unmarshal(p, &k); err != nil {
		return err
	}

	a.Kind = k.Kind
	a.payload = p

	return nil
}

func (a ValueWithKind) MarshalJSON() ([]byte, error) {
	return a.payload, nil
}

func DecodeValueWithKind[V any](v ValueWithKind) (*V, error) {
	var parsed V

	return &parsed, json.Unmarshal(v.payload, &parsed)
}

func MustEncodeValueWithKind(kind string, value any) ValueWithKind {
	v, err := EncodeValueWithKind(kind, value)
	if err != nil {
		panic(err)
	}

	return v
}

func EncodeValueWithKind(kind string, value any) (ValueWithKind, error) {
	p, err := json.Marshal(value)
	if err != nil {
		return ValueWithKind{}, err
	}

	if len(p) == 0 {
		return ValueWithKind{}, errors.New("unexpected marshal of an empty value")
	}

	if p[0] == '[' {
		return ValueWithKind{}, errors.New("encoding of arrays isn't supported")
	}

	// I'm going to regret this.
	if bytes.Equal(p, []byte("{}")) {
		p = []byte(`{"kind":"` + kind + `"}`)
	} else {
		p = append([]byte(`{"kind":"`+kind+`",`), p[1:]...)
	}

	return ValueWithKind{
		Kind:    kind,
		payload: p,
	}, nil
}
