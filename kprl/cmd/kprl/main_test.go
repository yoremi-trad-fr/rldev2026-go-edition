package main

import (
	"testing"

	"github.com/yoremi/rldev-go/pkg/disasm"
)

func TestParseFileVersionString(t *testing.T) {
	tests := []struct {
		in   string
		want disasm.Version
		ok   bool
	}{
		{"1.6.3.4", disasm.Version{1, 6, 3, 4}, true},
		{"1, 6, 3, 4", disasm.Version{1, 6, 3, 4}, true},
		{"File version 1.6.3.4", disasm.Version{1, 6, 3, 4}, true},
		{"RealLive 1.2.3.5", disasm.Version{1, 2, 3, 5}, true},
		{"040904b0", disasm.Version{}, false},
		{"2010", disasm.Version{}, false},
		{"RealLive", disasm.Version{}, false},
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

func TestDisasmVersionExplicitStringWins(t *testing.T) {
	got := disasmVersionForTarget(disasm.ModeRealLive, "1.4.4.8")
	if got != (disasm.Version{1, 4, 4, 8}) {
		t.Fatalf("disasmVersionForTarget() = %v, want 1.4.4.8", got)
	}
}
