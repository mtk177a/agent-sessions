package provider

import "testing"

func TestSemverAtLeast(t *testing.T) {
	for _, tc := range []struct {
		name, value, floor string
		want               bool
	}{
		{name: "equal", value: "2.1.260", floor: "2.1.260", want: true},
		{name: "future_patch", value: "2.1.270", floor: "2.1.260", want: true},
		{name: "future_major", value: "9.0.0", floor: "2.1.260", want: true},
		{name: "prerelease_after_floor", value: "2.2.0-alpha.1", floor: "2.1.260", want: true},
		{name: "prerelease_before_release", value: "2.1.260-alpha.1", floor: "2.1.260"},
		{name: "older", value: "2.1.259", floor: "2.1.260"},
		{name: "build_metadata", value: "2.1.260+build.4", floor: "2.1.260", want: true},
		{name: "large_core_number", value: "184467440737095516160.0.0", floor: "9.0.0", want: true},
		{name: "large_prerelease_number", value: "2.1.261-alpha.184467440737095516160", floor: "2.1.261-alpha.9", want: true},
		{name: "invalid", value: "latest", floor: "2.1.260"},
		{name: "leading_zero", value: "2.1.0260", floor: "2.1.260"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SemverAtLeast(tc.value, tc.floor); got != tc.want {
				t.Fatalf("SemverAtLeast(%q, %q) = %v, want %v", tc.value, tc.floor, got, tc.want)
			}
		})
	}
}
