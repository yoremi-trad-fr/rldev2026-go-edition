package ast

import "testing"

func TestLocString(t *testing.T) {
	l := Loc{File: "test.org", Line: 42}
	if s := l.String(); s != "test.org:42" {
		t.Errorf("Loc.String() = %q, want 'test.org:42'", s)
	}
	l2 := Loc{Line: 10}
	if s := l2.String(); s != "line 10" {
		t.Errorf("Loc.String() no file = %q, want 'line 10'", s)
	}
}

func TestOperatorStrings(t *testing.T) {
	if OpAdd.String() != "+" { t.Error("OpAdd") }
	if OpShl.String() != "<<" { t.Error("OpShl") }
	if UnarySub.String() != "-" { t.Error("UnarySub") }
	if UnaryNot.String() != "!" { t.Error("UnaryNot") }
	if UnaryInv.String() != "~" { t.Error("UnaryInv") }
	if AssignSet.String() != "=" { t.Error("AssignSet") }
	if AssignAdd.String() != "+=" { t.Error("AssignAdd") }
	if AssignShl.String() != "<<=" { t.Error("AssignShl") }
	if CmpEqu.String() != "==" { t.Error("CmpEqu") }
	if CmpLtn.String() != "<" { t.Error("CmpLtn") }
	if ChainAnd.String() != "&&" { t.Error("ChainAnd") }
	if ChainOr.String() != "||" { t.Error("ChainOr") }
}

func TestExprInterface(t *testing.T) {
	loc := Loc{File: "test", Line: 1}

	// Verify all expression types satisfy the Expr interface
	exprs := []Expr{
		IntLit{Loc: loc, Val: 42},
		StrLit{Loc: loc},
		ResRef{Loc: loc, Key: "key"},
		StoreRef{Loc: loc},
		IntVar{Loc: loc, Bank: 0x00, Index: IntLit{Val: 0}},
		StrVar{Loc: loc, Bank: 0x12, Index: IntLit{Val: 0}},
		Deref{Loc: loc, Ident: "x", Index: IntLit{Val: 0}},
		VarOrFunc{Loc: loc, Ident: "foo"},
		BinOp{Loc: loc, LHS: IntLit{Val: 1}, Op: OpAdd, RHS: IntLit{Val: 2}},
		CmpExpr{Loc: loc, LHS: IntLit{Val: 1}, Op: CmpEqu, RHS: IntLit{Val: 1}},
		ChainExpr{Loc: loc, LHS: IntLit{Val: 1}, Op: ChainAnd, RHS: IntLit{Val: 0}},
		UnaryExpr{Loc: loc, Op: UnarySub, Val: IntLit{Val: 5}},
		ParenExpr{Loc: loc, Expr: IntLit{Val: 3}},
		FuncCall{Loc: loc, Ident: "func"},
		SelFuncCall{Loc: loc, Ident: "select"},
		ExprSeq{Loc: loc},
	}
	for _, e := range exprs {
		if e.ExprLoc() != loc {
			t.Errorf("%T.ExprLoc() != loc", e)
		}
	}
}

func TestStmtInterface(t *testing.T) {
	loc := Loc{File: "test", Line: 5}

	stmts := []Stmt{
		HaltStmt{Loc: loc},
		BreakStmt{Loc: loc},
		ContinueStmt{Loc: loc},
		LabelStmt{Loc: loc, Label: Label{Ident: "start"}},
		ReturnStmt{Loc: loc, Expr: IntLit{Val: 0}},
		AssignStmt{Loc: loc, Op: AssignSet},
		FuncCallStmt{Loc: loc, Ident: "func"},
		SelectStmt{Loc: loc, Ident: "select"},
		GotoOnStmt{Loc: loc, Ident: "goto_on"},
		GotoCaseStmt{Loc: loc, Ident: "goto_case"},
		UnknownOpStmt{Loc: loc},
		VarOrFuncStmt{Loc: loc, Ident: "x"},
		RawCodeStmt{Loc: loc},
		IfStmt{Loc: loc},
		WhileStmt{Loc: loc},
		RepeatStmt{Loc: loc},
		ForStmt{Loc: loc},
		CaseStmt{Loc: loc},
		BlockStmt{Loc: loc},
		DeclStmt{Loc: loc},
		DefineStmt{Loc: loc, Ident: "X"},
		DConstStmt{Loc: loc, Ident: "C"},
		DUndefStmt{Loc: loc},
		DSetStmt{Loc: loc},
		DTargetStmt{Loc: loc, Target: "RealLive"},
		DVersionStmt{Loc: loc},
		DirectiveStmt{Loc: loc, Name: "file"},
		LoadFileStmt{Loc: loc},
		DInlineStmt{Loc: loc},
		DForStmt{Loc: loc},
		DIfStmt{Loc: loc},
	}
	for _, s := range stmts {
		got := s.StmtLoc()
		if got != loc && got != Nowhere {
			t.Errorf("%T.StmtLoc() = %v, want loc or Nowhere", s, got)
		}
	}
	// SeqStmt and HidingStmt return Nowhere
	seq := SeqStmt{}
	if seq.StmtLoc() != Nowhere { t.Error("SeqStmt should return Nowhere") }
	hid := HidingStmt{Loc: loc}
	if hid.StmtLoc() != Nowhere { t.Error("HidingStmt should return Nowhere") }
}

