package disasm

import "testing"

func TestPrettifyCondLogicalBranches(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"intG[43] == 0 && intG[45] == 0", "!intG[43] && !intG[45]"},
		{"intF[1013] == 0 || intF[1013] == 2", "!intF[1013] || intF[1013] == 2"},
		{"(intG[1] == 0 && intG[2] == 0) || intG[3] != 0", "(intG[1] == 0 && intG[2] == 0) || intG[3]"},
		{"intA[3] - intA[0] == 0", "!(intA[3] - intA[0])"},
		{"(intA[3] - intA[0]) % 2 == 0", "!((intA[3] - intA[0]) % 2)"},
		{"intA[intB[0] + 1] == 0", "!intA[intB[0] + 1]"},
	}
	for _, tt := range tests {
		if got := prettifyCond(tt.in); got != tt.want {
			t.Fatalf("prettifyCond(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
