package handlers

import (
	"log"
	"time"

	"backend/internal/domain"
	"backend/internal/usecase"
)

// Este arquivo concentra o que o painel precisa para AUTOPREENCHER a tela de colaborador
// e a de advertência: o horário fixo cadastrado na Secullum e a filial em que a pessoa
// está lotada. Os dois formatos são compartilhados entre o endpoint de prefill e a
// listagem de ocorrências, para o frontend não ter de montar o mesmo objeto de dois
// jeitos diferentes.

// fixedScheduleResponse é o horário fixo do colaborador, um item por dia da grade.
type fixedScheduleResponse struct {
	DiaSemana    int    `json:"dia_semana"`
	Entrada1     string `json:"entrada_1"`
	Saida1       string `json:"saida_1"`
	Entrada2     string `json:"entrada_2"`
	Saida2       string `json:"saida_2"`
	CargaMinutos int    `json:"carga_minutos"`
}

// branchSummaryResponse é a filial resolvida, com a origem da informação.
//
// `source` existe porque a filial pode ter vindo do aparelho da batida (forte: a pessoa
// esteve lá) ou do nº de folha (cadastro de lotação). O painel usa isso para diferenciar
// "confirmado" de "inferido" antes de gravar uma advertência em nome daquela filial.
type branchSummaryResponse struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	ManagerName  string `json:"manager_name"`
	ManagerPhone string `json:"manager_phone"`
	Source       string `json:"source"` // "aparelho" | "numero_folha"
}

func toFixedSchedule(schedules []domain.CollaboratorSchedule) []fixedScheduleResponse {
	out := make([]fixedScheduleResponse, 0, len(schedules))
	for _, s := range schedules {
		out = append(out, fixedScheduleResponse{
			DiaSemana:    s.DiaSemana,
			Entrada1:     s.Entrada1,
			Saida1:       s.Saida1,
			Entrada2:     s.Entrada2,
			Saida2:       s.Saida2,
			CargaMinutos: s.CargaMinutos,
		})
	}
	return out
}

func toBranchSummary(branch *domain.Branch, source domain.BranchResolutionSource) *branchSummaryResponse {
	if branch == nil {
		return nil
	}
	return &branchSummaryResponse{
		ID:           branch.ID,
		Name:         branch.Name,
		ManagerName:  branch.ManagerName,
		ManagerPhone: branch.ManagerPhone,
		Source:       string(source),
	}
}

// punchesForBranchResolution busca as batidas de um dia só para extrair os aparelhos
// usados (EquipId), que resolvem a filial.
//
// É uma chamada externa: se falhar, NÃO derruba o autopreenchimento — a resolução apenas
// cai para o nº de folha, que é o caminho da maioria dos colaboradores mesmo. O erro fica
// no log para investigação, nunca silencioso.
func punchesForBranchResolution(
	secullumSvc domain.SecullumService,
	tenant *domain.Tenant,
	date time.Time,
) map[int]domain.DailyPunch {

	if secullumSvc == nil || tenant == nil {
		return nil
	}

	punches, err := secullumSvc.GetDailyPunches(tenant, date)
	if err != nil {
		log.Printf("[Aviso Filial] Tenant %d: falha ao buscar batidas de %s para resolver a filial pelo aparelho (segue pelo nº de folha): %v",
			tenant.ID, date.Format("2006-01-02"), err)
		return nil
	}

	out := make(map[int]domain.DailyPunch, len(punches))
	for _, p := range punches {
		out[p.CollaboratorID] = p
	}
	return out
}

// resolveBranchFor resolve a filial de um único colaborador, tolerando falha: filial
// desconhecida é um estado válido da interface, não um erro de requisição.
func resolveBranchFor(
	resolver *usecase.BranchResolverService,
	tenantID int,
	collab *domain.Collaborator,
	punch *domain.DailyPunch,
) *branchSummaryResponse {

	if resolver == nil {
		return nil
	}
	branch, source, err := resolver.Resolve(tenantID, collab, punch)
	if err != nil {
		log.Printf("[Aviso Filial] Tenant %d, colaborador %d: falha ao resolver filial: %v", tenantID, collab.SecullumID, err)
		return nil
	}
	return toBranchSummary(branch, source)
}
