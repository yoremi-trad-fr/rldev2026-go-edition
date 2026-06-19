package main

import "testing"

func TestParseFileVersionString(t *testing.T) {
	tests := []struct {
		in   string
		ok   bool
		want [4]int
	}{
		{in: "1, 2, 3, 5", ok: true, want: [4]int{1, 2, 3, 5}},
		{in: "1.2.7.0", ok: true, want: [4]int{1, 2, 7, 0}},
		{in: "RealLive 1.2.3.5", ok: true, want: [4]int{1, 2, 3, 5}},
		{in: "RealLive", ok: false},
	}
	for _, tt := range tests {
		got, ok := parseFileVersionString(tt.in)
		if ok != tt.ok {
			t.Fatalf("parseFileVersionString(%q) ok=%v, want %v", tt.in, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("parseFileVersionString(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
