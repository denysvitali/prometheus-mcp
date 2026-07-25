package server

import "testing"

func TestBoundedLimit(t *testing.T) {
	tests := map[string]struct {
		in   *int
		def  int
		want int
	}{
		"absent uses default":   {in: nil, def: 42, want: 42},
		"negative uses default": {in: intPtr(-7), def: 42, want: 42},
		"zero means unlimited":  {in: intPtr(0), def: 42, want: 0},
		"explicit value wins":   {in: intPtr(5), def: 42, want: 5},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := boundedLimit(tc.in, tc.def); got != tc.want {
				t.Errorf("boundedLimit(%v, %d) = %d, want %d", tc.in, tc.def, got, tc.want)
			}
		})
	}
}
