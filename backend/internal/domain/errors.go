package domain

import "fmt"

// ErrorKind classifica o tipo de erro de negócio, permitindo que a camada HTTP
// escolha o status adequado sem conhecer detalhes de infraestrutura.
type ErrorKind string

const (
	KindValidation ErrorKind = "VALIDATION" // entrada inválida -> 400
	KindNotFound   ErrorKind = "NOT_FOUND"  // recurso inexistente -> 404
	KindConflict   ErrorKind = "CONFLICT"   // violação de unicidade/estado -> 409
	KindForbidden  ErrorKind = "FORBIDDEN"  // sem permissão/vínculo com o recurso -> 403
	KindInternal   ErrorKind = "INTERNAL"   // falha inesperada -> 500
)

// AppError é o erro estruturado e contextualizado da aplicação.
//
//   - Kind:    a categoria (define o status HTTP).
//   - Op:      o ponto de origem (ex.: "tenantRepository.Update") — encadeável para
//     formar um "rastro" do erro, útil na verificação de problemas.
//   - Message: mensagem segura para exibir ao cliente/interface web.
//   - Details: informação adicional opcional (ex.: qual campo falhou na validação).
//   - Err:     o erro subjacente, preservado para log e para errors.Is/As.
type AppError struct {
	Kind    ErrorKind
	Op      string
	Message string
	Details string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s [%s] %s: %v", e.Op, e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("%s [%s] %s", e.Op, e.Kind, e.Message)
}

// Unwrap permite errors.Is/errors.As alcançarem o erro subjacente.
func (e *AppError) Unwrap() error { return e.Err }

// WithDetails devolve uma cópia do erro com detalhes adicionais preenchidos.
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// Construtores auxiliares (mantêm a criação enxuta nos repositórios e handlers).

func NewValidation(op, message string, err error) *AppError {
	return &AppError{Kind: KindValidation, Op: op, Message: message, Err: err}
}

func NewNotFound(op, message string, err error) *AppError {
	return &AppError{Kind: KindNotFound, Op: op, Message: message, Err: err}
}

func NewConflict(op, message string, err error) *AppError {
	return &AppError{Kind: KindConflict, Op: op, Message: message, Err: err}
}

func NewForbidden(op, message string, err error) *AppError {
	return &AppError{Kind: KindForbidden, Op: op, Message: message, Err: err}
}

func NewInternal(op, message string, err error) *AppError {
	return &AppError{Kind: KindInternal, Op: op, Message: message, Err: err}
}
