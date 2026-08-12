//go:build integration

// Teste de integração com Postgres REAL.
//
// Os testes do reconciliador (usecase/reconciler_test.go) provam a lógica da máquina de
// estados contra um repositório em memória. Este aqui prova a outra metade, que nenhum
// fake pode provar: que o índice único (tenant, colaborador, data, tipo) existe de fato no
// banco e que a bateria de syncs no mesmo dia não deixa linhas duplicadas — inclusive com
// varreduras CONCORRENTES, o cenário em que dois workers pegam o mesmo tenant.
//
// Como rodar:
//
//	docker compose -f infrastructure/docker-compose.local.yml up -d
//	DATABASE_URL="host=localhost user=postgres password=postgres dbname=auditoria_db port=5432 sslmode=disable" \
//	  go test -tags=integration ./internal/infrastructure/database/...
package database_test

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/domain"
	"backend/internal/infrastructure/database/models"
	"backend/internal/infrastructure/database/repositories"
	"backend/internal/usecase"
)

// tenantID fictício e fora da faixa de uso real, para o teste não colidir com dados de
// desenvolvimento no mesmo banco.
const itTenantID = 999999

var itDay = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func setupDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL não definida: pulando o teste de integração com Postgres")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("falha ao conectar ao Postgres: %v", err)
	}
	if err := db.AutoMigrate(&models.Occurrence{}, &models.OccurrenceEvent{}); err != nil {
		t.Fatalf("falha na migração: %v", err)
	}

	cleanup := func() {
		db.Where("tenant_id = ?", itTenantID).Delete(&models.OccurrenceEvent{})
		db.Where("tenant_id = ?", itTenantID).Delete(&models.Occurrence{})
	}
	cleanup()
	t.Cleanup(cleanup)

	return db
}

func itInconsistency(collabID int, tipo, desc string) domain.AuditInconsistency {
	return domain.AuditInconsistency{
		CollaboratorID:   collabID,
		CollaboratorName: fmt.Sprintf("Colaborador %d", collabID),
		Type:             tipo,
		Severity:         domain.SeverityCritical,
		Description:      desc,
	}
}

func countOccurrences(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&models.Occurrence{}).Where("tenant_id = ?", itTenantID).Count(&n).Error; err != nil {
		t.Fatalf("falha ao contar ocorrências: %v", err)
	}
	return n
}

// TestSyncsRepetidosNoMesmoDiaNaoDuplicamNoBanco é o item do roadmap: "rodar bateria de
// sync múltiplos no mesmo dia e validar no banco que não duplica".
func TestSyncsRepetidosNoMesmoDiaNaoDuplicamNoBanco(t *testing.T) {
	db := setupDB(t)
	svc := usecase.NewReconcilerService(repositories.NewOccurrenceRepository(db))

	fresh := []domain.AuditInconsistency{
		itInconsistency(1, "Almoço Reduzido", "O intervalo foi de apenas 43 minutos."),
		itInconsistency(2, "Interjornada Curta", "O descanso entre jornadas foi de apenas 9h."),
		itInconsistency(3, "Batida Esquecida", "O colaborador encerrou o dia com número ímpar de marcações (3 batida(s))."),
	}

	const syncs = 10
	for i := 1; i <= syncs; i++ {
		if _, err := svc.Reconcile(itTenantID, itDay, fresh, time.Now()); err != nil {
			t.Fatalf("sync %d falhou: %v", i, err)
		}
	}

	if got := countOccurrences(t, db); got != int64(len(fresh)) {
		t.Fatalf("%d varreduras deveriam deixar %d ocorrências no banco, vieram %d", syncs, len(fresh), got)
	}

	var rows []models.Occurrence
	if err := db.Where("tenant_id = ?", itTenantID).Find(&rows).Error; err != nil {
		t.Fatalf("falha ao ler ocorrências: %v", err)
	}
	for _, row := range rows {
		if row.TimesSeen != syncs {
			t.Errorf("ocorrência %q: esperava times_seen=%d, veio %d", row.Type, syncs, row.TimesSeen)
		}
		if row.State != string(domain.OccurrenceOpen) {
			t.Errorf("ocorrência %q: esperava estado %q, veio %q", row.Type, domain.OccurrenceOpen, row.State)
		}
	}

	// O log registra decisões, não repetições: só as 3 criações.
	var eventos int64
	db.Model(&models.OccurrenceEvent{}).Where("tenant_id = ?", itTenantID).Count(&eventos)
	if eventos != int64(len(fresh)) {
		t.Errorf("esperava %d eventos (só as criações), vieram %d", len(fresh), eventos)
	}
}

