package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseArges(t *testing.T) {
	testCases := []struct {
		desc     string
		rawArgs  []string
		wantArgs escalateArgs
		wantErr  error
	}{
		{
			desc:    "raises an error if not enough args",
			rawArgs: []string{"not-enough"},
			wantErr: errors.New("you need to provide a policy name and a reason"),
		},
		{
			desc:    "raises an error if policy name is blank",
			rawArgs: []string{"    ", "not-enough"},
			wantErr: errors.New("you need to provide a non blank policy name"),
		},
		{
			desc:    "raises an error if reason is blank",
			rawArgs: []string{"policyyyy", " "},
			wantErr: errors.New("you need to provide a non blank reason"),
		},
		{
			desc:    "parse args",
			rawArgs: []string{"policy", "reason"},
			wantArgs: escalateArgs{
				policyName: "policy",
				reason:     "reason",
			},
		},
		{
			desc:    "concat args into reaso",
			rawArgs: []string{"policy", "reason", "with", "more", "elaboration"},
			wantArgs: escalateArgs{
				policyName: "policy",
				reason:     "reason with more elaboration",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			gotArgs, err := parseArgs(testCase.rawArgs)
			assert.Equal(t, testCase.wantErr, err)
			assert.Equal(t, testCase.wantArgs, gotArgs)
		})
	}
}
