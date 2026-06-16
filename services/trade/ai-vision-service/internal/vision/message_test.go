package vision

import (
	"reflect"
	"strings"
	"testing"
)

func TestMessageAnalysisCacheKey(t *testing.T) {
	short := messageAnalysisCacheKey("6205 berapa?")
	if len(short) > 64 {
		t.Errorf("short message cache key too long: %d", len(short))
	}

	long := messageAnalysisCacheKey(strings.Repeat("a", 100))
	if len(long) != 64 {
		t.Errorf("long message cache key = %d chars, want 64", len(long))
	}
}

func TestParseMessageAnalysisResponse(t *testing.T) {
	cases := []struct {
		name    string
		content string
		message string
		want    *MessageAnalysis
	}{
		{
			name:    "full JSON response",
			content: `{"intent":"price_check","products":["6205"],"quantity":2,"keywords":["6205","harga"],"containsProfanity":false,"enhancedQuery":"6205"}`,
			message: "harga 6205 berapa, mau 2",
			want: &MessageAnalysis{
				Keywords:          []string{"6205", "harga"},
				Intent:            "price_check",
				Products:          []string{"6205"},
				Quantity:          2,
				ContainsProfanity: false,
				EnhancedQuery:     "6205",
				OriginalMessage:   "harga 6205 berapa, mau 2",
			},
		},
		{
			name:    "JSON wrapped in markdown fence",
			content: "```json\n{\"intent\":\"greeting\",\"keywords\":[\"halo\"]}\n```",
			message: "halo",
			want: &MessageAnalysis{
				Keywords:        []string{"halo"},
				Intent:          "greeting",
				Products:        []string{},
				Quantity:        1,
				EnhancedQuery:   "halo",
				OriginalMessage: "halo",
			},
		},
		{
			name:    "missing optional fields default",
			content: `{"keywords":["6205"]}`,
			message: "6205",
			want: &MessageAnalysis{
				Keywords:        []string{"6205"},
				Intent:          "general_search",
				Products:        []string{},
				Quantity:        1,
				EnhancedQuery:   "6205",
				OriginalMessage: "6205",
			},
		},
		{
			name:    "unparseable response falls back",
			content: "maaf, saya tidak mengerti",
			message: "asdfasdf",
			want: &MessageAnalysis{
				Keywords:        []string{},
				EnhancedQuery:   "asdfasdf",
				OriginalMessage: "asdfasdf",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseMessageAnalysisResponse(c.content, c.message)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseMessageAnalysisResponse(%q, %q) = %#v, want %#v", c.content, c.message, got, c.want)
			}
		})
	}
}
