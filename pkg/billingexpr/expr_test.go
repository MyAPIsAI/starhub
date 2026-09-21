package billingexpr

import (
	"math"
	"strings"
	"testing"
)

func TestEvalArithmeticAndPrecedence(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"1 + 2 * 3", 7},
		{"(1 + 2) * 3", 9},
		{"10 / 4", 2.5},
		{"10 % 3", 1},
		{"-3 + 10", 7},
		{"2 * -3", -6},
		{"1.5 * 2", 3},
		{".5 + .5", 1},
	}
	for _, c := range cases {
		got, err := Eval(c.expr, nil)
		if err != nil {
			t.Fatalf("Eval(%q) error: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestEvalTernaryTieredPricing(t *testing.T) {
	expr := "completion_tokens <= 1000 ? 0.001 : completion_tokens <= 10000 ? 0.01 : 0.05"
	cases := []struct {
		completion float64
		want       float64
	}{
		{1000, 0.001},
		{0, 0.001},
		{1001, 0.01},
		{10000, 0.01},
		{99999, 0.05},
	}
	for _, c := range cases {
		got, err := Eval(expr, map[string]float64{"completion_tokens": c.completion})
		if err != nil {
			t.Fatalf("Eval tiered(%v) error: %v", c.completion, err)
		}
		if got != c.want {
			t.Errorf("tiered price for %v tokens = %v, want %v", c.completion, got, c.want)
		}
	}
}

func TestEvalLogicAndComparison(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"1 < 2", 1},
		{"2 <= 1", 0},
		{"3 == 3", 1},
		{"3 != 3", 0},
		{"1 < 2 && 2 < 3", 1},
		{"1 > 2 || 2 < 3", 1},
		{"!0", 1},
		{"!5", 0},
		{"true && false", 0},
		{"(1 > 0) ? 10 : 20", 10},
	}
	for _, c := range cases {
		got, err := Eval(c.expr, nil)
		if err != nil {
			t.Fatalf("Eval(%q) error: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestEvalFunctions(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"min(3, 1, 2)", 1},
		{"max(3, 1, 2)", 3},
		{"floor(2.7)", 2},
		{"ceil(2.1)", 3},
		{"round(2.5)", 3},
		{"abs(0 - 5)", 5},
		{"min(max(0, 5), 10)", 5},
	}
	for _, c := range cases {
		got, err := Eval(c.expr, nil)
		if err != nil {
			t.Fatalf("Eval(%q) error: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("Eval(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestEvalVariables(t *testing.T) {
	vars := map[string]float64{"prompt_tokens": 100, "completion_tokens": 50}
	got, err := Eval("prompt_tokens + completion_tokens", vars)
	if err != nil {
		t.Fatalf("Eval error: %v", err)
	}
	if got != 150 {
		t.Fatalf("vars eval = %v, want 150", got)
	}
}

func TestEvalErrors(t *testing.T) {
	bad := []string{
		"",            // empty
		"1 +",         // dangling operator
		"(1 + 2",      // unbalanced paren
		"1 ? 2",       // incomplete ternary
		"unknown_var", // unknown variable
		"1 / 0",       // division by zero
		"5 % 0",       // modulo by zero
		"hack(1)",     // non-whitelisted function
		"min()",       // min needs arguments
		"floor(1, 2)", // floor arity
		"1 @ 2",       // bad character
		"1 2",         // trailing input
	}
	for _, expr := range bad {
		if _, err := Eval(expr, nil); err == nil {
			t.Errorf("Eval(%q) should fail", expr)
		}
	}
}

func TestValidateMatchesEval(t *testing.T) {
	good := []string{"1 + 2", "a ? b : c", "min(x, 10) * 0.5"}
	for _, expr := range good {
		if err := Validate(expr); err != nil {
			t.Errorf("Validate(%q) failed: %v", expr, err)
		}
	}
	if err := Validate("bad("); err == nil {
		t.Fatal("Validate should reject broken syntax")
	}
}

func TestMaxLengthLimit(t *testing.T) {
	long := strings.Repeat("1+", 300) + "1" // 601 chars, valid otherwise
	if len(long) <= MaxExprLength {
		t.Fatalf("test expression should exceed MaxExprLength, got %d", len(long))
	}
	if err := Validate(long); err == nil {
		t.Fatal("over-length expression must be rejected")
	}
}

func TestMaxDepthLimit(t *testing.T) {
	deep := strings.Repeat("(", 100) + "1" + strings.Repeat(")", 100)
	if err := Validate(deep); err == nil {
		t.Fatal("deeply nested expression must be rejected")
	}
	okDeep := strings.Repeat("(", 20) + "1" + strings.Repeat(")", 20)
	if err := Validate(okDeep); err != nil {
		t.Fatalf("20 levels of nesting should be fine: %v", err)
	}
}

func TestStepLimit(t *testing.T) {
	// A <=512-char expression can never exceed MaxExprSteps (every node needs
	// at least one character), but the guard protects against future grammar
	// changes — verify it directly with a synthetic tree.
	root := &node{kind: ndNumber, num: 1}
	for i := 0; i < MaxExprSteps; i++ {
		root = &node{kind: ndBinary, op: "+",
			args: []*node{root, {kind: ndNumber, num: 1}}}
	}
	ev := &evaluator{vars: map[string]float64{}}
	if _, err := ev.eval(root); err == nil {
		t.Fatal("step-bomb tree should error")
	}

	flat := strings.Repeat("1 + ", 100) + "1" // well under both limits
	if _, err := Eval(flat, nil); err != nil {
		t.Fatalf("reasonable expression should evaluate: %v", err)
	}
}

func TestDeterministicAndPure(t *testing.T) {
	vars := map[string]float64{"total_tokens": 1234}
	expr := "max(0.001, 0.002 - 0.0000005 * total_tokens)"
	first, err := Eval(expr, vars)
	if err != nil {
		t.Fatalf("Eval error: %v", err)
	}
	for i := 0; i < 100; i++ {
		again, err := Eval(expr, vars)
		if err != nil {
			t.Fatalf("Eval %d error: %v", i, err)
		}
		if again != first {
			t.Fatalf("expression is not deterministic: %v vs %v", again, first)
		}
	}
	if math.IsNaN(first) || math.IsInf(first, 0) {
		t.Fatalf("expression result must be finite, got %v", first)
	}
}
