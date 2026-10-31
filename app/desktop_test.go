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
	}
	for _, c := range cases {
		if got := StartHidden(c.args); got != c.want {
			t.Errorf("StartHidden(%v) = %v, quería %v", c.args, got, c.want)
		}
	}
}

func TestTrayTooltip(t *testing.T) {
	cases := map[int]string{
		0: "PostaControl",
		1: "PostaControl — 1 correo sin leer",
		5: "PostaControl — 5 correos sin leer",
	}
	for n, want := range cases {
		if got := TrayTooltip(n); got != want {
			t.Errorf("TrayTooltip(%d) = %q, quería %q", n, got, want)
		}
	}
}
