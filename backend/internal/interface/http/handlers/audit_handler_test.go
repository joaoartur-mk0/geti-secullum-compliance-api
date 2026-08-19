package handlers

import (
	"testing"
	"time"
)

var agora = time.Date(2026, 7, 10, 9, 30, 0, 0, time.UTC)

func TestResolveDate_SemDataAuditaDMenos1(t *testing.T) {
	got, err := TriggerRequest{TenantID: 1}.resolveDate("teste", agora)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-09" {
		t.Errorf("esperava o fechamento de D-1, veio %s", got.Format("2006-01-02"))
	}
}

func TestResolveDate_DiaEscolhido(t *testing.T) {
	got, err := TriggerRequest{TenantID: 1, Date: "2026-06-15"}.resolveDate("teste", agora)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.Format("2006-01-02") != "2026-06-15" {
		t.Errorf("esperava 2026-06-15, veio %s", got.Format("2006-01-02"))
	}
}

func TestResolveDate_FormatoInvalido(t *testing.T) {
	if _, err := (TriggerRequest{TenantID: 1, Date: "15/06/2026"}).resolveDate("teste", agora); err == nil {
		t.Error("data fora do formato YYYY-MM-DD deveria ser recusada")
	}
}

// Auditar o dia corrente como fechamento daria falso positivo em toda jornada em
// andamento (a regra de contagem ímpar de batidas, por exemplo).
func TestResolveDate_HojeEFuturoRecusados(t *testing.T) {
	for _, data := range []string{"2026-07-10", "2026-07-11"} {
		if _, err := (TriggerRequest{TenantID: 1, Date: data}).resolveDate("teste", agora); err == nil {
			t.Errorf("data %s não deveria ser aceita (dia não encerrado)", data)
		}
	}
}

func TestResolvePeriod_IntervaloValido(t *testing.T) {
	req := TriggerRequest{TenantID: 1, StartDate: "2026-07-01", EndDate: "2026-07-07"}
	start, end, err := req.resolvePeriod("teste", agora)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if start.Format("2006-01-02") != "2026-07-01" || end.Format("2006-01-02") != "2026-07-07" {
		t.Errorf("esperava 2026-07-01..2026-07-07, veio %s..%s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}
}

func TestResolvePeriod_FaltandoUmaDasDatas(t *testing.T) {
	if _, _, err := (TriggerRequest{TenantID: 1, StartDate: "2026-07-01"}).resolvePeriod("teste", agora); err == nil {
		t.Error("start_date sem end_date deveria ser recusado")
	}
	if _, _, err := (TriggerRequest{TenantID: 1, EndDate: "2026-07-01"}).resolvePeriod("teste", agora); err == nil {
		t.Error("end_date sem start_date deveria ser recusado")
	}
}

func TestResolvePeriod_EndAntesDeStart(t *testing.T) {
	req := TriggerRequest{TenantID: 1, StartDate: "2026-07-10", EndDate: "2026-07-01"}
	if _, _, err := req.resolvePeriod("teste", agora); err == nil {
		t.Error("end_date anterior a start_date deveria ser recusado")
	}
}

func TestResolvePeriod_PeriodoNaoEncerrado(t *testing.T) {
	req := TriggerRequest{TenantID: 1, StartDate: "2026-07-01", EndDate: "2026-07-10"} // hoje é 2026-07-10
	if _, _, err := req.resolvePeriod("teste", agora); err == nil {
		t.Error("período cujo end_date é hoje ainda não está encerrado, deveria ser recusado")
	}
}

func TestResolvePeriod_MuitoLongoRecusado(t *testing.T) {
	req := TriggerRequest{TenantID: 1, StartDate: "2026-01-01", EndDate: "2026-06-01"} // > 62 dias
	if _, _, err := req.resolvePeriod("teste", agora); err == nil {
		t.Error("período acima do limite deveria ser recusado")
	}
}

func TestIsRange(t *testing.T) {
	if (TriggerRequest{TenantID: 1}).isRange() {
		t.Error("requisição sem datas não deveria ser de período")
	}
	if !(TriggerRequest{TenantID: 1, StartDate: "2026-07-01", EndDate: "2026-07-07"}).isRange() {
		t.Error("requisição com start_date/end_date deveria ser de período")
	}
}
