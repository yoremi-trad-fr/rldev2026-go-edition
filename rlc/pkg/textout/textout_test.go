package textout

import (
	"testing"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

func sp(e ast.Expr) ast.Param { return ast.SimpleParam{Expr: e} }
func ilit(v int32) ast.Expr   { return ast.IntLit{Val: v} }

// ============================================================
// Token encoding
// ============================================================

func TestMakeToken(t *testing.T) {
	// MakeToken(8, 0, 0x0d) = (0 << 12) | 8 | (0x0d << 4)
	// = 0 | 8 | 208 = 216
	e := MakeToken(IDCCode, 0, CCReturn)
	// Should produce a BinOp tree
	if _, ok := e.(ast.BinOp); !ok {
		t.Fatalf("MakeToken: got %T", e)
	}
}

func TestMakeTokenExpr(t *testing.T) {
	e := MakeTokenExpr(IDVrble, ilit(5), ilit(0x12))
	if _, ok := e.(ast.BinOp); !ok {
		t.Fatalf("MakeTokenExpr: got %T", e)
	}
}

func TestTokenIDConstants(t *testing.T) {
	// Verify token IDs are in range [0, 15]
	ids := []int32{IDSText, IDNText, IDDText, IDDQuot, IDSpace, IDNameV,
		IDFSize, IDVrble, IDCCode, IDWName, IDRuby, IDEmoji, IDMove}
	for _, id := range ids {
		if id < 0 || id >= 16 {
			t.Errorf("token ID %d out of range [0,15]", id)
		}
	}
}

// ============================================================
// Control code processing
// ============================================================

func TestProcessCodeReturn(t *testing.T) {
	r, err := ProcessControlCode("r", nil, nil)
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Fatalf("\\r: got %d tokens", len(r.Tokens)) }
	if r.Text != "" { t.Error("\\r should not produce text") }
}

