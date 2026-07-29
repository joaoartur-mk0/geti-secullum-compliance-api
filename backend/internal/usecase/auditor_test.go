package usecase

import (
	"testing"
	"time"

	"backend/internal/domain"
)

func strPtr(s string) *string { return &s }

// allEnabled devolve settings com todas as regras ligadas e severidades no default.
func allEnabled() *domain.TenantSettings {
	return &domain.TenantSettings{
		Almoco:       true,
		Interjornada: true,
		Hextras:      true,
		Esquecimento: true,
	}
}

// findByType procura uma inconsistência pelo Type.
func findByType(list []domain.AuditInconsistency, t string) (domain.AuditInconsistency, bool) {
	for _, i := range list {
		if i.Type == t {
			return i, true
		}
	}
	return domain.AuditInconsistency{}, false
}

func TestExpectedDailyHours(t *testing.T) {
	date := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC) // dia da semana fixo para o teste
	weekday := int(date.Weekday())

	t.Run("sem jornada sincronizada para o dia", func(t *testing.T) {
		collab := &domain.Collaborator{}
		if _, ok := expectedDailyHours(collab, date); ok {
			t.Errorf("esperava ok=false sem Schedules")
		}
	})

	t.Run("usa CargaMinutos quando presente", func(t *testing.T) {
		collab := &domain.Collaborator{Schedules: []domain.CollaboratorSchedule{
			{DiaSemana: weekday, CargaMinutos: 440}, // 7h20 — vem calculado pela Secullum
		}}
		h, ok := expectedDailyHours(collab, date)
		if !ok || h != 440.0/60.0 {
			t.Errorf("h=%.4f ok=%v, quer %.4f/true", h, ok, 440.0/60.0)
		}
	})

	t.Run("calcula dos horários quando não há CargaMinutos", func(t *testing.T) {
		collab := &domain.Collaborator{Schedules: []domain.CollaboratorSchedule{
			{DiaSemana: weekday, Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "17:00"},
		}}
		h, ok := expectedDailyHours(collab, date)
		if !ok || h != 8.0 {
			t.Errorf("h=%.2f ok=%v, quer 8.00/true", h, ok)
		}
	})

	t.Run("folga contratual (Carga=0, sem horários) => 0h esperadas", func(t *testing.T) {
		collab := &domain.Collaborator{Schedules: []domain.CollaboratorSchedule{
			{DiaSemana: weekday}, // dia sincronizado, mas sem carga nem horários = folga
		}}
		h, ok := expectedDailyHours(collab, date)
		if !ok || h != 0 {
			t.Errorf("h=%.2f ok=%v, quer 0.00/true (folga => qualquer trabalho vira extra)", h, ok)
		}
	})

	t.Run("dia da semana errado não casa", func(t *testing.T) {
		collab := &domain.Collaborator{Schedules: []domain.CollaboratorSchedule{
			{DiaSemana: (weekday + 1) % 7, CargaMinutos: 440},
		}}
		if _, ok := expectedDailyHours(collab, date); ok {
			t.Errorf("esperava ok=false: jornada é de outro dia da semana")
		}
	})
}

func TestDurationBetween_CruzaMeiaNoite(t *testing.T) {
	parse := func(s string) time.Time {
		tm, err := parseClock(s)
		if err != nil {
			t.Fatalf("parseClock(%q): %v", s, err)
		}
		return tm
	}

	cases := []struct {
		start, end string
		wantHours  float64
	}{
		{"08:00", "12:00", 4},  // mesmo dia
		{"22:00", "02:00", 4},  // vira a meia-noite
		{"18:00", "08:00", 14}, // interjornada típica
		{"23:30", "00:30", 1},  // vira a meia-noite (curto)
	}
	for _, c := range cases {
		got := durationBetween(parse(c.start), parse(c.end)).Hours()
		if got != c.wantHours {
			t.Errorf("durationBetween(%s,%s) = %.2fh, quer %.2fh", c.start, c.end, got, c.wantHours)
		}
	}
}

