package main

import (
	"testing"
)

func TestConstructReplacementLine(t *testing.T) {
	testCases := []struct {
		name          string
		oldLine       string
		location		  []int
		paramValue    string
		expected      string
	}{
		{
			name: "Top-level standalone parameter",
			oldLine: "test-values-key: {{ssm /configmgmt/my/parameter}}",
			location: []int{17, 49},
			paramValue: "test-param-value",
			expected:    "test-values-key: test-param-value\n",
		},
		{
			name: "Nested standalone parameter",
			oldLine: "    test-values-key: {{ssm /configmgmt/my/parameter}}",
			location: []int{21, 53},
			paramValue: "test-param-value-2",
			expected:    "    test-values-key: test-param-value-2\n",
		},
		{
			name: "List item path parameter",
			oldLine: "- {{ssm-path /configmgmt/path/to/my/parameter}}",
			location: []int{2, 47},
			paramValue: "{param1: testtttt, param2: hello}",
			expected:    "- {param1: testtttt, param2: hello}\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(subT *testing.T) {
			result := constructReplacementLine(tc.oldLine, tc.location, tc.paramValue)
			if result != tc.expected {
				subT.Errorf("expected line %s, got %s\n", tc.expected, result)
			}
		})
	}
}
