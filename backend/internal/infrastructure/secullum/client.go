package secullum

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"backend/internal/domain"
)

// headerBancoSelecionado identifica, por requisição, qual banco de dados da Secullum
// deve ser consultado. A credencial de autenticação é GLOBAL (uma só para todos os
// tenants); o que muda entre tenants é este id de banco.
const headerBancoSelecionado = "secullumidbancoselecionado"

// requestTimeout limita o tempo total de cada operação (espera no rate limiter +
// eventual renovação de token + a própria requisição HTTP).
const requestTimeout = 30 * time.Second

// Config reúne os parâmetros GLOBAIS do client (mesma credencial para todos os tenants).
type Config struct {
	BaseURL              string // ex.: https://pontoweb.secullum.com.br
	AuthURL              string // endpoint de autenticação (username/password -> access_token)
	Username             string
	Password             string
	StaticToken          string // opcional: token fixo (pula autenticação/refresh), útil em testes
	MaxRequestsPerMinute int    // default 100
}

// secullumPunchResponse mapeia apenas os campos de batida que importam do JSON da Secullum.
type secullumPunchResponse struct {
	FuncionarioId int     `json:"FuncionarioId"`
	Data          string  `json:"Data"`
	Entrada1      *string `json:"Entrada1"`
	Saida1        *string `json:"Saida1"`
	Entrada2      *string `json:"Entrada2"`
	Saida2        *string `json:"Saida2"`
}

// secullumFuncionarioResponse mapeia identidade + descrição da jornada do colaborador.
type secullumFuncionarioResponse struct {
	Id      int    `json:"Id"`
	Cpf     string `json:"Cpf"`
	Celular string `json:"Celular"`
	Horario struct {
		Descricao string `json:"Descricao"`
	} `json:"Horario"`
}

type secullumClient struct {
	httpClient *http.Client
	baseURL    string
	tokens     *tokenManager
	limiter    *rateLimiter
}

// NewSecullumClient monta o client com renovação de token e rate limit globais.
func NewSecullumClient(cfg Config) domain.SecullumService {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	return &secullumClient{
		httpClient: httpClient,
		baseURL:    cfg.BaseURL,
		tokens: &tokenManager{
			httpClient:  httpClient,
			authURL:     cfg.AuthURL,
			username:    cfg.Username,
			password:    cfg.Password,
			staticToken: cfg.StaticToken,
		},
		limiter: newRateLimiter(cfg.MaxRequestsPerMinute),
	}
}

// do respeita o rate limit, garante um token válido e executa a requisição já com os
// cabeçalhos de autenticação e de banco selecionado.
func (c *secullumClient) do(ctx context.Context, method, endpoint string, tenant *domain.Tenant) (*http.Response, error) {
	// 1. Rate limit (100 req/min): aguarda um token do bucket.
	if err := c.limiter.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	// 2. Token global válido (renova automaticamente se expirado).
	token, err := c.tokens.get(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(headerBancoSelecionado, strconv.Itoa(tenant.SecullumDatabaseID))

	return c.httpClient.Do(req)
}

// GetDailyPunches extrai as batidas do dia e as "limpa" para o domínio.
func (c *secullumClient) GetDailyPunches(tenant *domain.Tenant, date time.Time) ([]domain.DailyPunch, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/api/batidas?data=%s", c.baseURL, date.Format("2006-01-02"))

	resp, err := c.do(ctx, http.MethodGet, endpoint, tenant)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro na API Secullum (batidas): status %d", resp.StatusCode)
	}

	var rawResponses []secullumPunchResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawResponses); err != nil {
		return nil, err
	}

	// Mapeamento: DTO sujo -> Entidade pura de Domínio
	var domainPunches []domain.DailyPunch
	for _, raw := range rawResponses {
		parsedDate, err := time.Parse("2006-01-02T15:04:05", raw.Data)
		if err != nil {
			// Registro com data ilegível é ignorado para não corromper a auditoria,
			// mas o problema fica visível no log para investigação (não silenciado).
			log.Printf("[Aviso Secullum] Ignorando batida do funcionário %d: data inválida %q: %v",
				raw.FuncionarioId, raw.Data, err)
			continue
		}

		domainPunches = append(domainPunches, domain.DailyPunch{
			CollaboratorID: raw.FuncionarioId,
			Date:           parsedDate,
			Entrada1:       raw.Entrada1,
			Saida1:         raw.Saida1,
			Entrada2:       raw.Entrada2,
			Saida2:         raw.Saida2,
		})
	}

	return domainPunches, nil
}

// GetCollaborators busca os funcionários do tenant, mapeia identidade e parseia a
// jornada contratual (matriz de turnos) a partir da descrição textual do horário.
func (c *secullumClient) GetCollaborators(tenant *domain.Tenant) ([]domain.Collaborator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/api/funcionarios", c.baseURL)

	resp, err := c.do(ctx, http.MethodGet, endpoint, tenant)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro na API Secullum (funcionarios): status %d", resp.StatusCode)
	}

	var rawResponses []secullumFuncionarioResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawResponses); err != nil {
		return nil, err
	}

	collaborators := make([]domain.Collaborator, 0, len(rawResponses))
	for _, raw := range rawResponses {
		collab := domain.Collaborator{
			TenantID:   tenant.ID,
			SecullumID: raw.Id,
			Cpf:        raw.Cpf,
			Celular:    raw.Celular,
		}

		// Jornada contratual: parseia a descrição textual, se interpretável.
		if sch, ok := parseHorario(raw.Horario.Descricao); ok {
			collab.Schedules = []domain.CollaboratorSchedule{sch}
		} else if raw.Horario.Descricao != "" {
			log.Printf("[Aviso Secullum] Jornada não interpretável do funcionário %d: %q",
				raw.Id, raw.Horario.Descricao)
		}

		collaborators = append(collaborators, collab)
	}

	return collaborators, nil
}
