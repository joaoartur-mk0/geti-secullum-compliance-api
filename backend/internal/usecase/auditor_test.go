package usecase

import (
	"testing"
	"time"

	"backend/internal/domain"
)

func strPtr(s string) *string { return &s }

// pairs monta pares "HH:MM"/"HH:MM"; string vazia representa marcação ausente.
func pairs(vals ...string) []domain.PunchPair {
	if len(vals)%2 != 0 {
		vals = append(vals, "")
	}
	out := make([]domain.PunchPair, 0, len(vals)/2)
	for i := 0; i < len(vals); i += 2 {
		p := domain.PunchPair{}
		if vals[i] != "" {
			p.Entrada = strPtr(vals[i])
		}
		if vals[i+1] != "" {
			p.Saida = strPtr(vals[i+1])
		}
		out = append(out, p)
	}
	return out
}

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

func TestMinutesBetween_CruzaMeiaNoite(t *testing.T) {
	cases := []struct {
		start, end string
		want       int
	}{
		{"08:00", "12:00", 240}, // mesmo dia
		{"22:00", "02:00", 240}, // vira a meia-noite
		{"18:00", "08:00", 840}, // interjornada típica (14h)
		{"23:30", "00:30", 60},  // vira a meia-noite (curto)
	}
	for _, c := range cases {
		got, err := domain.MinutesBetween(c.start, c.end)
		if err != nil {
			t.Fatalf("MinutesBetween(%s,%s): %v", c.start, c.end, err)
		}
		if got != c.want {
			t.Errorf("MinutesBetween(%s,%s) = %d, quer %d", c.start, c.end, got, c.want)
		}
	}
}

func TestFormatMinutes(t *testing.T) {
	cases := []struct {
		minutes int
		want    string
	}{
		{1198, "19h58min"},
		{102, "1h42min"},
		{540, "9h"}, // hora cheia não mostra "00min"
		{150, "2h30min"},
		{30, "0h30min"},
		{660, "11h"},
	}
	for _, c := range cases {
		if got := formatMinutes(c.minutes); got != c.want {
			t.Errorf("formatMinutes(%d) = %q, quer %q", c.minutes, got, c.want)
		}
	}
}

func TestRequiredBreakMinutes(t *testing.T) {
	cases := []struct {
		carga int
		want  int
	}{
		{440, 60}, // 7h20 (jornada de sábado) > 6h => 60min
		{480, 60}, // 8h
		{360, 15}, // 6h (jornada de domingo) => 15min
		{300, 15}, // 5h
		{240, 0},  // 4h => sem intervalo obrigatório
		{0, 0},
	}
	for _, c := range cases {
		if got := requiredBreakMinutes(c.carga); got != c.want {
			t.Errorf("requiredBreakMinutes(%d) = %d, quer %d", c.carga, got, c.want)
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

func TestProcessRules_IntervaloReduzidoJornadaLonga(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		CollaboratorID: 1,
		Previstas:      pairs("08:00", "12:00", "13:00", "17:00"), // 8h previstas
		Marcacoes:      pairs("08:00", "12:00", "12:30", "18:00"), // só 30min de intervalo
	}

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, TipoAlmocoReduzido)
	if !ok {
		t.Fatalf("esperava %q, veio %+v", TipoAlmocoReduzido, inc)
	}
	if got.Severity != domain.SeverityCritical {
		t.Errorf("severidade default deveria ser CRITICO, veio %q", got.Severity)
	}
}

// Caso real do payload de domingo: jornada prevista de 6h com intervalo previsto de
// 15min. O intervalo de 15min registrado é legal (Art. 71 §1º) e não pode ser apontado.
func TestProcessRules_IntervaloDe15MinutosEmJornadaDe6h(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		CollaboratorID: 36,
		Previstas:      pairs("08:00", "09:00", "09:15", "14:15"), // 6h
		Marcacoes:      pairs("06:56", "09:05", "09:20", "13:38"), // intervalo real de 15min
	}

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 36}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got, ok := findByType(inc, TipoAlmocoReduzido); ok {
		t.Errorf("15min é o mínimo legal para jornada de 6h; não deveria infringir: %q", got.Description)
	}
}

func TestProcessRules_IntervaloDe15MinutosEmJornadaLongaInfringe(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Previstas: pairs("07:00", "14:20"),                   // 7h20 (sábado, turno corrido)
		Marcacoes: pairs("07:00", "12:00", "12:15", "14:35"), // intervalo de 15min
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, TipoAlmocoReduzido); !ok {
		t.Fatalf("jornada de 7h20 exige 60min de intervalo, veio %+v", inc)
	}
}

