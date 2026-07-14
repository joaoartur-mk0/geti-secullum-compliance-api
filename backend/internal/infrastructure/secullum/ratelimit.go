package secullum

import (
	"context"
	"time"
)

// rateLimiter é um token bucket simples (stdlib) que limita o número de requisições
// por minuto à API Secullum (limite documentado: 100 req/min).
//
// Funcionamento: o bucket começa cheio (burst = maxPerMinute) e um ticker repõe um
// token a cada (1min / maxPerMinute). Cada requisição consome um token via wait();
// se o bucket estiver vazio, wait() bloqueia até haver token ou o contexto expirar.
type rateLimiter struct {
	tokens chan struct{}
}

// newRateLimiter cria o limitador e inicia o goroutine de reposição. Destinado a um
// client singleton de vida longa (o goroutine roda enquanto o processo existir).
func newRateLimiter(maxPerMinute int) *rateLimiter {
	if maxPerMinute <= 0 {
		maxPerMinute = 100
	}

	rl := &rateLimiter{tokens: make(chan struct{}, maxPerMinute)}
	for i := 0; i < maxPerMinute; i++ {
		rl.tokens <- struct{}{}
	}

	interval := time.Minute / time.Duration(maxPerMinute)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case rl.tokens <- struct{}{}: // repõe um token
			default: // bucket cheio: descarta a reposição
			}
		}
	}()

	return rl
}

// wait consome um token, bloqueando até haver um disponível ou o contexto ser cancelado.
func (rl *rateLimiter) wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
