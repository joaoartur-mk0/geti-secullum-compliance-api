package messaging

import (
	"context"
	"errors"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ErrDeliveriesClosed indica que o RabbitMQ fechou o canal de entregas de um worker —
// queda da conexão, reinício do broker ou canal encerrado pelo servidor.
var ErrDeliveriesClosed = errors.New("canal de entregas do RabbitMQ foi fechado")

// consumeLoop entrega cada mensagem de `msgs` para `handle` até o contexto ser cancelado
// (retorna nil) ou o canal de entregas ser fechado (retorna ErrDeliveriesClosed).
//
// A checagem de `ok` é obrigatória: em Go, ler de um canal fechado devolve o valor zero
// imediatamente e para sempre. Sem ela, quando a conexão com o RabbitMQ cai, o worker
// passa a receber amqp.Delivery vazios num laço apertado — cada um vira "payload inválido"
// e um Nack que falha, gravando log sem parar e consumindo CPU (incidente de 13/11/2026).
//
// Sair com erro, em vez de tentar reconectar aqui, é proposital: a conexão é compartilhada
// com os outros workers, com o pool de publicação e com o agendador. O chamador encerra o
// processo e o restart do container reabre tudo do zero, num estado consistente.
func consumeLoop(ctx context.Context, worker string, msgs <-chan amqp.Delivery, handle func(amqp.Delivery)) error {
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("worker de %s: %w", worker, ErrDeliveriesClosed)
			}
			handle(msg)
		case <-ctx.Done():
			log.Printf("[*] Desligando Worker de %s graciosamente...\n", worker)
			return nil
		}
	}
}
