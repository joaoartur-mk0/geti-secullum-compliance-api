package usecase

import (
	"errors"
	"testing"
	"time"

	"backend/internal/domain"
)

// --- Fakes mínimos das interfaces de domínio, só para este teste ---

type fakeTenantRepo struct {
	tenant *domain.Tenant
	err    error
}

func (f *fakeTenantRepo) GetByID(id int) (*domain.Tenant, error)           { return f.tenant, f.err }
func (f *fakeTenantRepo) GetActiveTenants() ([]*domain.Tenant, error)      { return nil, nil }
func (f *fakeTenantRepo) List(bool) ([]*domain.Tenant, error)              { return nil, nil }
func (f *fakeTenantRepo) Save(*domain.Tenant) error                        { return nil }
func (f *fakeTenantRepo) Update(*domain.Tenant) error                      { return nil }
func (f *fakeTenantRepo) Activate(int) error                               { return nil }
func (f *fakeTenantRepo) Deactivate(int) error                             { return nil }
func (f *fakeTenantRepo) Delete(int) error                                 { return nil }
func (f *fakeTenantRepo) GetSettings(int) (*domain.TenantSettings, error)  { return nil, nil }
func (f *fakeTenantRepo) UpdateSettings(int, *domain.TenantSettings) error { return nil }

type fakeCollabRepo struct {
	saved []domain.Collaborator
	err   error
}

func (f *fakeCollabRepo) Save(*domain.Collaborator) error { return nil }
func (f *fakeCollabRepo) SaveAll(collaborators []domain.Collaborator) error {
	if f.err != nil {
		return f.err
	}
	f.saved = collaborators
	return nil
}
func (f *fakeCollabRepo) GetByTenantID(int) ([]domain.Collaborator, error) { return nil, nil }
func (f *fakeCollabRepo) GetBySecullumID(int, int) (*domain.Collaborator, error) {
	return nil, nil
}

type fakeSecullumSvc struct {
	collaborators []domain.Collaborator
	err           error

	horarios     map[int][]domain.CollaboratorSchedule
	horarioCalls int // conta chamadas a GetHorario, para testar o dedupe
}

func (f *fakeSecullumSvc) GetDailyPunches(*domain.Tenant, time.Time) ([]domain.DailyPunch, error) {
	return nil, nil
}
func (f *fakeSecullumSvc) GetCollaborators(*domain.Tenant) ([]domain.Collaborator, error) {
	return f.collaborators, f.err
}
func (f *fakeSecullumSvc) GetHorario(_ *domain.Tenant, numero int) ([]domain.CollaboratorSchedule, error) {
	f.horarioCalls++
	return f.horarios[numero], nil
}

func TestSyncTenant_Sucesso(t *testing.T) {
	tenantRepo := &fakeTenantRepo{tenant: &domain.Tenant{ID: 1, SecullumDatabaseID: 82720}}
	collabRepo := &fakeCollabRepo{}
	secullumSvc := &fakeSecullumSvc{
		collaborators: []domain.Collaborator{
			{SecullumID: 10, Name: "Fulano"},
			{SecullumID: 20, Name: "Ciclana"},
		},
	}

	sync := NewSynchronizerService(tenantRepo, collabRepo, secullumSvc)
	n, err := sync.SyncTenant(1)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, quer 2", n)
	}
	if len(collabRepo.saved) != 2 {
		t.Fatalf("esperava 2 colaboradores salvos, veio %d", len(collabRepo.saved))
	}
	// TenantID deve ser propagado para cada colaborador antes de salvar.
	for _, c := range collabRepo.saved {
		if c.TenantID != 1 {
			t.Errorf("colaborador %d: TenantID = %d, quer 1", c.SecullumID, c.TenantID)
		}
	}
}

