package secullum

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// secullumPunchResponse mapeia os campos de batida que importam do JSON da Secullum.
//
// Os campos Memoria* trazem a jornada que a Secullum efetivamente alocou para o
// funcionário NAQUELE dia — já resolvendo escala, horário alternativo, feriado e trocas
// pontuais. É daí que sai a carga esperada da auditoria; a grade semanal do horário
// (endpoint Horarios) não serve, pois diverge do que foi aplicado no dia.
type secullumPunchResponse struct {
	FuncionarioId int     `json:"FuncionarioId"`
	Data          string  `json:"Data"`
	Entrada1      *string `json:"Entrada1"`
	Saida1        *string `json:"Saida1"`
	Entrada2      *string `json:"Entrada2"`
	Saida2        *string `json:"Saida2"`
	Entrada3      *string `json:"Entrada3"`
	Saida3        *string `json:"Saida3"`
	Entrada4      *string `json:"Entrada4"`
	Saida4        *string `json:"Saida4"`
	Entrada5      *string `json:"Entrada5"`
	Saida5        *string `json:"Saida5"`

	MemoriaEntrada1 *string `json:"MemoriaEntrada1"`
	MemoriaSaida1   *string `json:"MemoriaSaida1"`
	MemoriaEntrada2 *string `json:"MemoriaEntrada2"`
	MemoriaSaida2   *string `json:"MemoriaSaida2"`
	MemoriaEntrada3 *string `json:"MemoriaEntrada3"`
	MemoriaSaida3   *string `json:"MemoriaSaida3"`
	MemoriaEntrada4 *string `json:"MemoriaEntrada4"`
	MemoriaSaida4   *string `json:"MemoriaSaida4"`
	MemoriaEntrada5 *string `json:"MemoriaEntrada5"`
	MemoriaSaida5   *string `json:"MemoriaSaida5"`

	Folga  bool `json:"Folga"`
	Neutro bool `json:"Neutro"`
}

// marcacoes devolve os 5 pares de batidas reais, preservando as posições vazias (uma
// contagem ímpar de marcações é o sinal da regra de batida esquecida).
func (r secullumPunchResponse) marcacoes() []domain.PunchPair {
	return []domain.PunchPair{
		{Entrada: normalizeTime(r.Entrada1), Saida: normalizeTime(r.Saida1)},
		{Entrada: normalizeTime(r.Entrada2), Saida: normalizeTime(r.Saida2)},
		{Entrada: normalizeTime(r.Entrada3), Saida: normalizeTime(r.Saida3)},
		{Entrada: normalizeTime(r.Entrada4), Saida: normalizeTime(r.Saida4)},
		{Entrada: normalizeTime(r.Entrada5), Saida: normalizeTime(r.Saida5)},
	}
}

// previstas devolve os pares da jornada alocada para o dia (campos Memoria*).
func (r secullumPunchResponse) previstas() []domain.PunchPair {
	return []domain.PunchPair{
		{Entrada: normalizeTime(r.MemoriaEntrada1), Saida: normalizeTime(r.MemoriaSaida1)},
		{Entrada: normalizeTime(r.MemoriaEntrada2), Saida: normalizeTime(r.MemoriaSaida2)},
		{Entrada: normalizeTime(r.MemoriaEntrada3), Saida: normalizeTime(r.MemoriaSaida3)},
		{Entrada: normalizeTime(r.MemoriaEntrada4), Saida: normalizeTime(r.MemoriaSaida4)},
		{Entrada: normalizeTime(r.MemoriaEntrada5), Saida: normalizeTime(r.MemoriaSaida5)},
	}
}

// secullumFuncionarioResponse mapeia identidade + o número do horário do colaborador.
// O número identifica a jornada contratual, buscada à parte via GetHorario (o endpoint
// de funcionários não traz os horários por dia — só a descrição/número do horário).
type secullumFuncionarioResponse struct {
	Id      int    `json:"Id"`
	Nome    string `json:"Nome"`
	Cpf     string `json:"Cpf"`
	Celular string `json:"Celular"`
	Horario struct {
		Numero int `json:"Numero"`
	} `json:"Horario"`
}

