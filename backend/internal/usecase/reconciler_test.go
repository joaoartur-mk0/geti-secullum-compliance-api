package usecase

import (
	"fmt"
	"testing"
	"time"

	"backend/internal/domain"
)

// fakeOccurrenceRepo é um espelho em memória do repositório real que IMPÕE a mesma chave
// única do banco (tenant + colaborador + data + tipo). Se o reconciliador tentar inserir
// uma ocorrência que já existe, o teste falha aqui — exatamente como o Postgres falharia.
type fakeOccurrenceRepo struct {
	rows   map[string]domain.Occurrence
	events []domain.OccurrenceEvent
	nextID int
}

func newFakeOccurrenceRepo() *fakeOccurrenceRepo {
	return &fakeOccurrenceRepo{rows: map[string]domain.Occurrence{}, nextID: 1}
}

func fakeKey(tenantID, collaboratorID int, date time.Time, typ string) string {
	return fmt.Sprintf("%d|%d|%s|%s", tenantID, collaboratorID, date.Format("2006-01-02"), typ)
}

func (f *fakeOccurrenceRepo) keyOf(o domain.Occurrence) string {
	return fakeKey(o.TenantID, o.CollaboratorID, o.Date, o.Type)
}

func (f *fakeOccurrenceRepo) ListByTenantAndDate(tenantID int, date time.Time) ([]domain.Occurrence, error) {
	var out []domain.Occurrence
	for _, o := range f.rows {
		if o.TenantID == tenantID && o.Date.Equal(domain.DayOf(date)) {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeOccurrenceRepo) List(domain.OccurrenceFilter) ([]domain.Occurrence, error) {
	return nil, nil
}

func (f *fakeOccurrenceRepo) GetByID(id int) (*domain.Occurrence, error) {
	for _, o := range f.rows {
		if o.ID == id {
			return &o, nil
		}
	}
	return nil, domain.NewNotFound("fake.GetByID", "ocorrência não encontrada", nil)
}

func (f *fakeOccurrenceRepo) ApplyChanges(changes []domain.OccurrenceChange) error {
	for _, ch := range changes {
		key := f.keyOf(ch.Occurrence)
		if ch.Kind == domain.ChangeInsert {
			if _, exists := f.rows[key]; exists {
				return fmt.Errorf("violação da chave única: %s já existe", key)
			}
			ch.Occurrence.ID = f.nextID
			f.nextID++
		}
		f.rows[key] = ch.Occurrence
		if ch.Event != nil {
			ev := *ch.Event
			ev.OccurrenceID = f.rows[key].ID
			f.events = append(f.events, ev)
		}
	}
	return nil
}

func (f *fakeOccurrenceRepo) Ignore(id int, reason string, actorUserID *int) error {
	for key, o := range f.rows {
		if o.ID != id {
			continue
		}
		o.State = domain.OccurrenceResolvedManual
		o.IgnoredReason = reason
		o.IgnoredByUserID = actorUserID
		f.rows[key] = o
		f.events = append(f.events, domain.OccurrenceEvent{
			OccurrenceID: id,
			Type:         domain.EventResolvedManual,
			ToState:      domain.OccurrenceResolvedManual,
			Reason:       reason,
			ActorUserID:  actorUserID,
		})
		return nil
	}
	return domain.NewNotFound("fake.Ignore", "ocorrência não encontrada", nil)
}

func (f *fakeOccurrenceRepo) ListEvents(occurrenceID int) ([]domain.OccurrenceEvent, error) {
	var out []domain.OccurrenceEvent
	for _, e := range f.events {
		if e.OccurrenceID == occurrenceID {
			out = append(out, e)
		}
	}
	return out, nil
}

// --- Helpers ---

const testTenant = 7

var testDay = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func inc(collabID int, typ string, sev domain.Severity, desc string) domain.AuditInconsistency {
	return domain.AuditInconsistency{
		CollaboratorID:   collabID,
		CollaboratorName: "Fulano",
		Type:             typ,
		Severity:         sev,
		Description:      desc,
	}
}

func almoco(minutos int) domain.AuditInconsistency {
	return inc(1, TipoAlmocoReduzido, domain.SeverityCritical,
		fmt.Sprintf("O intervalo foi de apenas %d minutos, inferior ao mínimo.", minutos))
}

// only devolve a única ocorrência do dia, falhando se houver zero ou mais de uma.
func only(t *testing.T, repo *fakeOccurrenceRepo) domain.Occurrence {
	t.Helper()
	got, err := repo.ListByTenantAndDate(testTenant, testDay)
	if err != nil {
		t.Fatalf("erro inesperado ao listar: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("esperava exatamente 1 ocorrência, veio %d: %+v", len(got), got)
	}
	return got[0]
}

// --- Testes ---

func TestReconcile_PrimeiraVarreduraCriaOcorrencia(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)

	res, err := svc.Reconcile(testTenant, testDay, []domain.AuditInconsistency{almoco(43)}, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("esperava 1 ocorrência criada, veio %+v", res)
	}

	occ := only(t, repo)
	if occ.State != domain.OccurrenceOpen {
		t.Errorf("ocorrência nova deveria nascer %q, veio %q", domain.OccurrenceOpen, occ.State)
	}
	if occ.TimesSeen != 1 {
		t.Errorf("times_seen deveria ser 1, veio %d", occ.TimesSeen)
	}
	if len(repo.events) != 1 || repo.events[0].Type != domain.EventCreated {
		t.Errorf("esperava um único evento %q, veio %+v", domain.EventCreated, repo.events)
	}
}

// O teste que o roadmap pede: bateria de syncs no mesmo dia não pode duplicar.
func TestReconcile_SyncsRepetidosNoMesmoDiaNaoDuplicam(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)
	fresh := []domain.AuditInconsistency{
		almoco(43),
		inc(2, TipoInterjornada, domain.SeverityCritical, "O descanso entre jornadas foi de apenas 9h."),
		inc(3, TipoTrabalhoEmFolga, domain.SeverityOperational, "O colaborador trabalhou 8h em dia de folga."),
	}

	for i := 1; i <= 5; i++ {
		res, err := svc.Reconcile(testTenant, testDay, fresh, time.Now())
		if err != nil {
			t.Fatalf("sync %d: erro inesperado: %v", i, err)
		}
		if i == 1 {
			continue
		}
		if len(res.Created) != 0 || len(res.Updated) != 0 || len(res.Resolved) != 0 {
			t.Errorf("sync %d não deveria mexer em nada, veio %+v", i, res)
		}
		if res.Unchanged != len(fresh) {
			t.Errorf("sync %d: esperava %d inalteradas, veio %d", i, len(fresh), res.Unchanged)
		}
	}

	rows, _ := repo.ListByTenantAndDate(testTenant, testDay)
	if len(rows) != len(fresh) {
		t.Fatalf("5 varreduras deveriam deixar %d ocorrências, vieram %d", len(fresh), len(rows))
	}
	for _, o := range rows {
		if o.TimesSeen != 5 {
			t.Errorf("ocorrência %q deveria ter sido vista 5 vezes, veio %d", o.Type, o.TimesSeen)
		}
		if o.State != domain.OccurrenceOpen {
			t.Errorf("ocorrência %q deveria seguir %q, veio %q", o.Type, domain.OccurrenceOpen, o.State)
		}
	}
	// Nenhum evento além das 3 criações: repetir a varredura não polui o log.
	if len(repo.events) != len(fresh) {
		t.Errorf("esperava %d eventos (só as criações), vieram %d: %+v", len(fresh), len(repo.events), repo.events)
	}
}

func TestReconcile_ValorNovoAtualizaEReavalia(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)

	if _, err := svc.Reconcile(testTenant, testDay, []domain.AuditInconsistency{almoco(43)}, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// Mesma infração, valor pior: o intervalo caiu de 43min para 20min.
	res, err := svc.Reconcile(testTenant, testDay, []domain.AuditInconsistency{almoco(20)}, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(res.Updated) != 1 || len(res.Created) != 0 {
		t.Fatalf("esperava 1 atualização e nenhuma criação, veio %+v", res)
	}

	occ := only(t, repo)
	if occ.State != domain.OccurrenceUpdated {
		t.Errorf("valor novo deveria levar a %q, veio %q", domain.OccurrenceUpdated, occ.State)
	}
	if occ.Category() != domain.CategoryUnconfirmed {
		t.Errorf("ocorrência atualizada deveria cair na categoria %q, veio %q", domain.CategoryUnconfirmed, occ.Category())
	}

	ev := repo.events[len(repo.events)-1]
	if ev.Type != domain.EventUpdated {
		t.Fatalf("esperava evento %q, veio %q", domain.EventUpdated, ev.Type)
	}
	if ev.FromDescription == ev.ToDescription {
		t.Error("o evento deveria registrar a descrição antes e depois")
	}
}

func TestReconcile_OcorrenciaQueSomeResolveSozinha(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)

	if _, err := svc.Reconcile(testTenant, testDay, []domain.AuditInconsistency{almoco(43)}, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// A batida foi corrigida na Secullum: a inconsistência não é mais apurada.
	res, err := svc.Reconcile(testTenant, testDay, nil, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(res.Resolved) != 1 {
		t.Fatalf("esperava 1 resolução automática, veio %+v", res)
	}

	occ := only(t, repo)
	if occ.State != domain.OccurrenceResolvedAuto {
		t.Errorf("esperava %q, veio %q", domain.OccurrenceResolvedAuto, occ.State)
	}
	if occ.ResolvedAt == nil {
		t.Error("resolvida automaticamente deveria ter data de resolução")
	}
}

// A troca de escala não registrada nasce operacional e desaparece sozinha quando o gestor
// corrige a escala na Secullum — sem ação de ninguém no painel.
func TestReconcile_TrocaDeEscalaSeResolveAoCorrigirEscala(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)
	folga := inc(3, TipoTrabalhoEmFolga, domain.SeverityOperational, "O colaborador trabalhou 8h em dia de folga.")

	if _, err := svc.Reconcile(testTenant, testDay, []domain.AuditInconsistency{folga}, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got := only(t, repo).Category(); got != domain.CategoryScheduleChange {
		t.Errorf("esperava categoria %q, veio %q", domain.CategoryScheduleChange, got)
	}

	if _, err := svc.Reconcile(testTenant, testDay, nil, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got := only(t, repo).State; got != domain.OccurrenceResolvedAuto {
		t.Errorf("escala corrigida deveria resolver sozinha, veio %q", got)
	}
}

func TestReconcile_ReaparecerDepoisDeResolvidaReabre(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)

	fresh := []domain.AuditInconsistency{almoco(43)}
	if _, err := svc.Reconcile(testTenant, testDay, fresh, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, err := svc.Reconcile(testTenant, testDay, nil, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// A correção foi desfeita na Secullum e o problema voltou.
	res, err := svc.Reconcile(testTenant, testDay, fresh, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(res.Reopened) != 1 || len(res.Created) != 0 {
		t.Fatalf("esperava 1 reabertura e nenhuma criação, veio %+v", res)
	}

	occ := only(t, repo)
	if occ.State != domain.OccurrenceUpdated {
		t.Errorf("reaberta deveria voltar como %q, veio %q", domain.OccurrenceUpdated, occ.State)
	}
	if occ.ResolvedAt != nil {
		t.Error("reaberta não pode manter a data de resolução")
	}
	if last := repo.events[len(repo.events)-1]; last.Type != domain.EventReopened {
		t.Errorf("esperava evento %q, veio %q", domain.EventReopened, last.Type)
	}
}

func TestReconcile_IgnoradaManualmenteNaoRessuscita(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)
	fresh := []domain.AuditInconsistency{almoco(43)}

	if _, err := svc.Reconcile(testTenant, testDay, fresh, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := repo.Ignore(only(t, repo).ID, "abonado pelo RH", nil); err != nil {
		t.Fatalf("erro ao ignorar: %v", err)
	}

	// Varreduras seguintes continuam apurando a mesma infração.
	for i := 0; i < 3; i++ {
		res, err := svc.Reconcile(testTenant, testDay, fresh, time.Now())
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if res.Ignored != 1 || len(res.Created) != 0 || len(res.Updated) != 0 || len(res.Reopened) != 0 {
			t.Fatalf("ocorrência ignorada não deveria voltar ao radar, veio %+v", res)
		}
	}

	if got := only(t, repo).State; got != domain.OccurrenceResolvedManual {
		t.Errorf("deveria continuar %q, veio %q", domain.OccurrenceResolvedManual, got)
	}
}

// Ignorar vale para o dia ignorado, não para o problema em geral: no dia seguinte a mesma
// infração é uma ocorrência nova (a identidade inclui a data).
func TestReconcile_IdentidadeIncluiADataEOTipo(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)
	outroDia := testDay.AddDate(0, 0, 1)

	if _, err := svc.Reconcile(testTenant, testDay, []domain.AuditInconsistency{almoco(43)}, time.Now()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	res, err := svc.Reconcile(testTenant, outroDia, []domain.AuditInconsistency{almoco(43)}, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("o mesmo problema em outro dia é ocorrência nova, veio %+v", res)
	}
	// E o dia anterior não foi tocado (nem resolvido por ausência).
	if got := only(t, repo).State; got != domain.OccurrenceOpen {
		t.Errorf("auditar outro dia não pode resolver o dia anterior, veio %q", got)
	}
}

// A varredura pode ocorrer em qualquer horário; a chave é o DIA. Sem truncar, cada
// varredura criaria uma ocorrência nova.
func TestReconcile_HorarioDaVarreduraNaoAfetaAIdentidade(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)
	manha := time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC)
	noite := time.Date(2026, 7, 1, 23, 59, 0, 0, time.UTC)

	if _, err := svc.Reconcile(testTenant, manha, []domain.AuditInconsistency{almoco(43)}, manha); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if _, err := svc.Reconcile(testTenant, noite, []domain.AuditInconsistency{almoco(43)}, noite); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if occ := only(t, repo); occ.TimesSeen != 2 {
		t.Errorf("esperava a mesma ocorrência vista 2 vezes, veio times_seen=%d", occ.TimesSeen)
	}
}

func TestReconcile_DuplicataNaMesmaVarreduraNaoViraDuasOcorrencias(t *testing.T) {
	repo := newFakeOccurrenceRepo()
	svc := NewReconcilerService(repo)

	res, err := svc.Reconcile(testTenant, testDay, []domain.AuditInconsistency{almoco(43), almoco(43)}, time.Now())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("esperava 1 ocorrência criada, veio %+v", res)
	}
	only(t, repo)
}
