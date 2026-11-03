package app

import "testing"

func TestStartHidden(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"postacontrol", "--hidden"}, true},
		{[]string{"postacontrol"}, false},
		{[]string{"postacontrol", "--hiddenx"}, false},
		{[]string{"--hidden"}, true},
		{[]string{"--hidden", "mailto:a@x.com"}, false},
		{[]string{}, false},
	}
	for _, c := range cases {
		if got := StartHidden(c.args); got != c.want {
			t.Errorf("StartHidden(%v) = %v, quería %v", c.args, got, c.want)
		}
	}
}

func TestQuitKeyBindings(t *testing.T) {
	quit := 0
	kb := QuitKeyBindings(func() { quit++ })
	f, ok := kb["ctrl+q"]
	if !ok || len(kb) != 1 {
		t.Fatalf("falta el atajo ctrl+q: %v", kb)
	}
	f(nil)
	if quit != 1 {
		t.Fatal("ctrl+q no sale")
	}
}
