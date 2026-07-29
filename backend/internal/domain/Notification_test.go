package domain

import "testing"

func TestWhatsAppInstanceName(t *testing.T) {
	if got := WhatsAppInstanceName("tenant", 3); got != "tenant-3" {
		t.Errorf("WhatsAppInstanceName(tenant,3) = %q, quer %q", got, "tenant-3")
	}
	// Prefixo vazio cai no default "tenant" (evita instância sem prefixo).
	if got := WhatsAppInstanceName("", 7); got != "tenant-7" {
		t.Errorf("WhatsAppInstanceName(\"\",7) = %q, quer %q", got, "tenant-7")
	}
}
