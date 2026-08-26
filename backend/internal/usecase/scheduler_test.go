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
	s := NewSchedulerService(repo, pub, nil)

	s.tick(at("01:00"))

	if got := pub.count(); got != 1 {
		t.Fatalf("esperava 1 disparo, veio %d", got)
	}
}

func TestScheduler_NaoDisparaForaDoHorario(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub, nil)

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
	s := NewSchedulerService(repo, pub, nil)

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
	s := NewSchedulerService(repo, pub, nil)

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
	s := NewSchedulerService(repo, pub, nil)

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
	s := NewSchedulerService(repo, pub, nil)

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
	s := NewSchedulerService(repo, pub, nil)

	s.hourlyTick()

	if got := pub.count(); got != 2 {
		t.Fatalf("esperava 1 disparo silencioso por tenant ativo (2), veio %d", got)
	}
}

// A atualização horária nunca notifica o WhatsApp: notify:false no payload publicado.
func TestScheduler_HourlyTickPublicaNotifyFalse(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub, nil)

	s.hourlyTick()

	if len(pub.calls) != 1 {
		t.Fatalf("esperava 1 chamada, veio %d", len(pub.calls))
	}
	if !strings.Contains(pub.calls[0], `"notify":false`) {
		t.Errorf("esperava notify:false no payload da atualização horária, veio %s", pub.calls[0])
	}
}

// A atualização horária audita o MÊS CORRENTE inteiro (até o último dia encerrado), não
// mais um único dia: o payload carrega start_date/end_date em vez de date.
func TestScheduler_HourlyTickPublicaPeriodoDoMes(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub, nil)

	s.hourlyTick()

	if len(pub.calls) != 1 {
		t.Fatalf("esperava 1 chamada, veio %d", len(pub.calls))
	}
	if !strings.Contains(pub.calls[0], `"start_date"`) || !strings.Contains(pub.calls[0], `"end_date"`) {
		t.Errorf("esperava start_date/end_date (período) no payload da atualização horária, veio %s", pub.calls[0])
	}
	if strings.Contains(pub.calls[0], `"date":"`) {
		t.Errorf("atualização horária não deveria mais publicar um único `date`, veio %s", pub.calls[0])
	}
}

// A atualização horária não usa claimForToday: pode disparar de novo mesmo no mesmo dia
// (é o comportamento esperado, ao contrário da varredura diária).
func TestScheduler_HourlyTickNaoTemLimiteDiario(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenantWithHorario(1, "01:00")}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub, nil)

	s.hourlyTick()
	s.hourlyTick()

	if got := pub.count(); got != 2 {
		t.Fatalf("esperava 2 disparos (1 por chamada), veio %d", got)
	}
}

// Exemplo do pedido: hoje é dia 15 => audita do dia 01 até o dia 14 (o último encerrado).
func TestMonthToDateRange_MeioDoMes(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	start, end := monthToDateRange(now)
	if got := start.Format("2006-01-02"); got != "2026-07-01" {
		t.Errorf("start = %s, quer 2026-07-01", got)
	}
	if got := end.Format("2006-01-02"); got != "2026-07-14" {
		t.Errorf("end = %s, quer 2026-07-14", got)
	}
}

// No 2º dia do mês, só o dia 1 está encerrado — período de um único dia.
func TestMonthToDateRange_SegundoDiaDoMes(t *testing.T) {
	now := time.Date(2026, 7, 2, 3, 0, 0, 0, time.UTC)
	start, end := monthToDateRange(now)
	if got := start.Format("2006-01-02"); got != "2026-07-01" {
		t.Errorf("start = %s, quer 2026-07-01", got)
	}
	if got := end.Format("2006-01-02"); got != "2026-07-01" {
		t.Errorf("end = %s, quer 2026-07-01", got)
	}
}