func TestFormatHoursMinutes(t *testing.T) {
	cases := []struct {
		hours float64
		want  string
	}{
		{19.97, "19h58min"}, // caso real que motivou a mudança (era "19.97 horas")
		{1.70, "1h42min"},   // caso real (era "1.70 horas")
		{9.0, "9h"},         // hora cheia não mostra "00min"
		{2.5, "2h30min"},
		{0.5, "0h30min"},
		{1.999, "2h"}, // arredondamento não gera "1h60min"
		{11.0, "11h"},
	}
	for _, c := range cases {
		if got := formatHoursMinutes(c.hours); got != c.want {
			t.Errorf("formatHoursMinutes(%.3f) = %q, quer %q", c.hours, got, c.want)
		}
	}
}

func TestProcessRules_SettingsNil(t *testing.T) {
	s := NewAuditorService()
	inc, err := s.ProcessRules(nil, &domain.Collaborator{ID: 1}, &domain.DailyPunch{}, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(inc) != 0 {
		t.Fatalf("esperava 0 inconsistências, veio %d", len(inc))
	}
}

func TestProcessRules_AlmocoReduzido(t *testing.T) {
	s := NewAuditorService()
	collab := &domain.Collaborator{ID: 1}
	punch := &domain.DailyPunch{
		CollaboratorID: 1,
		Entrada1:       strPtr("08:00"),
		Saida1:         strPtr("12:00"),
		Entrada2:       strPtr("12:30"), // só 30min de almoço
		Saida2:         strPtr("18:00"),
	}

	inc, err := s.ProcessRules(allEnabled(), collab, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, "Almoço Reduzido")
	if !ok {
		t.Fatalf("esperava 'Almoço Reduzido', veio %+v", inc)
	}
	if got.Severity != domain.SeverityCritical {
		t.Errorf("severidade default deveria ser CRITICO, veio %q", got.Severity)
	}
}

func TestProcessRules_AlmocoSeveridadeConfiguravel(t *testing.T) {
	s := NewAuditorService()
	settings := allEnabled()
	settings.AlmocoSeverity = domain.SeverityAlert // tenant configurou ALERTA

	punch := &domain.DailyPunch{
		CollaboratorID: 1,
		Saida1:         strPtr("12:00"),
		Entrada2:       strPtr("12:30"),
	}
	inc, err := s.ProcessRules(settings, &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, "Almoço Reduzido")
	if !ok {
		t.Fatalf("esperava 'Almoço Reduzido'")
	}
	if got.Severity != domain.SeverityAlert {
		t.Errorf("severidade deveria respeitar config ALERTA, veio %q", got.Severity)
	}
}

func TestProcessRules_AlmocoNormalNaoInfringe(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Saida1:   strPtr("12:00"),
		Entrada2: strPtr("13:00"), // 60min exatos
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, "Almoço Reduzido"); ok {
		t.Errorf("almoço de 60min não deveria infringir")
	}
}

func TestProcessRules_InterjornadaCurta(t *testing.T) {
	s := NewAuditorService()
	yesterday := &domain.DailyPunch{Saida2: strPtr("22:00")}
	today := &domain.DailyPunch{CollaboratorID: 1, Entrada1: strPtr("07:00")} // 9h de descanso

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, today, yesterday, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, "Interjornada Curta"); !ok {
		t.Fatalf("esperava 'Interjornada Curta' (9h < 11h)")
	}
}

func TestProcessRules_InterjornadaOk(t *testing.T) {
	s := NewAuditorService()
	yesterday := &domain.DailyPunch{Saida2: strPtr("18:00")}
	today := &domain.DailyPunch{Entrada1: strPtr("08:00")} // 14h de descanso

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, today, yesterday, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, "Interjornada Curta"); ok {
		t.Errorf("14h de descanso não deveria infringir")
	}
}

