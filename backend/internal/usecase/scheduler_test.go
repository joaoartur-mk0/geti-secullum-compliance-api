package usecase

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"backend/internal/domain"
)

// schedulerTenantRepo devolve uma lista fixa de tenants ativos, com o horário configurado
// que o teste quiser. Só os métodos usados pelo SchedulerService estão implementados;
// os demais entram em pânico se chamados, para o teste falhar alto se o contrato mudar.
type schedulerTenantRepo struct {
	tenants []*domain.Tenant
	err     error
}

func (f *schedulerTenantRepo) GetActiveTenants() ([]*domain.Tenant, error) { return f.tenants, f.err }

func (f *schedulerTenantRepo) GetByID(int) (*domain.Tenant, error)              { panic("não usado") }
func (f *schedulerTenantRepo) List(bool) ([]*domain.Tenant, error)              { panic("não usado") }
func (f *schedulerTenantRepo) Save(*domain.Tenant) error                        { panic("não usado") }
func (f *schedulerTenantRepo) Update(*domain.Tenant) error                      { panic("não usado") }
func (f *schedulerTenantRepo) Activate(int) error                               { panic("não usado") }
func (f *schedulerTenantRepo) Deactivate(int) error                             { panic("não usado") }
func (f *schedulerTenantRepo) Delete(int) error                                 { panic("não usado") }
func (f *schedulerTenantRepo) GetSettings(int) (*domain.TenantSettings, error)  { panic("não usado") }
func (f *schedulerTenantRepo) UpdateSettings(int, *domain.TenantSettings) error { panic("não usado") }

// fakePublisher registra cada publicação, para o teste contar quantas vezes (e para
// quais tenants) o agendador realmente disparou.
type fakePublisher struct {
	mu    sync.Mutex
	calls []string // tenant_id em texto, na ordem de chamada
}

func (f *fakePublisher) Publish(_ context.Context, queue string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, queue+":"+string(body))
	return nil
}

func (f *fakePublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func tenantWithHorario(id int, horario string) *domain.Tenant {
	return &domain.Tenant{ID: id, Active: true, Settings: &domain.TenantSettings{Horario: horario}}
}

var refDay = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func at(hhmm string) time.Time {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		panic(err)
	}
	return time.Date(refDay.Year(), refDay.Month(), refDay.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
}

func TestScheduler_DisparaNoHorarioConfigurado(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.tick(at("01:00"))

	if got := pub.count(); got != 1 {
		t.Fatalf("esperava 1 disparo, veio %d", got)
	}
}

func TestScheduler_NaoDisparaForaDoHorario(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.tick(at("00:59"))
	s.tick(at("01:01"))

	if got := pub.count(); got != 0 {
		t.Fatalf("esperava 0 disparos fora do horário, veio %d", got)
	}
}

// O ticker roda a cada 30s — o mesmo minuto "01:00" é observado em mais de um tick. O
// tenant só pode ser auditado automaticamente UMA vez naquele dia.
func TestScheduler_NaoDisparaDuasVezesNoMesmoDia(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.tick(at("01:00"))
	s.tick(at("01:00").Add(30 * time.Second)) // ainda dentro do minuto 01:00
	s.tick(at("01:00").Add(90 * time.Second)) // já 01:01, não bate mais o horário

	if got := pub.count(); got != 1 {
		t.Fatalf("esperava exatamente 1 disparo no dia, vieram %d", got)
	}
}

// No dia seguinte, no mesmo horário, deve disparar de novo.
func TestScheduler_DisparaDeNovoNoDiaSeguinte(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.tick(at("01:00"))
	s.tick(at("01:00").AddDate(0, 0, 1))

	if got := pub.count(); got != 2 {
		t.Fatalf("esperava 1 disparo por dia (2 dias = 2 disparos), veio %d", got)
	}
}

func TestScheduler_IgnoraTenantSemHorarioConfigurado(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{
		tenantWithHorario(1, ""),
		{ID: 2, Active: true, Settings: nil},
	}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.tick(at("01:00"))

	if got := pub.count(); got != 0 {
		t.Fatalf("tenant sem horário/settings não deveria disparar, veio %d", got)
	}
}

// A varredura diária (tick) é a ÚNICA que deve notificar o WhatsApp: notify:true no
// payload publicado.
func TestScheduler_TickPublicaNotifyTrue(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.tick(at("01:00"))

	if len(pub.calls) != 1 {
		t.Fatalf("esperava 1 chamada, veio %d", len(pub.calls))
	}
	if !strings.Contains(pub.calls[0], `"notify":true`) {
		t.Errorf("esperava notify:true no payload da varredura diária, veio %s", pub.calls[0])
	}
}

func TestScheduler_HourlyTickDisparaParaTodosOsAtivos(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{
		tenantWithHorario(1, "01:00"),
		tenantWithHorario(2, ""), // sem horário diário configurado; a atualização horária não depende disso
	}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.hourlyTick()

	if got := pub.count(); got != 2 {
		t.Fatalf("esperava 1 disparo silencioso por tenant ativo (2), veio %d", got)
	}
}

// A atualização horária nunca notifica o WhatsApp: notify:false no payload publicado.
func TestScheduler_HourlyTickPublicaNotifyFalse(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.hourlyTick()

	if len(pub.calls) != 1 {
		t.Fatalf("esperava 1 chamada, veio %d", len(pub.calls))
	}
	if !strings.Contains(pub.calls[0], `"notify":false`) {
		t.Errorf("esperava notify:false no payload da atualização horária, veio %s", pub.calls[0])
	}
}

// A atualização horária não usa claimForToday: pode disparar de novo mesmo no mesmo dia
// (é o comportamento esperado, ao contrário da varredura diária).
func TestScheduler_HourlyTickNaoTemLimiteDiario(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.hourlyTick()
	s.hourlyTick()

	if got := pub.count(); got != 2 {
		t.Fatalf("esperava 2 disparos (1 por chamada), veio %d", got)
	}
}

func TestScheduler_CadaTenantNoSeuProprioHorario(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{
		tenantWithHorario(1, "01:00"),
		tenantWithHorario(2, "02:00"),
	}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub)

	s.tick(at("01:00"))
	if got := pub.count(); got != 1 {
		t.Fatalf("às 01:00 só o tenant 1 deveria disparar, vieram %d", got)
	}

	s.tick(at("02:00"))
	if got := pub.count(); got != 2 {
		t.Fatalf("às 02:00 o tenant 2 também deveria ter disparado, vieram %d", got)
	}
}