// No dia 1 de um mês, "ontem" pertence ao mês ANTERIOR — sem cair num intervalo vazio, o
// período cobre o mês anterior inteiro (dia 1 até o último dia dele), que é o mês cujo
// fechamento mais precisa ser conferido bem na virada.
func TestMonthToDateRange_PrimeiroDiaDoMesCobreMesAnteriorInteiro(t *testing.T) {
	now := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	start, end := monthToDateRange(now)
	if got := start.Format("2006-01-02"); got != "2026-07-01" {
		t.Errorf("start = %s, quer 2026-07-01", got)
	}
	if got := end.Format("2006-01-02"); got != "2026-07-31" {
		t.Errorf("end = %s, quer 2026-07-31", got)
	}
}

func TestScheduler_CadaTenantNoSeuProprioHorario(t *testing.T) {
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{
		tenantWithHorario(1, "01:00"),
		tenantWithHorario(2, "02:00"),
	}}
	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub, nil)

	s.tick(at("01:00"))
	if got := pub.count(); got != 1 {
		t.Fatalf("às 01:00 só o tenant 1 deveria disparar, vieram %d", got)
	}

	s.tick(at("02:00"))
	if got := pub.count(); got != 2 {
		t.Fatalf("às 02:00 o tenant 2 também deveria ter disparado, vieram %d", got)
	}
}

// multiTenantRepo é o fakeTenantRepo do SynchronizerService, mas com um tenant por id —
// necessário aqui porque o agendador sincroniza vários tenants por id real, ao contrário
// dos outros testes de sincronização, que giram em torno de um único tenant fixo.
type multiTenantRepo struct {
	byID map[int]*domain.Tenant
}

func (f *multiTenantRepo) GetByID(id int) (*domain.Tenant, error) { return f.byID[id], nil }
func (f *multiTenantRepo) GetActiveTenants() ([]*domain.Tenant, error) {
	panic("não usado")
}
func (f *multiTenantRepo) List(bool) ([]*domain.Tenant, error)              { panic("não usado") }
func (f *multiTenantRepo) Save(*domain.Tenant) error                        { panic("não usado") }
func (f *multiTenantRepo) Update(*domain.Tenant) error                      { panic("não usado") }
func (f *multiTenantRepo) Activate(int) error                               { panic("não usado") }
func (f *multiTenantRepo) Deactivate(int) error                             { panic("não usado") }
func (f *multiTenantRepo) Delete(int) error                                 { panic("não usado") }
func (f *multiTenantRepo) GetSettings(int) (*domain.TenantSettings, error)  { panic("não usado") }
func (f *multiTenantRepo) UpdateSettings(int, *domain.TenantSettings) error { panic("não usado") }

// TestScheduler_SincronizacaoDiariaFixaRodaParaTodosOsTenantsAtivos cobre o item 4 da
// especificação: às 03:00 (dailySyncTime), TODOS os tenants ativos têm colaboradores e
// equipamentos sincronizados — no MESMO horário para todos, independente do horário de
// fechamento configurado individualmente (que continua sendo checado à parte).
func TestScheduler_SincronizacaoDiariaFixaRodaParaTodosOsTenantsAtivos(t *testing.T) {
	tenant1 := &domain.Tenant{ID: 1, Active: true}
	tenant2 := &domain.Tenant{ID: 2, Active: true}
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenant1, tenant2}}

	syncTenantRepo := &multiTenantRepo{byID: map[int]*domain.Tenant{1: tenant1, 2: tenant2}}
	collabRepo := &fakeCollabRepo{}
	equipRepo := &fakeEquipRepo{}
	secullumSvc := &fakeSecullumSvc{
		collaborators: []domain.Collaborator{{SecullumID: 10}},
		equipments:    []domain.Equipment{{SecullumID: 1}},
	}
	syncService := NewSynchronizerService(syncTenantRepo, collabRepo, equipRepo, secullumSvc)

	s := NewSchedulerService(repo, &fakePublisher{}, syncService)
	s.tick(at(dailySyncTime))
	// runDailySync roda em goroutine própria (tick() não bloqueia — ver comentário em
	// tick()); o teste precisa esperar essa goroutine terminar antes de conferir o
	// resultado.
	s.waitForDailySync()

	if len(collabRepo.saved) != 1 {
		t.Errorf("esperava colaboradores sincronizados, veio %d chamada(s)", len(collabRepo.saved))
	}
	if len(equipRepo.saved) != 1 {
		t.Errorf("esperava equipamentos sincronizados, veio %d chamada(s)", len(equipRepo.saved))
	}
}

