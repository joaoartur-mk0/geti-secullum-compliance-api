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
