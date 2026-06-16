package vision

import (
	"reflect"
	"testing"
)

func TestParseMultiProductInput(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "comma separated",
			text: "ada 6203, 6204, 6205 ready ga",
			want: []string{"6203", "6204", "6205 ga"},
		},
		{
			name: "comma with dan on last item",
			text: "tolong cari 6203, 6204 dan 6205",
			want: []string{"6203", "6204", "6205"},
		},
		{
			name: "numbered list",
			text: "tolong carikan:\n1. 6203\n2. SKF 6204\n3. 6205 2RS",
			want: []string{"6203", "SKF 6204", "6205 2RS"},
		},
		{
			name: "bullet points",
			text: "minta info\n- 6203\n- 6204\n- NTN 6205",
			want: []string{"6203", "6204", "NTN 6205"},
		},
		{
			name: "single product (no split)",
			text: "ada bearing 6203 2RS ready ga",
			want: []string{"ada bearing 6203 2RS ready ga"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseMultiProductInput(c.text)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseMultiProductInput(%q) = %#v, want %#v", c.text, got, c.want)
			}
		})
	}
}

func TestIsMultiProductSearch(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"ada 6203, 6204, 6205 ready ga", true},
		{"6203 dan 6205", true},
		{"ada bearing 6203 2RS ready ga", false},
		{"6203", false},
	}

	for _, c := range cases {
		got := isMultiProductSearch(c.text)
		if got != c.want {
			t.Errorf("isMultiProductSearch(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}