// TestVarredurasConcorrentesNaoDuplicam cobre a corrida que o fake não alcança: dois
// workers reconciliando o mesmo tenant/dia ao mesmo tempo. Sem o índice único + ON
// CONFLICT, aqui apareceriam linhas duplicadas ou a varredura estouraria.
func TestVarredurasConcorrentesNaoDuplicam(t *testing.T) {
	db := setupDB(t)
	svc := usecase.NewReconcilerService(repositories.NewOccurrenceRepository(db))

	fresh := []domain.AuditInconsistency{
		itInconsistency(1, "Almoço Reduzido", "O intervalo foi de apenas 43 minutos."),
		itInconsistency(2, "Hora Extra Excedente", "Realizou 2h30min de horas extras."),
	}

	const workers = 4
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Reconcile(itTenantID, itDay, fresh, time.Now()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("varredura concorrente falhou: %v", err)
	}
	if got := countOccurrences(t, db); got != int64(len(fresh)) {
		t.Fatalf("esperava %d ocorrências após %d varreduras simultâneas, vieram %d", len(fresh), workers, got)
	}
}

// TestCicloDeVidaNoBanco percorre a máquina de estados inteira contra o Postgres.
func TestCicloDeVidaNoBanco(t *testing.T) {
	db := setupDB(t)
	repo := repositories.NewOccurrenceRepository(db)
	svc := usecase.NewReconcilerService(repo)

	almoco := func(minutos int) []domain.AuditInconsistency {
		return []domain.AuditInconsistency{
			itInconsistency(1, "Almoço Reduzido", fmt.Sprintf("O intervalo foi de apenas %d minutos.", minutos)),
		}
	}

	readOne := func() domain.Occurrence {
		t.Helper()
		got, err := repo.ListByTenantAndDate(itTenantID, itDay)
		if err != nil {
			t.Fatalf("falha ao listar: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("esperava 1 ocorrência, vieram %d", len(got))
		}
		return got[0]
	}

	// 1. Nasce aberta.
	if _, err := svc.Reconcile(itTenantID, itDay, almoco(43), time.Now()); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got := readOne().State; got != domain.OccurrenceOpen {
		t.Fatalf("esperava %q, veio %q", domain.OccurrenceOpen, got)
	}

	// 2. Valor muda -> atualizada.
	if _, err := svc.Reconcile(itTenantID, itDay, almoco(20), time.Now()); err != nil {
		t.Fatalf("erro: %v", err)
	}
	occ := readOne()
	if occ.State != domain.OccurrenceUpdated {
		t.Fatalf("esperava %q, veio %q", domain.OccurrenceUpdated, occ.State)
	}

	// 3. Some da apuração -> resolvida automaticamente.
	if _, err := svc.Reconcile(itTenantID, itDay, nil, time.Now()); err != nil {
		t.Fatalf("erro: %v", err)
	}
	if got := readOne(); got.State != domain.OccurrenceResolvedAuto || got.ResolvedAt == nil {
		t.Fatalf("esperava resolvida automaticamente com data, veio %q / %v", got.State, got.ResolvedAt)
	}

	// 4. Volta -> reaberta como atualizada.
	if _, err := svc.Reconcile(itTenantID, itDay, almoco(20), time.Now()); err != nil {
		t.Fatalf("erro: %v", err)
	}
	occ = readOne()
	if occ.State != domain.OccurrenceUpdated {
		t.Fatalf("esperava reabertura em %q, veio %q", domain.OccurrenceUpdated, occ.State)
	}

	// 5. Ignorada manualmente -> não ressuscita nas varreduras seguintes.
	if err := repo.Ignore(occ.ID, "abonado pelo RH", nil); err != nil {
		t.Fatalf("falha ao ignorar: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.Reconcile(itTenantID, itDay, almoco(20), time.Now()); err != nil {
			t.Fatalf("erro: %v", err)
		}
	}
	if got := readOne().State; got != domain.OccurrenceResolvedManual {
		t.Fatalf("ocorrência ignorada deveria permanecer %q, veio %q", domain.OccurrenceResolvedManual, got)
	}

	// O log conta a história inteira, sem perder nenhuma transição.
	events, err := repo.ListEvents(occ.ID)
	if err != nil {
		t.Fatalf("falha ao ler eventos: %v", err)
	}
	want := []domain.OccurrenceEventType{
		domain.EventCreated, domain.EventUpdated, domain.EventResolvedAuto,
		domain.EventReopened, domain.EventResolvedManual,
	}
	if len(events) != len(want) {
		t.Fatalf("esperava %d eventos, vieram %d: %+v", len(want), len(events), events)
	}
	for i, w := range want {
		if events[i].Type != w {
			t.Errorf("evento %d: esperava %q, veio %q", i, w, events[i].Type)
		}
	}
}