// secullumHorarioResponse mapeia a jornada contratual (por dia da semana) de um
// horário da Secullum. A resposta do endpoint é um array (normalmente com 1 item).
type secullumHorarioResponse struct {
	Id   int `json:"Id"`
	Dias []struct {
		DiaSemana int     `json:"DiaSemana"`
		Entrada1  *string `json:"Entrada1"`
		Saida1    *string `json:"Saida1"`
		Entrada2  *string `json:"Entrada2"`
		Saida2    *string `json:"Saida2"`
		Carga     int     `json:"Carga"` // carga diária contratual, em minutos
	} `json:"Dias"`
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
//
// A Secullum pode invalidar um token ANTES do nosso TTL local de 55min (ex.: um novo
// login na mesma conta derruba o token anterior). Por isso, ao receber 401 no modo
// login, descartamos o token em cache, autenticamos de novo e repetimos a requisição
// UMA única vez. Se o 401 persistir (credenciais realmente inválidas), ele é devolvido
// ao chamador. Com token estático não há o que renovar — o 401 é devolvido direto.
func (c *secullumClient) do(ctx context.Context, method, endpoint string, tenant *domain.Tenant) (*http.Response, error) {
	// Rate limit (100 req/min): aguarda um token do bucket.
	if err := c.limiter.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	resp, err := c.doOnce(ctx, method, endpoint, tenant)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && c.tokens.canRefresh() {
		resp.Body.Close()
		log.Printf("[Secullum] 401 recebido (token invalidado no servidor?) — renovando token e repetindo a requisição uma vez...")
		c.tokens.invalidate()
		return c.doOnce(ctx, method, endpoint, tenant)
	}

	return resp, nil
}

// doOnce executa uma única tentativa da requisição, obtendo um token válido do manager.
func (c *secullumClient) doOnce(ctx context.Context, method, endpoint string, tenant *domain.Tenant) (*http.Response, error) {
	token, err := c.tokens.get(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(headerBancoSelecionado, strconv.Itoa(tenant.SecullumDatabaseID))

	return c.httpClient.Do(req)
}

// statusError monta um erro incluindo um trecho do corpo da resposta da Secullum,
// para que o motivo real (token expirado, banco inválido, etc.) apareça no log.
func statusError(area string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("erro na API Secullum (%s): status %d: %s", area, resp.StatusCode, string(body))
}

// isClock indica se o valor é um horário "HH:MM" válido.
func isClock(v *string) bool {
	if v == nil {
		return false
	}
	_, err := time.Parse("15:04", *v)
	return err == nil
}

// normalizeTime mantém o horário só se for um "HH:MM" válido; caso contrário (nil ou
// marcador de abono como "INSS"), devolve nil — ou seja, "sem batida".
func normalizeTime(v *string) *string {
	if isClock(v) {
		return v
	}
	return nil
}

// abonoMarker devolve o primeiro valor não-nulo que NÃO seja um horário (o marcador de
// abono/afastamento), ou "" se todos os campos forem horário/nulo.
func abonoMarker(vals ...*string) string {
	for _, v := range vals {
		if v != nil && !isClock(v) {
			return *v
		}
	}
	return ""
}

// GetDailyPunches extrai as batidas do dia e as "limpa" para o domínio.
func (c *secullumClient) GetDailyPunches(tenant *domain.Tenant, date time.Time) ([]domain.DailyPunch, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/IntegracaoExterna/Batidas?DataInicio=%s&DataFim=%s", c.baseURL, date.Format("2006-01-02"), date.Format("2006-01-02"))

	resp, err := c.do(ctx, http.MethodGet, endpoint, tenant)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, statusError("batidas", resp)
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

		// Campos de batida podem trazer um marcador de abono/afastamento (ex.: "INSS",
		// "FOLGA", "FÉRIAS") em vez de um horário. Isso NÃO é erro de dado: é um dia de
		// ausência legítima. Registramos como informação e tratamos como "sem batida".
		if marker := abonoMarker(raw.Entrada1, raw.Saida1, raw.Entrada2, raw.Saida2,
			raw.Entrada3, raw.Saida3, raw.Entrada4, raw.Saida4, raw.Entrada5, raw.Saida5); marker != "" {
			log.Printf("[Info Secullum] Funcionário %d em %s: marcador %q (abono/afastamento) — tratado como ausência.",
				raw.FuncionarioId, parsedDate.Format("2006-01-02"), marker)
		}

		domainPunches = append(domainPunches, domain.DailyPunch{
			CollaboratorID: raw.FuncionarioId,
			Date:           parsedDate,
			Marcacoes:      raw.marcacoes(),
			Previstas:      raw.previstas(),
			Folga:          raw.Folga,
			Neutro:         raw.Neutro,
		})
	}

	return domainPunches, nil
}

// GetCollaborators busca os funcionários do tenant e mapeia a identidade. A jornada
// contratual (matriz de turnos) NÃO vem aqui — o endpoint de funcionários só traz o
// número do horário; use GetHorario para buscar os dias/horários de cada um.
func (c *secullumClient) GetCollaborators(tenant *domain.Tenant) ([]domain.Collaborator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/IntegracaoExterna/Funcionarios", c.baseURL)

	resp, err := c.do(ctx, http.MethodGet, endpoint, tenant)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, statusError("funcionarios", resp)
	}

	var rawResponses []secullumFuncionarioResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawResponses); err != nil {
		return nil, err
	}

	collaborators := make([]domain.Collaborator, 0, len(rawResponses))
	for _, raw := range rawResponses {
		collaborators = append(collaborators, domain.Collaborator{
			TenantID:      tenant.ID,
			SecullumID:    raw.Id,
			Name:          raw.Nome,
			Cpf:           raw.Cpf,
			Celular:       raw.Celular,
			HorarioNumero: raw.Horario.Numero,
		})
	}

	return collaborators, nil
}

// GetHorario busca a jornada contratual (um registro por dia da semana) associada ao
// número de horário informado. Poucos horários distintos existem por tenant — o
// Synchronizer deve deduplicar e chamar este método uma vez por número, não por
// colaborador, para não estourar o rate limit da Secullum.
func (c *secullumClient) GetHorario(tenant *domain.Tenant, numero int) ([]domain.CollaboratorSchedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/IntegracaoExterna/Horarios?numero=%d", c.baseURL, numero)

	resp, err := c.do(ctx, http.MethodGet, endpoint, tenant)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, statusError("horarios", resp)
	}

	var rawResponses []secullumHorarioResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawResponses); err != nil {
		return nil, err
	}
	if len(rawResponses) == 0 {
		return nil, nil
	}

	schedules := make([]domain.CollaboratorSchedule, 0, len(rawResponses[0].Dias))
	for _, dia := range rawResponses[0].Dias {
		sch := domain.CollaboratorSchedule{
			DiaSemana:    dia.DiaSemana,
			CargaMinutos: dia.Carga,
		}
		if isClock(dia.Entrada1) {
			sch.Entrada1 = *dia.Entrada1
		}
		if isClock(dia.Saida1) {
			sch.Saida1 = *dia.Saida1
		}
		if isClock(dia.Entrada2) {
			sch.Entrada2 = *dia.Entrada2
		}
		if isClock(dia.Saida2) {
			sch.Saida2 = *dia.Saida2
		}
		schedules = append(schedules, sch)
	}

	return schedules, nil
}
