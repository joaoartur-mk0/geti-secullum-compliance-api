package messaging

import (
	"testing"
	"time"

	"backend/internal/domain"
)

func TestIndexPunchesByCollaborator(t *testing.T) {
	date := time.Now()
	punches := []domain.DailyPunch{
		{CollaboratorID: 10, Date: date},
		{CollaboratorID: 20, Date: date},
	}

	m := indexPunchesByCollaborator(punches)
	if len(m) != 2 {
		t.Fatalf("esperava 2 entradas, veio %d", len(m))
	}
	if _, ok := m[10]; !ok {
		t.Errorf("esperava punch do colaborador 10")
	}
	if _, ok := m[30]; ok {
		t.Errorf("não deveria haver punch do colaborador 30")
	}
}

func TestIndexPunchesByCollaborator_Vazio(t *testing.T) {
	m := indexPunchesByCollaborator(nil)
	if len(m) != 0 {
		t.Errorf("esperava mapa vazio, veio %d entradas", len(m))
	}
}

func TestIndexPunchesByDay(t *testing.T) {
	d1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	punches := []domain.DailyPunch{
		{CollaboratorID: 10, Date: d1},
		{CollaboratorID: 20, Date: d1},
		{CollaboratorID: 10, Date: d2},
	}

	byDay := indexPunchesByDay(punches)
	if len(byDay) != 2 {
		t.Fatalf("esperava 2 dias, veio %d", len(byDay))
	}
	if _, ok := byDay["2026-07-01"][20]; !ok {
		t.Errorf("esperava punch do colaborador 20 em 2026-07-01")
	}
	if _, ok := byDay["2026-07-02"][20]; ok {
		t.Errorf("colaborador 20 não bateu ponto em 2026-07-02")
	}
	if _, ok := byDay["2026-07-02"][10]; !ok {
		t.Errorf("esperava punch do colaborador 10 em 2026-07-02")
	}
}

func TestResolveTargetDays_DiaUnico(t *testing.T) {
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	dias := resolveTargetDays("2026-07-05", "", "", now)
	if len(dias) != 1 || dias[0].Format("2006-01-02") != "2026-07-05" {
		t.Fatalf("esperava [2026-07-05], veio %v", dias)
	}
}

func TestResolveTargetDays_SemNadaAuditaDMenos1(t *testing.T) {
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	dias := resolveTargetDays("", "", "", now)
	if len(dias) != 1 || dias[0].Format("2006-01-02") != "2026-07-09" {
		t.Fatalf("esperava [2026-07-09] (D-1), veio %v", dias)
	}
}

func TestResolveTargetDays_Periodo(t *testing.T) {
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	dias := resolveTargetDays("", "2026-07-01", "2026-07-03", now)
	want := []string{"2026-07-01", "2026-07-02", "2026-07-03"}
	if len(dias) != len(want) {
		t.Fatalf("esperava %d dias, veio %d: %v", len(want), len(dias), dias)
	}
	for i, w := range want {
		if got := dias[i].Format("2006-01-02"); got != w {
			t.Errorf("dia %d: esperava %s, veio %s", i, w, got)
		}
	}
}

func TestResolveTargetDays_PeriodoInvalidoCaiParaDMenos1(t *testing.T) {
	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	dias := resolveTargetDays("", "2026-07-10", "2026-07-01", now) // end antes de start
	if len(dias) != 1 || dias[0].Format("2006-01-02") != "2026-07-09" {
		t.Fatalf("período inválido deveria cair para D-1, veio %v", dias)
	}
}

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func TestBuildPunchRecord_CorrelacionaPeloIdDaBatida(t *testing.T) {
	date := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	punch := domain.DailyPunch{
		CollaboratorID: 1,
		Date:           date,
		Marcacoes: []domain.PunchPair{
			{FonteDadosIDEntrada: intPtr(11911), FonteDadosIDSaida: intPtr(11914)},
		},
	}
	fonteDadosByID := map[int]domain.FonteDadoItem{
		11911: {ID: 11911, EquipamentoID: intPtr(6), Motivo: strPtr("Inclusão manual")},
	}

	rec, found := buildPunchRecord(5, 1, date, punch, fonteDadosByID)
	if !found {
		t.Fatalf("esperava correspondência encontrada")
	}
	if rec.TenantID != 5 || rec.CollaboratorID != 1 || !rec.Date.Equal(date) {
		t.Errorf("identidade do registro incorreta: %+v", rec)
	}
	if rec.EquipamentoID == nil || *rec.EquipamentoID != 6 {
		t.Errorf("EquipamentoID = %v, quer 6", rec.EquipamentoID)
	}
	if rec.Motivo == nil || *rec.Motivo != "Inclusão manual" {
		t.Errorf("Motivo = %v, quer \"Inclusão manual\"", rec.Motivo)
	}
}

func TestBuildPunchRecord_SemFonteDadosID(t *testing.T) {
	date := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	punch := domain.DailyPunch{Marcacoes: []domain.PunchPair{{}}}

	if _, found := buildPunchRecord(5, 1, date, punch, map[int]domain.FonteDadoItem{1: {}}); found {
		t.Errorf("não deveria encontrar correspondência sem id de fonte de dados na marcação")
	}
}

func TestBuildPunchRecord_IdSemCorrespondenciaNoPeriodo(t *testing.T) {
	date := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	punch := domain.DailyPunch{
		Marcacoes: []domain.PunchPair{{FonteDadosIDEntrada: intPtr(999)}},
	}

	if _, found := buildPunchRecord(5, 1, date, punch, map[int]domain.FonteDadoItem{1: {}}); found {
		t.Errorf("id sem correspondência no mapa de FonteDados não deveria ser encontrado")
	}
}

func TestBuildPunchRecord_SemFonteDadosDoPeriodo(t *testing.T) {
	date := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	punch := domain.DailyPunch{
		Marcacoes: []domain.PunchPair{{FonteDadosIDEntrada: intPtr(1)}},
	}

	if _, found := buildPunchRecord(5, 1, date, punch, nil); found {
		t.Errorf("sem FonteDados carregado para o período, não deveria produzir registro")
	}
}
