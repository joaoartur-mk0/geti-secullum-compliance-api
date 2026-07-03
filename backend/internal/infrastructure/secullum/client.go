package secullum

import (
	"backend/internal/domain"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// secullumPunchResponse é uma struct DTO (Data Transfer Object) auxiliar.
// Ela mapeia apenas os campos que importam do JSON gigante da Secullum.
type secullumPunchResponse struct {
	FuncionarioId int     `json:"FuncionarioId"`
	Data          string  `json:"Data"`
	Entrada1      *string `json:"Entrada1"`
	Saida1        *string `json:"Saida1"`
	Entrada2      *string `json:"Entrada2"`
	Saida2        *string `json:"Saida2"`
}

type secullumClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewSecullumClient(baseURL string) domain.SecullumService {
	return &secullumClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
	}
}

// GetDailyPunches extrai as batidas do dia e as "limpa" para o domínio
func (c *secullumClient) GetDailyPunches(tenant *domain.Tenant, date time.Time) ([]domain.DailyPunch, error) {
	// NOTA: Em produção, você precisa implementar a lógica que checa se o token atual
	// do Tenant expirou (duração de 1h) e renová-lo antes desta requisição, além
	// de respeitar o limite de 100 requisições/minuto.

	endpoint := fmt.Sprintf("%s/api/batidas?data=%s", c.baseURL, date.Format("2006-01-02"))

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Injeta o token de autenticação do Tenant logado
	req.Header.Set("Authorization", "Bearer "+tenant.SecullumToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro na API Secullum: status %d", resp.StatusCode)
	}

	var rawResponses []secullumPunchResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawResponses); err != nil {
		return nil, err
	}

	// Mapeamento: DTO sujo -> Entidade pura de Domínio
	var domainPunches []domain.DailyPunch
	for _, raw := range rawResponses {
		parsedDate, _ := time.Parse("2006-01-02T15:04:05", raw.Data)

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
