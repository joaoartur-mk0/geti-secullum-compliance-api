package domain

import "testing"

func TestWarningStatus_FluxoPermitido(t *testing.T) {
	cases := []struct {
		from, to WarningStatus
		allowed  bool
	}{
		{WarningDraft, WarningSent, true},
		{WarningSent, WarningSigned, true},
		{WarningDraft, WarningDraft, true},   // idempotente
		{WarningSigned, WarningSigned, true}, // idempotente
		{WarningDraft, WarningSigned, false}, // não se pula a entrega
		{WarningSent, WarningDraft, false},   // não se "desenvia"
		{WarningSigned, WarningSent, false},  // não se "desassina"
	}
	for _, c := range cases {
		if got := c.from.CanTransitionTo(c.to); got != c.allowed {
			t.Errorf("%s -> %s: got %v, want %v", c.from, c.to, got, c.allowed)
		}
	}
}

func TestWarningStatus_Valid(t *testing.T) {
	for _, s := range []WarningStatus{WarningDraft, WarningSent, WarningSigned} {
		if !s.Valid() {
			t.Errorf("%q deveria ser um status válido", s)
		}
	}
	if WarningStatus("cancelada").Valid() {
		t.Error("status desconhecido não deveria ser aceito")
	}
}
