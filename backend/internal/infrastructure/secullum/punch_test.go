package secullum

import "testing"

func ptr(s string) *string { return &s }

func TestNormalizeTime(t *testing.T) {
	cases := []struct {
		in   *string
		want *string // nil quando esperamos "sem batida"
	}{
		{ptr("08:00"), ptr("08:00")},
		{ptr("18:03"), ptr("18:03")},
		{ptr("INSS"), nil},  // abono/afastamento
		{ptr("FOLGA"), nil}, // marcador textual
		{ptr(""), nil},      // vazio
		{nil, nil},          // já ausente
	}
	for _, c := range cases {
		got := normalizeTime(c.in)
		if (got == nil) != (c.want == nil) {
			t.Errorf("normalizeTime(%v): got nil=%v, want nil=%v", c.in, got == nil, c.want == nil)
			continue
		}
		if got != nil && *got != *c.want {
			t.Errorf("normalizeTime(%v) = %q, want %q", c.in, *got, *c.want)
		}
	}
}

func TestAbonoMarker(t *testing.T) {
	if m := abonoMarker(ptr("08:00"), ptr("12:00"), nil, nil); m != "" {
		t.Errorf("dia normal não deveria ter marcador, veio %q", m)
	}
	if m := abonoMarker(ptr("INSS"), nil, nil, nil); m != "INSS" {
		t.Errorf("esperava marcador INSS, veio %q", m)
	}
}
