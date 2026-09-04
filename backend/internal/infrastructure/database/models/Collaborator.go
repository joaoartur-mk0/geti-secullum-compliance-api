package models

import "time"

type Collaborator struct {
	ID         int    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID   int    `gorm:"not null;uniqueIndex:idx_collaborator_tenant_secullum" json:"tenant_id"`
	SecullumID int    `gorm:"not null;uniqueIndex:idx_collaborator_tenant_secullum" json:"secullum_id"`
	Name       string `gorm:"type:varchar(255)" json:"name"`
	Cpf        string `gorm:"type:varchar(20)" json:"cpf"`
	Celular    string `gorm:"type:varchar(20)" json:"celular"`
	// NumeroFolha vem da Secullum como string (pode ter zeros à esquerda). Indexado
	// porque é chave de busca na resolução de filial.
	NumeroFolha string `gorm:"type:varchar(40);index" json:"numero_folha"`

	Admissao *time.Time `gorm:"type:date" json:"admissao"`
	Demissao *time.Time `gorm:"type:date" json:"demissao"`
	// Demitido é derivada de Demissao preenchida a cada sincronização — indexada porque
	// é o filtro de GET /collaborators (só ativos).
	Demitido bool `gorm:"not null;default:false;index" json:"demitido"`

	// Departamento, Função e Empresa vêm crus da Secullum, sem normalização — ver
	// domain.Collaborator. Indexados porque alimentam filtro por tenant.
	DepartamentoID   *int   `gorm:"index:idx_collaborator_departamento" json:"departamento_id"`
	Departamento     string `gorm:"type:varchar(255)" json:"departamento"`
	FuncaoID         *int   `gorm:"index:idx_collaborator_funcao" json:"funcao_id"`
	Funcao           string `gorm:"type:varchar(255)" json:"funcao"`
	EmpresaID        *int   `gorm:"index:idx_collaborator_empresa" json:"empresa_id"`
	Empresa          string `gorm:"type:varchar(255)" json:"empresa"`
	EmpresaDocumento string `gorm:"type:varchar(20)" json:"empresa_documento"`

	// Relacionamentos GORM
	Schedules []CollaboratorSchedule `gorm:"foreignKey:CollaboratorID" json:"schedules,omitempty"`
}

type CollaboratorSchedule struct {
	ID             int    `gorm:"primaryKey;autoIncrement" json:"id"`
	CollaboratorID int    `gorm:"not null;index" json:"collaborator_id"`
	DiaSemana      int    `gorm:"not null" json:"dia_semana"` // índice do dia conforme a Secullum (convenção não confirmada — ver domain.CollaboratorSchedule)
	Entrada1       string `gorm:"type:varchar(5)" json:"entrada_1"`
	Saida1         string `gorm:"type:varchar(5)" json:"saida_1"`
	Entrada2       string `gorm:"type:varchar(5)" json:"entrada_2"`
	Saida2         string `gorm:"type:varchar(5)" json:"saida_2"`
	CargaMinutos   int    `gorm:"not null;default:0" json:"carga_minutos"`
}
