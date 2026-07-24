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