func TestProcessCodeNewline(t *testing.T) {
	r, err := ProcessControlCode("n", nil, nil)
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Fatalf("\\n: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodePage(t *testing.T) {
	r, err := ProcessControlCode("p", nil, nil)
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Fatalf("\\p: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeIntConst(t *testing.T) {
	// \i{42} with constant → should produce text "42"
	r, err := ProcessControlCode("i", nil, []ast.Param{sp(ilit(42))})
	if err != nil { t.Fatal(err) }
	if r.Text != "42" { t.Errorf("\\i{42}: text=%q, want '42'", r.Text) }
}

func TestProcessCodeIntNeg(t *testing.T) {
	r, err := ProcessControlCode("i", nil, []ast.Param{sp(ilit(-7))})
	if err != nil { t.Fatal(err) }
	if r.Text != "-7" { t.Errorf("\\i{-7}: text=%q", r.Text) }
}

func TestProcessCodeIntVar(t *testing.T) {
	// \i{var} with non-constant → should produce 2 tokens
	v := ast.IntVar{Bank: 0, Index: ilit(0)}
	r, err := ProcessControlCode("i", nil, []ast.Param{sp(v)})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 2 { t.Errorf("\\i{var}: got %d tokens, want 2", len(r.Tokens)) }
}

func TestProcessCodeIntWithWidth(t *testing.T) {
	// \i{var} with width specifier
	v := ast.IntVar{Bank: 0, Index: ilit(0)}
	r, err := ProcessControlCode("i", ilit(5), []ast.Param{sp(v)})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 2 { t.Errorf("\\i{5}{var}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeStrVar(t *testing.T) {
	sv := ast.StrVar{Bank: 0x12, Index: ilit(3)}
	r, err := ProcessControlCode("s", nil, []ast.Param{sp(sv)})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Fatalf("\\s{}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeStrVarError(t *testing.T) {
	// \s{} with non-string param → error
	_, err := ProcessControlCode("s", nil, []ast.Param{sp(ilit(5))})
	if err == nil { t.Error("expected error for \\s{int}") }
}

func TestProcessCodeColor0(t *testing.T) {
	// \c{} with no params
	r, err := ProcessControlCode("c", nil, nil)
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\c{}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeColor1(t *testing.T) {
	// \c{fg}
	r, err := ProcessControlCode("c", nil, []ast.Param{sp(ilit(128))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\c{fg}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeColor2(t *testing.T) {
	// \c{fg, bg}
	r, err := ProcessControlCode("c", nil, []ast.Param{sp(ilit(128)), sp(ilit(64))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\c{fg,bg}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeSize(t *testing.T) {
	// \size{24}
	r, err := ProcessControlCode("size", nil, []ast.Param{sp(ilit(24))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\size: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeSizeReset(t *testing.T) {
	// \size{} (no params = reset)
	r, err := ProcessControlCode("size", nil, nil)
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\size{}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeWait(t *testing.T) {
	r, err := ProcessControlCode("wait", nil, []ast.Param{sp(ilit(1000))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\wait: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeWaitError(t *testing.T) {
	_, err := ProcessControlCode("wait", nil, nil)
	if err == nil { t.Error("expected error for \\wait without params") }
}

func TestProcessCodeEmoji(t *testing.T) {
	// \e{5}
	r, err := ProcessControlCode("e", nil, []ast.Param{sp(ilit(5))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\e{}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeEmojiMono(t *testing.T) {
	// \em{5}
	r, err := ProcessControlCode("em", nil, []ast.Param{sp(ilit(5))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 1 { t.Errorf("\\em{}: got %d tokens", len(r.Tokens)) }
}

func TestProcessCodeEmojiWithSize(t *testing.T) {
	// \e{5, 32} → size_on, emoji, size_off = 3 tokens
	r, err := ProcessControlCode("e", nil, []ast.Param{sp(ilit(5)), sp(ilit(32))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 3 { t.Errorf("\\e{5,32}: got %d tokens, want 3", len(r.Tokens)) }
}

func TestProcessCodeMv(t *testing.T) {
	// \mv{x, y} → 3 values (token + x + y)
	r, err := ProcessControlCode("mv", nil, []ast.Param{sp(ilit(100)), sp(ilit(200))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 3 { t.Errorf("\\mv: got %d tokens, want 3", len(r.Tokens)) }
}

func TestProcessCodeMvx(t *testing.T) {
	// \mvx{x} → 2 values
	r, err := ProcessControlCode("mvx", nil, []ast.Param{sp(ilit(100))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 2 { t.Errorf("\\mvx: got %d tokens, want 2", len(r.Tokens)) }
}

func TestProcessCodeMvy(t *testing.T) {
	r, err := ProcessControlCode("mvy", nil, []ast.Param{sp(ilit(100))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 2 { t.Errorf("\\mvy: got %d tokens, want 2", len(r.Tokens)) }
}

func TestProcessCodePos(t *testing.T) {
	r, err := ProcessControlCode("pos", nil, []ast.Param{sp(ilit(10)), sp(ilit(20))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 3 { t.Errorf("\\pos: got %d tokens, want 3", len(r.Tokens)) }
}

func TestProcessCodePosX(t *testing.T) {
	r, err := ProcessControlCode("posx", nil, []ast.Param{sp(ilit(10))})
	if err != nil { t.Fatal(err) }
	if len(r.Tokens) != 2 { t.Errorf("\\posx: got %d tokens, want 2", len(r.Tokens)) }
}

func TestProcessCodeMvNoParams(t *testing.T) {
	_, err := ProcessControlCode("mv", nil, nil)
	if err == nil { t.Error("expected error for \\mv without params") }
}

func TestProcessCodeUnknown(t *testing.T) {
	_, err := ProcessControlCode("xyz", nil, nil)
	if err == nil { t.Error("expected error for unknown code") }
}

// ============================================================
// Helpers
// ============================================================

func TestInt32ToString(t *testing.T) {
	tests := []struct{ v int32; want string }{
		{0, "0"}, {42, "42"}, {-7, "-7"}, {1000000, "1000000"},
		{-1, "-1"}, {2147483647, "2147483647"},
	}
	for _, tt := range tests {
		if got := int32ToString(tt.v); got != tt.want {
			t.Errorf("int32ToString(%d) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestOneSimpleParam(t *testing.T) {
	// With one simple param
	e := oneSimpleParam([]ast.Param{sp(ilit(5))})
	if e == nil { t.Fatal("nil") }
	if lit, ok := e.(ast.IntLit); !ok || lit.Val != 5 {
		t.Errorf("got %v", e)
	}
	// With wrong count
	if oneSimpleParam(nil) != nil { t.Error("nil params should return nil") }
	if oneSimpleParam([]ast.Param{sp(ilit(1)), sp(ilit(2))}) != nil { t.Error("2 params should return nil") }
}

func TestPauseTypeConstants(t *testing.T) {
	if PauseNone != 0 { t.Error("PauseNone") }
	if PausePause != 1 { t.Error("PausePause") }
	if PausePage != 2 { t.Error("PausePage") }
}

func TestSJISConstants(t *testing.T) {
	if SJISLeftBracket != "\x81\x79" { t.Error("left bracket") }
	if SJISRightBracket != "\x81\x7a" { t.Error("right bracket") }
}
