package attach

import "testing"

func TestParseLsof(t *testing.T) {
	// Server 100 for session "work", client 200 connected to it, plus an
	// unrelated socketpair.
	out := "p100\nf5\nd0xaaa\nn/tmp/zellij-501/contract_version_1/work\n" +
		"f6\nd0xbbb\nn/tmp/zellij-501/contract_version_1/work\n" +
		"f7\nd0xccc\nn->0xddd\n" +
		"p200\nf5\nd0xeee\nn->0xbbb\n"
	got := parseLsof(out)
	if len(got[100]) != 3 || len(got[200]) != 1 {
		t.Fatalf("parseLsof = %+v", got)
	}
	if got[200][0] != (socketFD{addr: "0xeee", name: "->0xbbb"}) {
		t.Fatalf("client fd = %+v", got[200][0])
	}
}

func TestIsNonClient(t *testing.T) {
	cases := map[string]bool{
		"attach work":                 false,
		"":                            false,
		"-s foo":                      false,
		"--server /tmp/zellij/x":      true,
		"--session foo action detach": true,
		"ls -n":                       true,
		"-n compact -s foo":           false,
	}
	for args, want := range cases {
		if got := isNonClient(fields(args)); got != want {
			t.Errorf("isNonClient(%q) = %v, want %v", args, got, want)
		}
	}
}

func fields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s + " " {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	return out
}

func TestSessionFromTitle(t *testing.T) {
	running := []string{"work", "work-2", "a | b"}
	cases := map[string]string{
		"work | ~/src":     "work",
		"work-2 | vim":     "work-2",
		"work":             "work",
		"a | b | htop":     "a | b",
		"workshop | x":     "",
		"~/src":            "",
		"other | work | x": "",
	}
	for title, want := range cases {
		if got := SessionFromTitle(title, running); got != want {
			t.Errorf("SessionFromTitle(%q) = %q, want %q", title, got, want)
		}
	}
}
