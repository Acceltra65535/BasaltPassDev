package utils

import (
	"math"
	"testing"
)

func TestParsePositiveUint(t *testing.T) {
	got, err := ParsePositiveUint(" 42 ")
	if err != nil {
		t.Fatalf("ParsePositiveUint returned error: %v", err)
	}
	if got != 42 {
		t.Fatalf("ParsePositiveUint = %d, want 42", got)
	}

	if _, err := ParsePositiveUint("0"); err == nil {
		t.Fatal("ParsePositiveUint accepted zero")
	}
}

func TestPositiveUintFromAnyRejectsUnsafeValues(t *testing.T) {
	cases := []interface{}{
		-1,
		int64(-1),
		1.2,
		math.NaN(),
		math.Inf(1),
		float64(maxSafeIntegerFloat64 + 1),
	}

	for _, tc := range cases {
		if _, err := PositiveUintFromAny(tc); err == nil {
			t.Fatalf("PositiveUintFromAny(%v) accepted unsafe value", tc)
		}
	}
}

func TestUint64ToUintRejectsOverflow(t *testing.T) {
	maxUint := uint64(^uint(0))
	if _, err := Uint64ToUint(maxUint); err != nil {
		t.Fatalf("Uint64ToUint rejected max uint: %v", err)
	}
	if maxUint < math.MaxUint64 {
		if _, err := Uint64ToUint(maxUint + 1); err == nil {
			t.Fatal("Uint64ToUint accepted overflow")
		}
	}
}
