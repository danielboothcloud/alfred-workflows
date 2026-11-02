package main

import (
	"reflect"
	"testing"

	"alfred-apps/internal/apps"
)

func TestMatch(t *testing.T) {
	list := []apps.App{
		{Name: "Calculator", File: "Calculator"},
		{Name: "Visual Studio Code", File: "Visual Studio Code"},
		{Name: "Voice Memos", File: "VoiceMemos"},
		{Name: "Logi Options+", File: "logioptionsplus"},
	}
	names := func(idx []int) []string {
		var out []string
		for _, i := range idx {
			out = append(out, list[i].Name)
		}
		return out
	}
	cases := map[string][]string{
		"":           {"Calculator", "Visual Studio Code", "Voice Memos", "Logi Options+"},
		"calc":       {"Calculator"},
		"code":       {"Visual Studio Code"},
		"vsc":        {"Visual Studio Code"},
		"voicememos": {"Voice Memos"}, // matched via the bundle file name, listed once
		"memos":      {"Voice Memos"},
		"optionsplu": {"Logi Options+"},
		"zzz":        nil,
	}
	for q, want := range cases {
		if got := names(match(q, list)); !reflect.DeepEqual(got, want) {
			t.Errorf("match(%q) = %v, want %v", q, got, want)
		}
	}
}
