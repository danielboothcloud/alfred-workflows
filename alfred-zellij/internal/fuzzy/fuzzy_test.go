package fuzzy

import (
	"reflect"
	"testing"
)

func TestFilter(t *testing.T) {
	names := []string{"agent-readiness", "gateway", "work", "iac", "seeker", "antis"}
	cases := map[string][]string{
		"":     names,
		"work": {"work"},
		"rdns": {"agent-readiness"},
		"ga":   {"gateway", "agent-readiness"},
		"zzz":  nil,
	}
	for q, want := range cases {
		var got []string
		for _, i := range Filter(q, names) {
			got = append(got, names[i])
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Filter(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestScoreOrder(t *testing.T) {
	if !(Score("ser", "seeker") == 0 || Score("see", "seeker") > Score("see", "foresee")) {
		t.Error("prefix should beat substring")
	}
	if Score("readi", "agent-readiness") <= Score("adns", "agent-readiness") {
		t.Error("word prefix should beat subsequence")
	}
}