func TestParamInterface(t *testing.T) {
	params := []Param{
		SimpleParam{Expr: IntLit{Val: 1}},
		ComplexParam{Exprs: []Expr{IntLit{Val: 1}, IntLit{Val: 2}}},
		SpecialParam{Tag: 0, Exprs: []Expr{IntLit{Val: 1}}},
	}
	for _, p := range params {
		_ = p // just verify it satisfies the interface
	}
}

func TestVariableName(t *testing.T) {
	tests := []struct {
		bank int
		want string
	}{
		{0x00, "intA"}, {0x01, "intB"}, {0x02, "intC"},
		{0x03, "intD"}, {0x04, "intE"}, {0x05, "intF"},
		{0x06, "intG"}, {0x19, "intZ"},
		{0x0a, "strK"}, {0x0b, "intL"}, {0x0c, "strM"}, {0x12, "strS"},
		{0x1a, "intAb"}, {0x34, "intA2b"}, {0x4e, "intA4b"}, {0x68, "intA8b"},
		{0x81, "intZ8b"},
	}
	for _, tt := range tests {
		got := VariableName(tt.bank)
		if got != tt.want {
			t.Errorf("VariableName(0x%02x) = %q, want %q", tt.bank, got, tt.want)
		}
	}
	// Unknown bank
	got := VariableName(0xFF)
	if got != "var_ff" {
		t.Errorf("VariableName(0xFF) = %q, want 'var_ff'", got)
	}
}

func TestIsConst(t *testing.T) {
	if !IsConst(IntLit{Val: 42}) { t.Error("IntLit should be const") }
	if !IsConst(StrLit{}) { t.Error("StrLit should be const") }
	if !IsConst(ParenExpr{Expr: IntLit{Val: 1}}) { t.Error("ParenExpr(IntLit) should be const") }
	if IsConst(VarOrFunc{Ident: "x"}) { t.Error("VarOrFunc should not be const") }
	if IsConst(IntVar{Bank: 0, Index: IntLit{Val: 0}}) { t.Error("IntVar should not be const") }
}

func TestIsStore(t *testing.T) {
	if !IsStore(StoreRef{}) { t.Error("StoreRef should be store") }
	if IsStore(IntLit{Val: 0}) { t.Error("IntLit should not be store") }
}

func TestTypeOf(t *testing.T) {
	if TypeOf(IntLit{Val: 1}) != TypeInt { t.Error("IntLit → TypeInt") }
	if TypeOf(StoreRef{}) != TypeInt { t.Error("StoreRef → TypeInt") }
	if TypeOf(StrLit{}) != TypeLiteral { t.Error("StrLit → TypeLiteral") }
	if TypeOf(StrVar{Bank: 0x12, Index: IntLit{Val: 0}}) != TypeStr { t.Error("StrVar → TypeStr") }
	if TypeOf(VarOrFunc{Ident: "x"}) != TypeInvalid { t.Error("VarOrFunc → TypeInvalid") }
	if TypeOf(BinOp{LHS: IntLit{Val: 1}, Op: OpAdd, RHS: IntLit{Val: 2}}) != TypeInt {
		t.Error("BinOp(int) → TypeInt")
	}
	if TypeOf(ParenExpr{Expr: StrLit{}}) != TypeLiteral { t.Error("Parens(StrLit) → TypeLiteral") }
}

func TestStrTokenInterface(t *testing.T) {
	tokens := []StrToken{
		TextToken{Text: "hello"},
		SpaceToken{Count: 1},
		DQuoteToken{},
		RCurToken{},
		LLenticToken{},
		RLenticToken{},
		AsteriskToken{},
		PercentToken{},
		HyphenToken{},
		SpeakerToken{},
		DeleteToken{},
		NameToken{Global: false, Index: IntLit{Val: 0}},
		GlossToken{IsRuby: true},
		CodeToken{Ident: "c"},
		AddToken{Key: "key"},
		ResRefToken{Key: "res"},
		RewriteToken{Key: 0},
	}
	for _, tok := range tokens {
		_ = tok // verify interface
	}
	if len(tokens) != 17 {
		t.Errorf("expected 17 str token types, got %d", len(tokens))
	}
}

func TestDeclDirString(t *testing.T) {
	if DeclDirString(DirZero) != "zero" { t.Error("DirZero") }
	if DeclDirString(DirBlock) != "block" { t.Error("DirBlock") }
	if DeclDirString(DirExt) != "ext" { t.Error("DirExt") }
	if DeclDirString(DirLabel) != "labelled" { t.Error("DirLabel") }
}