func TestSyncTenant_DedupeHorarios(t *testing.T) {
	tenantRepo := &fakeTenantRepo{tenant: &domain.Tenant{ID: 1}}
	collabRepo := &fakeCollabRepo{}
	horario8 := []domain.CollaboratorSchedule{
		{DiaSemana: 1, Entrada1: "08:00", Saida1: "12:00", Entrada2: "13:50", Saida2: "17:10", CargaMinutos: 440},
	}
	secullumSvc := &fakeSecullumSvc{
		// 3 colaboradores, mas só 2 números de horário distintos (8 aparece 2x).
		collaborators: []domain.Collaborator{
			{SecullumID: 1, HorarioNumero: 8},
			{SecullumID: 2, HorarioNumero: 8},
			{SecullumID: 3, HorarioNumero: 9},
		},
		horarios: map[int][]domain.CollaboratorSchedule{
			8: horario8,
			9: {{DiaSemana: 1, Entrada1: "09:00", Saida1: "18:00", CargaMinutos: 480}},
		},
	}

	sync := NewSynchronizerService(tenantRepo, collabRepo, secullumSvc)
	if _, err := sync.SyncTenant(1); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	// GetHorario deve ser chamado uma vez por número distinto (2), não uma vez por
	// colaborador (3) — é o que evita estourar o rate limit da Secullum.
	if secullumSvc.horarioCalls != 2 {
		t.Errorf("GetHorario chamado %d vez(es), quer 2 (dedupe por número)", secullumSvc.horarioCalls)
	}

	for _, c := range collabRepo.saved {
		if len(c.Schedules) == 0 {
			t.Errorf("colaborador %d sem Schedules", c.SecullumID)
			continue
		}
		if c.HorarioNumero == 8 && c.Schedules[0].CargaMinutos != 440 {
			t.Errorf("colaborador %d: CargaMinutos = %d, quer 440", c.SecullumID, c.Schedules[0].CargaMinutos)
		}
	}
}

func TestSyncTenant_FalhaAoBuscarHorarioNaoAbortaSincronizacao(t *testing.T) {
	tenantRepo := &fakeTenantRepo{tenant: &domain.Tenant{ID: 1}}
	collabRepo := &fakeCollabRepo{}
	secullumSvc := &fakeSecullumSvc{
		collaborators: []domain.Collaborator{{SecullumID: 1, HorarioNumero: 8}},
		horarios:      nil, // GetHorario devolve vazio para o número 8 (simula falha tratada)
	}

	sync := NewSynchronizerService(tenantRepo, collabRepo, secullumSvc)
	n, err := sync.SyncTenant(1)
	if err != nil {
		t.Fatalf("falha ao buscar UM horário não deveria abortar a sincronização: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, quer 1 (colaborador salvo mesmo sem jornada)", n)
	}
}

func TestSyncTenant_TenantNaoEncontrado(t *testing.T) {
	tenantRepo := &fakeTenantRepo{err: domain.NewNotFound("op", "tenant não encontrado", nil)}
	sync := NewSynchronizerService(tenantRepo, &fakeCollabRepo{}, &fakeSecullumSvc{})

	if _, err := sync.SyncTenant(99); err == nil {
		t.Fatalf("esperava erro de tenant não encontrado")
	}
}

func TestSyncTenant_FalhaNaSecullum(t *testing.T) {
	tenantRepo := &fakeTenantRepo{tenant: &domain.Tenant{ID: 1}}
	secullumSvc := &fakeSecullumSvc{err: errors.New("status 401")}
	sync := NewSynchronizerService(tenantRepo, &fakeCollabRepo{}, secullumSvc)

	if _, err := sync.SyncTenant(1); err == nil {
		t.Fatalf("esperava erro propagado da Secullum")
	}
}

func TestSyncTenant_FalhaAoSalvar(t *testing.T) {
	tenantRepo := &fakeTenantRepo{tenant: &domain.Tenant{ID: 1}}
	secullumSvc := &fakeSecullumSvc{collaborators: []domain.Collaborator{{SecullumID: 1}}}
	collabRepo := &fakeCollabRepo{err: errors.New("db down")}
	sync := NewSynchronizerService(tenantRepo, collabRepo, secullumSvc)

	if _, err := sync.SyncTenant(1); err == nil {
		t.Fatalf("esperava erro propagado do repositório")
	}
}
