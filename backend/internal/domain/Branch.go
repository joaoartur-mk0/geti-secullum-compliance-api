package domain

// Branch é a filial (unidade física) do cliente.
//
// Existe por dois motivos operacionais: saber DE ONDE veio a batida (o aparelho de ponto
// fica na filial) e QUEM cobrar quando algo está errado (o gestor da filial, que recebe a
// advertência e o alerta). A Secullum não modela isso — para ela existe só a empresa.
//
// Cardinalidades: uma filial tem N aparelhos e exatamente um gestor (nome + telefone,
// inline aqui porque não há vida própria fora da filial). Um aparelho pertence a uma
// única filial — garantido pela unicidade de SecullumEquipID dentro do tenant.
type Branch struct {
	ID       int
	TenantID int
	Name     string

	ManagerName  string
	ManagerPhone string

	Devices        []BranchDevice
	PayrollNumbers []BranchPayrollNumber
}

// BranchDevice é o aparelho (relógio de ponto) instalado na filial. SecullumEquipID é o
// `EquipId` que aparece em cada batida da Secullum — é por ele que a filial é resolvida.
type BranchDevice struct {
	ID              int
	BranchID        int
	TenantID        int
	SecullumEquipID int
	Label           string // identificação legível ("Recepção", "Doca 2")
}

// BranchPayrollNumber é o N° folha de um colaborador lotado na filial.
//
// É o plano B da resolução: batidas feitas pelo app/web chegam SEM EquipId (a maioria,
// nos dados reais da operação), e nesses casos o aparelho não diz nada. O número de folha
// é cadastro estável e resolve a filial mesmo sem batida no relógio.
type BranchPayrollNumber struct {
	ID       int
	BranchID int
	TenantID int
	Numero   string
}

// BranchResolutionSource registra COMO a filial foi determinada, para o painel poder
// mostrar ao gestor que aquilo é inferência e não cadastro direto.
type BranchResolutionSource string

const (
	BranchFromDevice        BranchResolutionSource = "aparelho"
	BranchFromPayrollNumber BranchResolutionSource = "numero_folha"
	BranchUnresolved        BranchResolutionSource = ""
)

// BranchRepository é o contrato de persistência de filiais, aparelhos e números de folha.
type BranchRepository interface {
	Create(branch *Branch) error
	Update(branch *Branch) error
	Delete(id int) error
	GetByID(id int) (*Branch, error)
	ListByTenant(tenantID int) ([]Branch, error)

	AddDevice(device *BranchDevice) error
	RemoveDevice(deviceID int) error
	AddPayrollNumber(pn *BranchPayrollNumber) error
	RemovePayrollNumber(pnID int) error

	// FindByDevice devolve a filial dona do aparelho, ou nil se o aparelho não estiver
	// cadastrado (aparelho novo instalado sem passar pelo painel).
	FindByDevice(tenantID int, secullumEquipID int) (*Branch, error)
	// FindByPayrollNumber devolve a filial que reivindica aquele N° folha, ou nil.
	FindByPayrollNumber(tenantID int, numero string) (*Branch, error)
}
