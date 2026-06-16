package codevariations

import "testing"

func TestNormalizeProductCode(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"63022rs", "6302 2rs"},
		{"6205", "6205"},
		{"6205zz", "6205zz"}, // 4 digits -> length < 5, no split
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeProductCode(c.in); got != c.want {
			t.Errorf("NormalizeProductCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCreateCodeVariations(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			"6205 2rs(korea).fag",
			[]string{
				"6205 2rs(korea).fag",
				"62052rs(korea).fag",
				"6205 2 rs(korea).fag",
				"62052rs(korea)fag",
				"6205 2rs(korea) fag",
			},
		},
		{
			"gwm-33a",
			[]string{
				"gwm-33a",
				"gwm-33 a",
				"gwm33a",
				"gwm 33a",
			},
		},
	}

	for _, c := range cases {
		got := CreateCodeVariations(c.in)
		gotSet := make(map[string]bool, len(got))
		for _, v := range got {
			gotSet[v] = true
		}
		for _, w := range c.want {
			if !gotSet[w] {
				t.Errorf("CreateCodeVariations(%q) missing %q, got %v", c.in, w, got)
			}
		}
	}
}
