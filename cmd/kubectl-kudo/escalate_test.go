package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEscalateArgs(t *testing.T) {
	testCases := []struct {
		desc     string
		rawArgs  []string
		wantArgs escalateArgs
		wantErr  error
	}{
		{
			desc:    "raises an error if not enough args",
			rawArgs: []string{},
			wantErr: errors.New("you need to provide a policy name"),
		},
		{
			desc:    "raises an error if policy name is blank",
			rawArgs: []string{"    ", "not-enough"},
			wantErr: errors.New("you need to provide a non blank policy name"),
		},
		{
			desc:    "parse args",
			rawArgs: []string{"policy"},
			wantArgs: escalateArgs{
				policyName: "policy",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			gotArgs, err := parseEscalateArgs(testCase.rawArgs)
			assert.Equal(t, testCase.wantErr, err)
			assert.Equal(t, testCase.wantArgs, gotArgs)
		})
	}
}