func TestProcessRules_HoraExtraUsaCargaContratual(t *testing.T) {
	s := NewAuditorService()
	// A jornada é por dia da semana; o punch precisa cair no mesmo DiaSemana da
	// jornada sincronizada para o auditor encontrá-la.
	date := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

	// Jornada contratual de 6h (08-12 / 13-15) nesse dia da semana.
	collab := &domain.Collaborator{
		ID: 1,
		Schedules: []domain.CollaboratorSchedule{
			{DiaSemana: int(date.Weekday()), Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "15:00"},
		},
	}
	// Trabalhou 7.5h (08-12 / 13-16:30) => 1.5h extra sobre a carga de 6h.
	punch := &domain.DailyPunch{
		Date:     date,
		Entrada1: strPtr("08:00"),
		Saida1:   strPtr("12:00"),
		Entrada2: strPtr("13:00"),
		Saida2:   strPtr("16:30"),
	}

	inc, err := s.ProcessRules(allEnabled(), collab, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// 1.5h extra => Alerta (>1h e <=2h). Se usasse 8h fixas, daria negativo (nenhuma).
	if _, ok := findByType(inc, "Alerta de Hora Extra"); !ok {
		t.Fatalf("esperava 'Alerta de Hora Extra' usando carga contratual de 6h, veio %+v", inc)
	}
}

func TestProcessRules_HoraExtraExcedenteCritico(t *testing.T) {
	s := NewAuditorService()
	// Sem schedule => usa 8h padrão. Trabalhou 11h => 3h extra => Crítico.
	punch := &domain.DailyPunch{
		Entrada1: strPtr("08:00"),
		Saida1:   strPtr("12:00"),
		Entrada2: strPtr("13:00"),
		Saida2:   strPtr("20:00"),
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, "Hora Extra Excedente")
	if !ok {
		t.Fatalf("esperava 'Hora Extra Excedente' (3h), veio %+v", inc)
	}
	if got.Severity != domain.SeverityCritical {
		t.Errorf("hora extra > 2h deveria ser CRITICO, veio %q", got.Severity)
	}
}

func TestProcessRules_ContagemImparApenasNoFechamento(t *testing.T) {
	s := NewAuditorService()
	// Só 1 batida (número ímpar).
	punch := &domain.DailyPunch{CollaboratorID: 1, Entrada1: strPtr("08:00")}
	settings := &domain.TenantSettings{Esquecimento: true}

	// Intra-dia (isClosing=false) sem jornada: regra suspensa => NÃO infringe.
	incIntra, err := s.ProcessRules(settings, &domain.Collaborator{ID: 1}, punch, nil, time.Now(), false)
	if err != nil {
		t.Fatalf("erro inesperado (intra): %v", err)
	}
	if _, ok := findByType(incIntra, "Batida Esquecida"); ok {
		t.Errorf("contagem ímpar NÃO deveria infringir durante o expediente sem jornada")
	}

	// Fechamento (isClosing=true): contagem ímpar => infringe.
	incClose, err := s.ProcessRules(settings, &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado (fechamento): %v", err)
	}
	if _, ok := findByType(incClose, "Batida Esquecida"); !ok {
		t.Errorf("contagem ímpar no fechamento deveria infringir")
	}
}

func TestProcessRules_EsquecimentoIntraDiaComJornada(t *testing.T) {
	s := NewAuditorService()
	// Só entrada batida; agora são 12:45 (passou 30min da saída p/ almoço de 12:00).
	punch := &domain.DailyPunch{CollaboratorID: 1, Entrada1: strPtr("08:00")}
	now := time.Date(2026, 7, 13, 12, 45, 0, 0, time.Local)

	// A jornada é por dia da semana; precisa bater com o DiaSemana de `now`.
	collab := &domain.Collaborator{
		ID: 1,
		Schedules: []domain.CollaboratorSchedule{
			{DiaSemana: int(now.Weekday()), Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:00", Saida2: "17:00"},
		},
	}

	inc, err := s.ProcessRules(&domain.TenantSettings{Esquecimento: true}, collab, punch, nil, now, false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, "Batida Esquecida"); !ok {
		t.Fatalf("esperava 'Batida Esquecida' intra-dia (passou 30min do almoço)")
	}
}

func TestProcessRules_DadoInvalidoRetornaErro(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Saida1:   strPtr("99:99"), // horário inválido
		Entrada2: strPtr("13:00"),
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err == nil {
		t.Fatalf("esperava erro de dado inválido, veio nil")
	}
	// A inconsistência de almoço NÃO deve ser gerada a partir de dado inválido.
	if _, ok := findByType(inc, "Almoço Reduzido"); ok {
		t.Errorf("não deveria gerar infração a partir de horário inválido")
	}
}
