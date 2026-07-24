package usecase

import (
	"fmt"
	"log"

	"backend/internal/domain"
)

// SynchronizerService busca o espelho de colaboradores (funcionários e jornadas
// contratuais) na API SecullumWEB e o persiste localmente, para que o motor de
// auditoria não dependa de chamadas externas por colaborador durante o fechamento.
type SynchronizerService struct {
	tenantRepo  domain.TenantRepository
	collabRepo  domain.CollaboratorRepository
	secullumSvc domain.SecullumService
}

func NewSynchronizerService(
	tenantRepo domain.TenantRepository,
	collabRepo domain.CollaboratorRepository,
	secullumSvc domain.SecullumService,
) *SynchronizerService {
	return &SynchronizerService{
		tenantRepo:  tenantRepo,
		collabRepo:  collabRepo,
		secullumSvc: secullumSvc,
	}
}

// SyncTenant busca os colaboradores do tenant na Secullum e os salva localmente
// (upsert por secullum_id). Devolve quantos colaboradores foram sincronizados.
func (s *SynchronizerService) SyncTenant(tenantID int) (int, error) {
	tenant, err := s.tenantRepo.GetByID(tenantID)
	if err != nil {
		return 0, fmt.Errorf("buscar tenant: %w", err)
	}

	collaborators, err := s.secullumSvc.GetCollaborators(tenant)
	if err != nil {
		return 0, fmt.Errorf("buscar colaboradores na Secullum: %w", err)
	}

	for i := range collaborators {
		collaborators[i].TenantID = tenant.ID
	}

	// Poucas jornadas (HorarioNumero) distintas costumam cobrir centenas de
	// colaboradores. Buscamos cada uma UMA ÚNICA VEZ (cache local) em vez de uma
	// chamada por colaborador — essencial para não estourar o rate limit da Secullum
	// (100 req/min) numa sincronização com muitos funcionários.
	horarioCache := make(map[int][]domain.CollaboratorSchedule)
	for i := range collaborators {
		numero := collaborators[i].HorarioNumero
		if numero == 0 {
			continue // sem horário associado na Secullum
		}

		schedules, cached := horarioCache[numero]
		if !cached {
			fetched, err := s.secullumSvc.GetHorario(tenant, numero)
			if err != nil {
				// Falha ao buscar UMA jornada não deve abortar a sincronização inteira:
				// o colaborador é salvo sem jornada (fica no fallback de 8h no auditor),
				// e o problema fica visível no log para investigação.
				log.Printf("[Aviso Sync] Tenant %d: falha ao buscar horário nº %d: %v", tenant.ID, numero, err)
				horarioCache[numero] = nil
				continue
			}
			horarioCache[numero] = fetched
			schedules = fetched
		}

		collaborators[i].Schedules = schedules
	}

	if err := s.collabRepo.SaveAll(collaborators); err != nil {
		return 0, fmt.Errorf("salvar colaboradores: %w", err)
	}

	return len(collaborators), nil
}
