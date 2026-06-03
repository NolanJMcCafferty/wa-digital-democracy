package db

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNonEmptyStrings(t *testing.T) {
	got := nonEmptyStrings([]string{"a", "", "  ", " b ", "c"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Empty input must yield a non-nil empty slice (callers append).
	if g := nonEmptyStrings(nil); g == nil || len(g) != 0 {
		t.Errorf("nil input: got %v, want empty non-nil slice", g)
	}
}

func TestTimeOrNull(t *testing.T) {
	if v := timeOrNull(time.Time{}); v.Valid {
		t.Error("zero time should be invalid")
	}
	now := time.Now()
	v := timeOrNull(now)
	if !v.Valid || !v.Time.Equal(now) {
		t.Errorf("non-zero time: valid=%v time=%v", v.Valid, v.Time)
	}
}

func TestDateOrNull(t *testing.T) {
	if v := dateOrNull(time.Time{}); v.Valid {
		t.Error("zero date should be invalid")
	}
	if v := dateOrNull(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); !v.Valid {
		t.Error("non-zero date should be valid")
	}
}

func TestStrOrNull(t *testing.T) {
	if strOrNull("") != nil {
		t.Error("empty must be nil")
	}
	got := strOrNull("x")
	if got == nil || *got != "x" {
		t.Errorf("got %v, want pointer to \"x\"", got)
	}
}

func TestNonNilStrings(t *testing.T) {
	if got := nonNilStrings(nil); got == nil {
		t.Error("nil input should return empty non-nil slice")
	}
	in := []string{"a"}
	if got := nonNilStrings(in); len(got) != 1 || got[0] != "a" {
		t.Errorf("non-nil input not preserved: %v", got)
	}
}

func TestFileSizeOrNull(t *testing.T) {
	if v := fileSizeOrNull(0); v != nil {
		t.Error("zero must be nil")
	}
	if v := fileSizeOrNull(42); v != int64(42) {
		t.Errorf("non-zero: got %v", v)
	}
}

func TestMarshalJSONDefault(t *testing.T) {
	b, err := marshalJSONDefault(nil, []string{"x"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `["x"]` {
		t.Errorf("default not used: %s", b)
	}
	b, _ = marshalJSONDefault(map[string]int{"k": 1}, nil)
	var m map[string]int
	if err := json.Unmarshal(b, &m); err != nil || m["k"] != 1 {
		t.Errorf("value not marshaled: %s err=%v", b, err)
	}
}

func TestDefaultStr(t *testing.T) {
	if defaultStr("", "d") != "d" {
		t.Error("empty should fall to default")
	}
	if defaultStr("x", "d") != "x" {
		t.Error("non-empty should be returned")
	}
}

func TestNullStringArray(t *testing.T) {
	v := nullStringArray(nil)
	arr, ok := v.([]string)
	if !ok || len(arr) != 0 {
		t.Errorf("nil input: got %T %v, want empty []string", v, v)
	}
	v = nullStringArray([]string{"a"})
	if arr, ok := v.([]string); !ok || arr[0] != "a" {
		t.Errorf("non-nil input not preserved: %v", v)
	}
}

func TestLowerTrim(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"FOO":        "foo",
		"  Hello  ":  "hello",
		"\tTAB":      "tab",
		"MixED CaSe": "mixed case",
		"ünIcode":    "ünicode",
	}
	for in, want := range cases {
		if got := lowerTrim(in); got != want {
			t.Errorf("lowerTrim(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDatePtrOrNull(t *testing.T) {
	if v := datePtrOrNull(nil); v.Valid {
		t.Error("nil should be invalid")
	}
	zero := time.Time{}
	if v := datePtrOrNull(&zero); v.Valid {
		t.Error("zero pointer should be invalid")
	}
	tt := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	if v := datePtrOrNull(&tt); !v.Valid {
		t.Error("real date should be valid")
	}
}

func TestBoolPtrOrNull(t *testing.T) {
	if boolPtrOrNull(nil) != nil {
		t.Error("nil should be nil")
	}
	tr := true
	if boolPtrOrNull(&tr) != true {
		t.Error("true pointer not unwrapped")
	}
}

func TestFloatIntPtrOrNull(t *testing.T) {
	if floatPtrOrNull(nil) != nil || intPtrOrNull(nil) != nil {
		t.Error("nil should be nil")
	}
	f := 3.14
	if floatPtrOrNull(&f) != 3.14 {
		t.Error("float pointer not unwrapped")
	}
	n := 7
	if intPtrOrNull(&n) != 7 {
		t.Error("int pointer not unwrapped")
	}
}

func TestHasMinLetterRatio(t *testing.T) {
	cases := []struct {
		s     string
		ratio float64
		want  bool
	}{
		{"", 0.5, false},       // empty → false
		{"   ", 0.5, false},    // spaces are skipped → total 0
		{"abc", 0.5, true},     // 100% letters
		{"a1b2", 0.5, true},    // 50% letters, ratio 0.5
		{"a123", 0.5, false},   // 25% letters
		{"foo bar", 0.9, true}, // spaces ignored
		{"###", 0.5, false},    // pure symbols
	}
	for _, c := range cases {
		if got := hasMinLetterRatio(c.s, c.ratio); got != c.want {
			t.Errorf("hasMinLetterRatio(%q, %v) = %v, want %v", c.s, c.ratio, got, c.want)
		}
	}
}

func TestBillStageCaseSQL(t *testing.T) {
	sql := billStageCaseSQL("status")
	// Verify the expected stage labels appear and that the column name is interpolated.
	for _, want := range []string{
		"CASE", "Session law", "Vetoed", "Signed by Governor",
		"On Governor''s desk", "Passed Legislature", "Shelved",
		"Reintroduced", "Died", "Passed chamber", "On floor calendar",
		"In committee", "Introduced", "In progress", "status ILIKE",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("billStageCaseSQL missing %q", want)
		}
	}
	// Pick a different column name to ensure interpolation works.
	if !strings.Contains(billStageCaseSQL("b.current_status"), "b.current_status ILIKE") {
		t.Error("column name not propagated")
	}
}

func TestZeroTimeToNil(t *testing.T) {
	if zeroTimeToNil(time.Time{}) != nil {
		t.Error("zero should be nil")
	}
	tt := time.Date(2026, 6, 3, 12, 0, 0, 0, time.FixedZone("PDT", -7*3600))
	got, ok := zeroTimeToNil(tt).(time.Time)
	if !ok {
		t.Fatalf("non-zero should return time.Time, got %T", got)
	}
	if got.Location() != time.UTC {
		t.Errorf("expected UTC, got %v", got.Location())
	}
	if !got.Equal(tt) {
		t.Errorf("instants differ: got %v want %v", got, tt)
	}
}

func TestNormalizePersonName(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"   ":           "",
		"jane doe":      "JANE DOE",
		"  Jane   Doe ": "JANE DOE",
		"jane\tdoe":     "JANE DOE",
	}
	for in, want := range cases {
		if got := normalizePersonName(in); got != want {
			t.Errorf("normalizePersonName(%q) = %q, want %q", in, got, want)
		}
	}
}
