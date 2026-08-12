package messaging

import (
	"strings"
	"testing"
	"time"

	"backend/internal/domain"
	"backend/internal/usecase"
)

var dia = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func occ(name, tipo, desc string) domain.Occurrence {
	return domain.Occurrence{
		CollaboratorName: name,
		Type:             tipo,
		Severity:         domain.SeverityCritical,
		Description:      desc,
	}
}

func TestBuildClosingSummaryMessage_SemOcorrencias(t *testing.T) {
	msg := buildClosingSummaryMessage("Acme", dia, usecase.ReconcileResult{})
	if !strings.Contains(msg, "Nenhuma inconsistência encontrada") {
		t.Errorf("esperava mensagem de dia limpo, veio:\n%s", msg)
	}
}

// O ponto da máquina de estados: reauditar um dia já conhecido não pode reenviar a lista
// inteira ao gestor como se fosse novidade.
func TestBuildClosingSummaryMessage_SemNovidadeNaoRepeteALista(t *testing.T) {
	msg := buildClosingSummaryMessage("Acme", dia, usecase.ReconcileResult{Unchanged: 12})

	if strings.Contains(msg, "⚠️") {
		t.Errorf("varredura sem novidade não deveria alarmar, veio:\n%s", msg)
	}
	if !strings.Contains(msg, "Nenhuma novidade") || !strings.Contains(msg, "12") {
		t.Errorf("esperava o resumo de 12 ocorrências já conhecidas, veio:\n%s", msg)
	}
}

func TestBuildClosingSummaryMessage_ReportaApenasOQueMudou(t *testing.T) {
	resumo := usecase.ReconcileResult{
		Created:   []domain.Occurrence{occ("Ana", "Almoço Reduzido", "intervalo de 20 minutos")},
		Updated:   []domain.Occurrence{occ("Bruno", "Interjornada Curta", "descanso de 8h")},
		Reopened:  []domain.Occurrence{occ("Carla", "Batida Esquecida", "número ímpar de marcações")},
		Resolved:  []domain.Occurrence{occ("Diego", "Hora Extra Excedente", "não se aplica mais")},
		Unchanged: 4,
	}

	msg := buildClosingSummaryMessage("Acme", dia, resumo)

	for _, esperado := range []string{"Ana", "Bruno", "Carla", "Diego", "01/07/2026", "Acme"} {
		if !strings.Contains(msg, esperado) {
			t.Errorf("esperava %q na mensagem, veio:\n%s", esperado, msg)
		}
	}
	// A descrição completa só vale para o que exige leitura; resolvida é só contagem.
	if strings.Contains(msg, "não se aplica mais") {
		t.Errorf("ocorrência resolvida não precisa da descrição completa, veio:\n%s", msg)
	}
	if !strings.Contains(msg, "4 ocorrência(s) já conhecida(s)") {
		t.Errorf("esperava a nota das inalteradas, veio:\n%s", msg)
	}
}

func TestResolveTargetDay(t *testing.T) {
	agora := time.Date(2026, 7, 10, 3, 0, 0, 0, time.UTC)

	// Sem data no evento: fechamento diário automático de D-1.
	if got := resolveTargetDay("", agora); got.Format("2006-01-02") != "2026-07-09" {
		t.Errorf("esperava D-1 (2026-07-09), veio %s", got.Format("2006-01-02"))
	}
	// Data escolhida pelo gestor.
	if got := resolveTargetDay("2026-06-15", agora); got.Format("2006-01-02") != "2026-06-15" {
		t.Errorf("esperava a data pedida, veio %s", got.Format("2006-01-02"))
	}
	// Data ilegível não pode derrubar a varredura noturna: cai em D-1.
	if got := resolveTargetDay("15/06/2026", agora); got.Format("2006-01-02") != "2026-07-09" {
		t.Errorf("data inválida deveria cair em D-1, veio %s", got.Format("2006-01-02"))
	}
}
