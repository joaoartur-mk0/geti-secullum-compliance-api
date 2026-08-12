package usecase

import (
	"backend/internal/domain"
)

// BranchResolverService descobre em qual filial um colaborador estava.
//
// A Secullum não tem o conceito de filial: ela conhece a empresa, os funcionários e os
// aparelhos. A ligação é feita aqui, em duas tentativas, nesta ordem:
//
//  1. pelo APARELHO em que a batida do dia foi registrada (`EquipId`) — é a informação
//     mais forte, porque diz onde a pessoa fisicamente esteve naquele dia;
//  2. pelo Nº DE FOLHA do colaborador, cadastrado na filial — é o cadastro de lotação, e
//     cobre o caso majoritário nos dados reais desta operação, em que a batida vem do
//     app/web e chega sem `EquipId`.
//
// Não achar filial não é erro: aparelho recém-instalado e colaborador ainda sem lotação
// são situações normais. Nesse caso devolve (nil, BranchUnresolved) e a interface mostra
// o campo vazio para o gestor preencher.
type BranchResolverService struct {
	branchRepo domain.BranchRepository
}

func NewBranchResolverService(branchRepo domain.BranchRepository) *BranchResolverService {
	return &BranchResolverService{branchRepo: branchRepo}
}

// Resolve devolve a filial e COMO ela foi determinada. `punch` é opcional (nil quando não
// há batida no dia consultado): sem ele, a resolução cai direto no nº de folha.
func (s *BranchResolverService) Resolve(
	tenantID int,
	collab *domain.Collaborator,
	punch *domain.DailyPunch,
) (*domain.Branch, domain.BranchResolutionSource, error) {

	if punch != nil {
		for _, equipID := range punch.EquipIDs {
			branch, err := s.branchRepo.FindByDevice(tenantID, equipID)
			if err != nil {
				return nil, domain.BranchUnresolved, err
			}
			if branch != nil {
				return branch, domain.BranchFromDevice, nil
			}
		}
	}

	if collab != nil && collab.NumeroFolha != "" {
		branch, err := s.branchRepo.FindByPayrollNumber(tenantID, collab.NumeroFolha)
		if err != nil {
			return nil, domain.BranchUnresolved, err
		}
		if branch != nil {
			return branch, domain.BranchFromPayrollNumber, nil
		}
	}

	return nil, domain.BranchUnresolved, nil
}

// ResolveMany resolve a filial de vários colaboradores de uma vez, devolvendo o resultado
// indexado pelo id Secullum do colaborador.
//
// Existe para a listagem de ocorrências: resolver um a um faria uma consulta por linha.
// Aqui as filiais do tenant são carregadas UMA vez e os índices (aparelho e nº de folha)
// são montados em memória.
func (s *BranchResolverService) ResolveMany(
	tenantID int,
	collaborators []domain.Collaborator,
	punches map[int]domain.DailyPunch,
) (map[int]BranchResolution, error) {

	branches, err := s.branchRepo.ListByTenant(tenantID)
	if err != nil {
		return nil, err
	}

	byDevice := make(map[int]*domain.Branch)
	byPayroll := make(map[string]*domain.Branch)
	for i := range branches {
		b := &branches[i]
		for _, d := range b.Devices {
			byDevice[d.SecullumEquipID] = b
		}
		for _, p := range b.PayrollNumbers {
			byPayroll[p.Numero] = b
		}
	}

	out := make(map[int]BranchResolution, len(collaborators))
	for i := range collaborators {
		collab := collaborators[i]

		if punch, ok := punches[collab.SecullumID]; ok {
			resolved := false
			for _, equipID := range punch.EquipIDs {
				if b, found := byDevice[equipID]; found {
					out[collab.SecullumID] = BranchResolution{Branch: b, Source: domain.BranchFromDevice}
					resolved = true
					break
				}
			}
			if resolved {
				continue
			}
		}

		if b, found := byPayroll[collab.NumeroFolha]; found && collab.NumeroFolha != "" {
			out[collab.SecullumID] = BranchResolution{Branch: b, Source: domain.BranchFromPayrollNumber}
		}
	}
	return out, nil
}

// HasDevices diz se o tenant tem ao menos um aparelho cadastrado.
//
// Serve para decidir se vale a pena buscar as batidas do dia na Secullum: sem nenhum
// aparelho vinculado, a resolução cairia no nº de folha de qualquer maneira, e a chamada
// externa (com rate limit e até 30s de timeout) seria puro desperdício na listagem.
func (s *BranchResolverService) HasDevices(tenantID int) bool {
	branches, err := s.branchRepo.ListByTenant(tenantID)
	if err != nil {
		return false
	}
	for _, b := range branches {
		if len(b.Devices) > 0 {
			return true
		}
	}
	return false
}

// BranchResolution é a filial encontrada mais a origem da informação.
type BranchResolution struct {
	Branch *domain.Branch
	Source domain.BranchResolutionSource
}
