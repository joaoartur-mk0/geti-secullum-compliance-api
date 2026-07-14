package secullum

import "testing"

func TestParseHorario(t *testing.T) {
	cases := []struct {
		name   string
		desc   string
		wantOK bool
		e1, s1 string
		e2, s2 string
	}{
		{
			name:   "jornada com intervalo",
			desc:   "Horário 08:00/12:00/14:00/18:00",
			wantOK: true, e1: "08:00", s1: "12:00", e2: "14:00", s2: "18:00",
		},
		{
			name:   "jornada com intervalo variante",
			desc:   "Horário 07:30/12:00/13:30/18:00",
			wantOK: true, e1: "07:30", s1: "12:00", e2: "13:30", s2: "18:00",
		},
		{
			name:   "jornada corrida (2 horários)",
			desc:   "Horário 08:00/17:00",
			wantOK: true, e1: "08:00", s1: "17:00",
		},
		{name: "descrição vazia", desc: "", wantOK: false},
		{name: "sem horários", desc: "Comercial", wantOK: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sch, ok := parseHorario(c.desc)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, quer %v", ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if sch.Entrada1 != c.e1 || sch.Saida1 != c.s1 || sch.Entrada2 != c.e2 || sch.Saida2 != c.s2 {
				t.Errorf("parse = {%q %q %q %q}, quer {%q %q %q %q}",
					sch.Entrada1, sch.Saida1, sch.Entrada2, sch.Saida2, c.e1, c.s1, c.e2, c.s2)
			}
		})
	}
}
