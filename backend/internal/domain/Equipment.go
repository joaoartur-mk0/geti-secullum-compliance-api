package domain

// Equipment é o espelho local de um aparelho (relógio de ponto) cadastrado na Secullum
// para o tenant. Sincronizado no cadastro do tenant e diariamente (mesmo fluxo da
// sincronização de colaboradores — ver usecase.SynchronizerService), sempre espelhado
// 1:1 com a Secullum: um aparelho removido lá é removido aqui também.
type Equipment struct {
	ID         int
	TenantID   int
	SecullumID int
	Descricao  string
	// EnderecoIP vem nulo para aparelhos que não expõem IP na Secullum (ex.: leitores
	// faciais sem rede própria).
	EnderecoIP *string
}

// EquipmentRepository é o contrato de persistência do espelho local de equipamentos.
type EquipmentRepository interface {
	// SaveAll espelha, para um tenant, exatamente a lista de equipamentos informada:
	// faz upsert por (tenant_id, secullum_id) e REMOVE (hard delete) qualquer
	// equipamento do tenant cujo secullum_id não esteja na lista — é isto que garante
	// o espelhamento 1:1 exigido (equipamento removido na Secullum some daqui também).
	SaveAll(tenantID int, equipments []Equipment) error
	GetByTenantID(tenantID int) ([]Equipment, error)
}
