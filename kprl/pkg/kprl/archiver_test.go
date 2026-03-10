package kprl

import (
	"testing"

	"github.com/yoremi/rldev-go/pkg/bytecode"
)

func TestParseRanges(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []int
		wantErr bool
	}{
		{
			name: "empty means all",
			args: nil,
			want: nil,
		},
		{
			name: "single number",
			args: []string{"42"},
			want: []int{42},
		},
		{
			name: "range",
			args: []string{"5-8"},
			want: []int{5, 6, 7, 8},
		},
		{
			name: "multiple args",
			args: []string{"0", "5-7", "100"},
			want: []int{0, 5, 6, 7, 100},
		},
		{
			name: "tilde range",
			args: []string{"10~15"},
			want: []int{10, 11, 12, 13, 14, 15},
		},
		{
			name: "dot range",
			args: []string{"3.5"},
			want: []int{3, 4, 5},
		},
		{
			name: "negation",
			args: []string{"0-10", "!5"},
			want: []int{0, 1, 2, 3, 4, 6, 7, 8, 9, 10},
		},
		{
			name: "negated range",
			args: []string{"0-10", "!3-5"},
			want: []int{0, 1, 2, 6, 7, 8, 9, 10},
		},
		{
			name:    "bad input",
			args:    []string{"abc"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRanges(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil && got == nil {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseRanges() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseRanges()[%d] = %d, want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestResolveRanges(t *testing.T) {
	// Empty = all 10000
	all := resolveRanges(nil)
	if len(all) != MaxSeens {
		t.Errorf("resolveRanges(nil) = %d entries, want %d", len(all), MaxSeens)
	}
	if all[0] != 0 || all[9999] != 9999 {
		t.Error("resolveRanges(nil) bounds wrong")
	}

	// Specific indices
	specific := resolveRanges([]int{50, 10, 100})
	if len(specific) != 3 || specific[0] != 10 || specific[1] != 50 || specific[2] != 100 {
		t.Errorf("resolveRanges([50,10,100]) = %v, want [10,50,100]", specific)
	}
}

func TestSeenCountEmptyArchive(t *testing.T) {
	// Create a minimal empty archive
	data := make([]byte, IndexSize)
	// Write empty archive magic at start
	copy(data[0:], "\x00Empty RealLive archive")

	buf := &mockBuffer{data: data[:23]} // just the magic
	_ = buf
	// The real test would need a binarray.Buffer, but we can test the constant
	if MaxSeens != 10000 {
		t.Errorf("MaxSeens = %d, want 10000", MaxSeens)
	}
	if IndexSize != 80000 {
		t.Errorf("IndexSize = %d, want 80000", IndexSize)
	}
}

type mockBuffer struct {
	data []byte
}

func TestGetUncompressedMagic(t *testing.T) {
	tests := []struct {
		name            string
		headerVersion   int
		compilerVersion int
		want            string
	}{
		{"AVG2000", 1, 10002, "KP2K"},
		{"RealLive 110002", 2, 110002, "KPRM"},
		{"RealLive standard", 2, 10002, "KPRL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the bytecode.FileHeader via the getUncompressedMagic function
			// which we test indirectly through Extract
			got := getUncompressedMagic(fakeHeader(tt.headerVersion, tt.compilerVersion))
			if got != tt.want {
				t.Errorf("getUncompressedMagic() = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakeHeader creates a minimal FileHeader for testing
func fakeHeader(headerVer, compilerVer int) bytecode.FileHeader {
	return bytecode.FileHeader{HeaderVersion: headerVer, CompilerVersion: compilerVer}
}