func TestProcessRules_SeveridadeAlmocoConfiguravel(t *testing.T) {
	s := NewAuditorService()
	settings := allEnabled()
	settings.AlmocoSeverity = domain.SeverityAlert // tenant configurou ALERTA

	punch := &domain.DailyPunch{
		Previstas: pairs("08:00", "12:00", "13:00", "17:00"),
		Marcacoes: pairs("08:00", "12:00", "12:30", "17:00"),
	}
	inc, err := s.ProcessRules(settings, &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, TipoAlmocoReduzido)
	if !ok {
		t.Fatalf("esperava %q", TipoAlmocoReduzido)
	}
	if got.Severity != domain.SeverityAlert {
		t.Errorf("severidade deveria respeitar config ALERTA, veio %q", got.Severity)
	}
}

func TestProcessRules_InterjornadaCurta(t *testing.T) {
	s := NewAuditorService()
	yesterday := &domain.DailyPunch{Marcacoes: pairs("14:00", "18:00", "19:00", "22:00")}
	today := &domain.DailyPunch{Marcacoes: pairs("07:00", "")} // 9h de descanso

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, today, yesterday, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, TipoInterjornada); !ok {
		t.Fatalf("esperava %q (9h < 11h)", TipoInterjornada)
	}
}

// A última saída do dia pode estar em qualquer bloco — inclusive no primeiro, quando o
// expediente é corrido.
func TestProcessRules_InterjornadaUsaUltimaSaidaDoDia(t *testing.T) {
	s := NewAuditorService()
	yesterday := &domain.DailyPunch{Marcacoes: pairs("15:00", "23:00")} // turno corrido
	today := &domain.DailyPunch{Marcacoes: pairs("07:00", "")}          // 8h de descanso

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, today, yesterday, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, TipoInterjornada); !ok {
		t.Fatalf("esperava %q usando a saída do turno corrido (8h < 11h)", TipoInterjornada)
	}
}

func TestProcessRules_InterjornadaOk(t *testing.T) {
	s := NewAuditorService()
	yesterday := &domain.DailyPunch{Marcacoes: pairs("08:00", "12:00", "13:00", "18:00")}
	today := &domain.DailyPunch{Marcacoes: pairs("08:00", "")} // 14h de descanso

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, today, yesterday, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, TipoInterjornada); ok {
		t.Errorf("14h de descanso não deveria infringir")
	}
}

func TestProcessRules_HoraExtraUsaCargaPrevistaDoDia(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Previstas: pairs("08:00", "12:00", "13:00", "15:00"), // 6h previstas
		Marcacoes: pairs("08:00", "12:00", "13:00", "16:30"), // 7h30 trabalhadas
	}

	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// 1h30 de extra => Alerta (>1h e <=2h).
	if _, ok := findByType(inc, TipoAlertaHoraExtra); !ok {
		t.Fatalf("esperava %q usando a carga prevista de 6h, veio %+v", TipoAlertaHoraExtra, inc)
	}
}

func TestProcessRules_HoraExtraExcedenteCritico(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Previstas: pairs("08:00", "12:00", "13:00", "17:00"), // 8h
		Marcacoes: pairs("08:00", "12:00", "13:00", "20:00"), // 11h => 3h extra
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, TipoHoraExtra)
	if !ok {
		t.Fatalf("esperava %q (3h), veio %+v", TipoHoraExtra, inc)
	}
	if got.Severity != domain.SeverityCritical {
		t.Errorf("hora extra > 2h deveria ser CRITICO, veio %q", got.Severity)
	}
}

// Turno corrido (2 batidas) passou a ser auditado: antes a regra exigia 4 batidas e
// simplesmente ignorava esses colaboradores.
func TestProcessRules_HoraExtraEmTurnoCorrido(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Previstas: pairs("08:00", "12:00"), // 4h previstas
		Marcacoes: pairs("08:00", "14:30"), // 6h30 => 2h30 de extra
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, TipoHoraExtra); !ok {
		t.Fatalf("esperava %q em jornada de bloco único, veio %+v", TipoHoraExtra, inc)
	}
}

// Caso real do payload de sábado: previsão em bloco único de 7h20, batidas em dois
// blocos somando 7h30 => 10min de extra, abaixo do limiar de alerta.
func TestProcessRules_SabadoPrevisaoCorridaComBatidasEmDoisBlocos(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Previstas: pairs("07:00", "14:20"),                   // 7h20
		Marcacoes: pairs("07:00", "12:09", "13:58", "16:19"), // 7h30
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 24}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(inc) != 0 {
		t.Fatalf("10min de excedente não deveria gerar infração, veio %+v", inc)
	}
}

