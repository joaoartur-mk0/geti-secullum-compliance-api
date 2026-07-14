package secullum

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_PermiteBurstInicial(t *testing.T) {
	rl := newRateLimiter(3) // bucket começa cheio com 3 tokens
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := rl.wait(ctx); err != nil {
			t.Fatalf("wait #%d falhou inesperadamente: %v", i+1, err)
		}
	}
}

func TestRateLimiter_RespeitaContextoCancelado(t *testing.T) {
	rl := newRateLimiter(1)
	// Consome o único token disponível.
	if err := rl.wait(context.Background()); err != nil {
		t.Fatalf("primeiro wait falhou: %v", err)
	}
	// Bucket vazio: com contexto expirado, wait deve retornar erro (não travar).
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := rl.wait(ctx); err == nil {
		t.Fatalf("esperava erro de contexto com bucket vazio")
	}
}
