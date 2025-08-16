package slices

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_Map(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		inFunc   func(string) int
		expected []int
	}{
		{
			name:  "string to int",
			input: []string{"1", "2", "3"},
			inFunc: func(s string) int {
				i, err := strconv.Atoi(s)
				if err != nil {
					panic(err)
				}
				return i
			},
			expected: []int{1, 2, 3},
		},
		{
			name:  "empty slice",
			input: []string{},
			inFunc: func(s string) int {
				return 0
			},
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := Map(tt.input, tt.inFunc)
			if !cmp.Equal(tt.expected, actual) {
				t.Errorf("unexpected value: %s", cmp.Diff(tt.expected, actual))
			}
		})
	}
}