func TestProcessRules_SemCargaPrevistaViraInfracao(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Marcacoes: pairs("08:00", "12:00", "13:00", "17:00"), // trabalhou 8h
		// Previstas vazio: a Secullum não informou jornada para o dia.
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, TipoCargaNaoApurada)
	if !ok {
		t.Fatalf("esperava %q, veio %+v", TipoCargaNaoApurada, inc)
	}
	if got.Severity != domain.SeverityCritical {
		t.Errorf("carga não apurada deveria ser CRITICO, veio %q", got.Severity)
	}
	// E a jornada inteira NÃO pode virar hora extra.
	if _, ok := findByType(inc, TipoHoraExtra); ok {
		t.Errorf("sem carga prevista, hora extra não pode ser estimada")
	}
}

func TestProcessRules_SemCargaESemTrabalhoNaoInfringe(t *testing.T) {
	s := NewAuditorService()
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, &domain.DailyPunch{}, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(inc) != 0 {
		t.Fatalf("dia sem jornada e sem batidas (folga) não deveria gerar infração, veio %+v", inc)
	}
}

func TestProcessRules_DiaNeutroNaoCobraCarga(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Neutro:    true,
		Marcacoes: pairs("08:00", "12:00"),
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, TipoCargaNaoApurada); ok {
		t.Errorf("dia neutro não tem carga a cumprir; não deveria virar infração de dado")
	}
}

func TestProcessRules_TrabalhoEmDiaDeFolga(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Folga:     true,
		Marcacoes: pairs("08:00", "12:00", "13:00", "17:00"),
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, ok := findByType(inc, TipoTrabalhoEmFolga)
	if !ok {
		t.Fatalf("esperava %q, veio %+v", TipoTrabalhoEmFolga, inc)
	}
	if got.Severity != domain.SeverityCritical {
		t.Errorf("trabalho em folga deveria ser CRITICO, veio %q", got.Severity)
	}
	// Não duplica como hora extra nem como carga não apurada.
	if _, ok := findByType(inc, TipoHoraExtra); ok {
		t.Errorf("trabalho em folga não deve ser reportado também como hora extra")
	}
	if _, ok := findByType(inc, TipoCargaNaoApurada); ok {
		t.Errorf("folga é jornada prevista zero, não falha de cadastro")
	}
}

func TestProcessRules_ContagemImparApenasNoFechamento(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{Marcacoes: pairs("08:00", "")} // 1 batida (ímpar)
	settings := &domain.TenantSettings{Esquecimento: true}

	// Intra-dia (isClosing=false) sem jornada prevista: regra suspensa => NÃO infringe.
	incIntra, err := s.ProcessRules(settings, &domain.Collaborator{ID: 1}, punch, nil, time.Now(), false)
	if err != nil {
		t.Fatalf("erro inesperado (intra): %v", err)
	}
	if _, ok := findByType(incIntra, TipoBatidaEsquecida); ok {
		t.Errorf("contagem ímpar NÃO deveria infringir durante o expediente sem jornada")
	}

	// Fechamento (isClosing=true): contagem ímpar => infringe.
	incClose, err := s.ProcessRules(settings, &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err != nil {
		t.Fatalf("erro inesperado (fechamento): %v", err)
	}
	if _, ok := findByType(incClose, TipoBatidaEsquecida); !ok {
		t.Errorf("contagem ímpar no fechamento deveria infringir")
	}
}

func TestProcessRules_EsquecimentoIntraDiaUsaPrimeiraSaidaPrevista(t *testing.T) {
	s := NewAuditorService()
	now := time.Date(2026, 7, 13, 12, 45, 0, 0, time.Local) // 45min após a saída prevista
	punch := &domain.DailyPunch{
		Previstas: pairs("08:00", "12:00", "13:00", "17:00"),
		Marcacoes: pairs("08:00", ""), // só a entrada
	}

	inc, err := s.ProcessRules(&domain.TenantSettings{Esquecimento: true}, &domain.Collaborator{ID: 1}, punch, nil, now, false)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, ok := findByType(inc, TipoBatidaEsquecida); !ok {
		t.Fatalf("esperava %q intra-dia (passou 30min da 1ª saída prevista)", TipoBatidaEsquecida)
	}
}

func TestProcessRules_DadoInvalidoRetornaErro(t *testing.T) {
	s := NewAuditorService()
	punch := &domain.DailyPunch{
		Previstas: pairs("08:00", "12:00", "13:00", "17:00"),
		Marcacoes: pairs("08:00", "99:99", "13:00", "18:00"), // horário inválido
	}
	inc, err := s.ProcessRules(allEnabled(), &domain.Collaborator{ID: 1}, punch, nil, time.Now(), true)
	if err == nil {
		t.Fatalf("esperava erro de dado inválido, veio nil")
	}
	// Nenhuma infração de carga pode ser derivada de dado ilegível.
	for _, tipo := range []string{TipoAlmocoReduzido, TipoHoraExtra, TipoAlertaHoraExtra, TipoCargaNaoApurada} {
		if _, ok := findByType(inc, tipo); ok {
			t.Errorf("não deveria gerar %q a partir de horário inválido", tipo)
		}
	}
}
