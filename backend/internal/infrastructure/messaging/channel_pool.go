package messaging

import (
	"context"
	"fmt"
	"log"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type ChannelPool struct {
	conn     *amqp.Connection
	channels chan *amqp.Channel

	mu     sync.Mutex
	closed bool
}

// NewChannelPool abre `size` canais sobre a conexão informada e os coloca no pool.
// Se qualquer canal falhar ao ser aberto, os já criados são fechados e um erro é retornado.
func NewChannelPool(conn *amqp.Connection, size int) (*ChannelPool, error) {
	if conn == nil {
		return nil, fmt.Errorf("channel pool: conexão AMQP nula")
	}
	if size <= 0 {
		return nil, fmt.Errorf("channel pool: tamanho inválido (%d), deve ser > 0", size)
	}

	pool := &ChannelPool{
		conn:     conn,
		channels: make(chan *amqp.Channel, size),
	}

	for i := 0; i < size; i++ {
		ch, err := conn.Channel()
		if err != nil {
			_ = pool.Close() // fecha os canais já abertos antes de desistir
			return nil, fmt.Errorf("channel pool: falha ao abrir canal %d/%d: %w", i+1, size, err)
		}
		pool.channels <- ch
	}

	log.Printf("[ChannelPool] Pool inicializado com %d canais de publicação.", size)
	return pool, nil
}

// get empresta um canal do pool, bloqueando até um estar disponível ou o contexto
// expirar. Se o canal retirado estiver fechado, tenta recriá-lo na hora.
func (p *ChannelPool) get(ctx context.Context) (*amqp.Channel, error) {
	select {
	case ch := <-p.channels:
		if ch == nil {
			return nil, fmt.Errorf("channel pool: pool fechado")
		}
		if ch.IsClosed() {
			newCh, err := p.conn.Channel()
			if err != nil {
				return nil, fmt.Errorf("channel pool: canal quebrado e falha ao recriar: %w", err)
			}
			return newCh, nil
		}
		return ch, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("channel pool: sem canal disponível a tempo: %w", ctx.Err())
	}
}

// put devolve um canal ao pool. Um canal fechado é substituído por um novo para que o
// pool não encolha silenciosamente. Nunca bloqueia.
func (p *ChannelPool) put(ch *amqp.Channel) {
	if ch == nil {
		return
	}
	if ch.IsClosed() {
		replacement, err := p.conn.Channel()
		if err != nil {
			log.Printf("[ChannelPool] Não foi possível recriar canal devolvido: %v", err)
			return // o pool encolhe até uma devolução saudável; melhor do que enfileirar um canal morto
		}
		ch = replacement
	}
	select {
	case p.channels <- ch:
	default:
		// Pool cheio (não deveria acontecer com uso correto): fecha o excedente para não vazar.
		_ = ch.Close()
	}
}

// Publish empresta um canal, publica a mensagem na fila e devolve o canal ao pool.
// É seguro para uso concorrente — cada chamada usa um canal exclusivo enquanto publica.
func (p *ChannelPool) Publish(ctx context.Context, queue string, body []byte) error {
	ch, err := p.get(ctx)
	if err != nil {
		return err
	}
	defer p.put(ch)

	err = ch.PublishWithContext(
		ctx,
		"",    // exchange padrão
		queue, // routing key = nome da fila
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // mensagem sobrevive a restart do broker (fila é durable)
		},
	)
	if err != nil {
		return fmt.Errorf("channel pool: falha ao publicar na fila %q: %w", queue, err)
	}
	return nil
}

// Close fecha todos os canais do pool. Idempotente e seguro para chamar em defer.
func (p *ChannelPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	close(p.channels)

	var firstErr error
	for ch := range p.channels {
		if err := ch.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