// TestScheduler_SincronizacaoDiariaSoRodaUmaVezPorDia garante que ticks repetidos no
// mesmo minuto (a checagem roda a cada 30s) não disparem a sincronização mais de uma vez.
func TestScheduler_SincronizacaoDiariaSoRodaUmaVezPorDia(t *testing.T) {
	tenant1 := &domain.Tenant{ID: 1, Active: true}
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenant1}}
	syncTenantRepo := &multiTenantRepo{byID: map[int]*domain.Tenant{1: tenant1}}

	var calls int
	collabRepo := &countingCollabRepo{fakeCollabRepo: fakeCollabRepo{}, onSaveAll: func() { calls++ }}
	syncService := NewSynchronizerService(syncTenantRepo, collabRepo, &fakeEquipRepo{}, &fakeSecullumSvc{})

	s := NewSchedulerService(repo, &fakePublisher{}, syncService)
	s.tick(at(dailySyncTime))
	s.tick(at(dailySyncTime))
	s.waitForDailySync() // espera a (única) goroutine de sync disparada pelo primeiro tick

	if calls != 1 {
		t.Errorf("sincronização diária rodou %d vez(es) no mesmo dia, quer 1", calls)
	}
}

// TestScheduler_SincronizacaoDiariaNaoBloqueiaOTick prova o bug encontrado em code
// review: tick() rodava a sincronização diária SINCRONAMENTE, bloqueando até todos os
// tenants terminarem (HTTP real à Secullum, minutos em produção). Como tick() roda dentro
// do único loop `select` de Start() (ver ticker/hourlyTicker), enquanto ele estivesse
// bloqueado o loop de disparo por Horario (mais abaixo NO MESMO tick()) ficava represado, e
// o próximo tick do ticker/hourlyTicker ficava pendente sem processar — tickers do Go não
// enfileiram ticks perdidos, descartam. Um tenant com Horario == dailySyncTime podia ter o
// fechamento atrasado; a atualização horária silenciosa podia ser pulada na hora certa.
func TestScheduler_SincronizacaoDiariaNaoBloqueiaOTick(t *testing.T) {
	tenant1 := &domain.Tenant{ID: 1, Active: true, Settings: &domain.TenantSettings{Horario: dailySyncTime}}
	repo := &schedulerTenantRepo{tenants: []*domain.Tenant{tenant1}}
	syncTenantRepo := &multiTenantRepo{byID: map[int]*domain.Tenant{1: tenant1}}

	const syncDelay = 200 * time.Millisecond
	syncService := NewSynchronizerService(syncTenantRepo, &fakeCollabRepo{}, &fakeEquipRepo{}, &fakeSecullumSvc{delay: syncDelay})

	pub := &fakePublisher{}
	s := NewSchedulerService(repo, pub, syncService)

	start := time.Now()
	s.tick(at(dailySyncTime))
	elapsed := time.Since(start)

	if elapsed >= syncDelay {
		t.Fatalf("tick() bloqueou por %v esperando a sincronização diária terminar (limiar %v) — "+
			"o disparo por Horario e o próximo tick do agendador ficam represados enquanto a sync roda", elapsed, syncDelay)
	}
	// O fechamento diário do tenant (Horario == dailySyncTime) precisa disparar mesmo com
	// a sincronização ainda em andamento em background — são independentes.
	if got := pub.count(); got != 1 {
		t.Errorf("esperava a auditoria de fechamento disparada mesmo com a sync em andamento, pub.count() = %d", got)
	}

	s.waitForDailySync() // ponto determinístico para o teste seguinte não vazar a goroutine
}

type countingCollabRepo struct {
	fakeCollabRepo
	onSaveAll func()
}

func (f *countingCollabRepo) SaveAll(collaborators []domain.Collaborator) error {
	f.onSaveAll()
	return f.fakeCollabRepo.SaveAll(collaborators)
}
