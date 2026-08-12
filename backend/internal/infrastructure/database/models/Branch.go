package models

// Branch é a filial. O gestor (1—1) mora inline: nome e telefone não têm vida própria
// fora da filial, então uma tabela separada só somaria um join.
type Branch struct {
	ID       int    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID int    `gorm:"not null;index" json:"tenant_id"`
	Name     string `gorm:"type:varchar(255);not null" json:"name"`

	ManagerName  string `gorm:"type:varchar(255)" json:"manager_name"`
	ManagerPhone string `gorm:"type:varchar(20)" json:"manager_phone"`

	Devices        []BranchDevice        `gorm:"foreignKey:BranchID" json:"devices,omitempty"`
	PayrollNumbers []BranchPayrollNumber `gorm:"foreignKey:BranchID" json:"payroll_numbers,omitempty"`
}

// BranchDevice é o aparelho de ponto. O índice único (tenant, equip) implementa a regra
// "um aparelho pertence a uma única filial" — tentar cadastrar o mesmo aparelho em duas
// filiais é rejeitado pelo banco, não só pela aplicação.
type BranchDevice struct {
	ID              int    `gorm:"primaryKey;autoIncrement" json:"id"`
	BranchID        int    `gorm:"not null;index" json:"branch_id"`
	TenantID        int    `gorm:"not null;uniqueIndex:idx_branch_device_equip,priority:1" json:"tenant_id"`
	SecullumEquipID int    `gorm:"not null;uniqueIndex:idx_branch_device_equip,priority:2" json:"secullum_equip_id"`
	Label           string `gorm:"type:varchar(120)" json:"label"`
}

// BranchPayrollNumber vincula um N° folha à filial. Único por tenant: um colaborador não
// pode estar lotado em duas filiais ao mesmo tempo.
type BranchPayrollNumber struct {
	ID       int    `gorm:"primaryKey;autoIncrement" json:"id"`
	BranchID int    `gorm:"not null;index" json:"branch_id"`
	TenantID int    `gorm:"not null;uniqueIndex:idx_branch_payroll_numero,priority:1" json:"tenant_id"`
	Numero   string `gorm:"type:varchar(40);not null;uniqueIndex:idx_branch_payroll_numero,priority:2" json:"numero"`
}
