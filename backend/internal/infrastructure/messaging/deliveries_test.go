package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// runLoop executa consumeLoop em background e falha o teste se ele não retornar a tempo —
// é exatamente o sintoma do bug de 13/11 (laço que nunca termina).
func runLoop(t *testing.T, ctx context.Context, msgs <-chan amqp.Delivery, handle func(amqp.Delivery)) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- consumeLoop(ctx, "Teste", msgs, handle) }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("consumeLoop não retornou: laço preso")
		return nil
	}
}

func TestConsumeLoop_CanalFechadoRetornaErroSemProcessarVazio(t *testing.T) {
	msgs := make(chan amqp.Delivery)
	close(msgs)

	chamadas := 0
	err := runLoop(t, context.Background(), msgs, func(amqp.Delivery) { chamadas++ })

	if !errors.Is(err, ErrDeliveriesClosed) {
		t.Fatalf("esperava ErrDeliveriesClosed, veio %v", err)
	}
	if chamadas != 0 {
		t.Errorf("canal fechado não deve gerar mensagens vazias para o handler; houve %d chamada(s)", chamadas)
	}
}

func TestConsumeLoop_ProcessaMensagensAteOFechamento(t *testing.T) {
	msgs := make(chan amqp.Delivery, 2)
	msgs <- amqp.Delivery{Body: []byte(`{"tenant_id":1}`)}
	msgs <- amqp.Delivery{Body: []byte(`{"tenant_id":2}`)}
	close(msgs)

	var corpos []string
	err := runLoop(t, context.Background(), msgs, func(d amqp.Delivery) { corpos = append(corpos, string(d.Body)) })

	if !errors.Is(err, ErrDeliveriesClosed) {
		t.Fatalf("esperava ErrDeliveriesClosed, veio %v", err)
	}
	if len(corpos) != 2 {
		t.Fatalf("esperava 2 mensagens processadas antes do fechamento, veio %d", len(corpos))
	}
}

func TestConsumeLoop_ContextoCanceladoRetornaNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runLoop(t, ctx, make(chan amqp.Delivery), func(amqp.Delivery) {
		t.Error("handler não deveria ser chamado")
	})
	if err != nil {
		t.Fatalf("cancelamento do contexto deve encerrar sem erro, veio %v", err)
	}
}
