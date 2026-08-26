package secullum

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"backend/internal/domain"
)

func newTestClient(baseURL string) domain.SecullumService {
	return NewSecullumClient(Config{
		BaseURL:              baseURL,
		StaticToken:          "tok-teste",
		MaxRequestsPerMinute: 1000,
	})
}

// TestDo_RenovaTokenEm401 cobre o cenário real observado em produção: a Secullum
// invalida o token ANTES do TTL local (ex.: novo login na mesma conta). O client deve
// reautenticar e repetir a requisição uma única vez, transparente para o chamador.
func TestDo_RenovaTokenEm401(t *testing.T) {
	var authCalls, funcCalls int32

	mux := http.NewServeMux()
	mux.HandleFunc("/Token", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&authCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": fmt.Sprintf("tok-%d", n)})
	})
	mux.HandleFunc("/IntegracaoExterna/Funcionarios", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&funcCalls, 1)
		// O primeiro token emitido já foi "invalidado no servidor": rejeita com 401.
		if r.Header.Get("Authorization") == "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewSecullumClient(Config{
		BaseURL:              srv.URL,
		AuthURL:              srv.URL + "/Token",
		Username:             "user",
		Password:             "pass",
		MaxRequestsPerMinute: 1000,
	})

	if _, err := client.GetCollaborators(&domain.Tenant{SecullumDatabaseID: 1}); err != nil {
		t.Fatalf("esperava sucesso após renovação do token, veio: %v", err)
	}
	if n := atomic.LoadInt32(&authCalls); n != 2 {
		t.Errorf("autenticou %d vez(es), esperava 2 (login original + renovação pós-401)", n)
	}
	if n := atomic.LoadInt32(&funcCalls); n != 2 {
		t.Errorf("chamou funcionarios %d vez(es), esperava 2 (401 + retry)", n)
	}
}

// TestDo_TokenEstaticoNaoRenovaEm401: com token estático não há renovação possível —
// o 401 deve ser devolvido direto (sem loop de retry no client).
func TestDo_TokenEstaticoNaoRenovaEm401(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL) // modo token estático
	if _, err := client.GetCollaborators(&domain.Tenant{}); err == nil {
		t.Fatalf("esperava erro 401 com token estático")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("chamou o endpoint %d vez(es), esperava 1 (sem retry no modo estático)", n)
	}
}

func TestGetCollaborators_MapeiaHorarioNumero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("secullumidbancoselecionado"); got != "82720" {
			t.Errorf("header banco = %q, quer 82720", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Id": 3, "Nome": "Fulano", "Cpf": "111.111.111-11", "Celular": "",
			 "Horario": {"Numero": 8}}
		]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	tenant := &domain.Tenant{SecullumDatabaseID: 82720}

	collabs, err := client.GetCollaborators(tenant)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(collabs) != 1 {
		t.Fatalf("esperava 1 colaborador, veio %d", len(collabs))
	}
	if collabs[0].HorarioNumero != 8 {
		t.Errorf("HorarioNumero = %d, quer 8", collabs[0].HorarioNumero)
	}
	if collabs[0].SecullumID != 3 || collabs[0].Name != "Fulano" {
		t.Errorf("colaborador mapeado incorretamente: %+v", collabs[0])
	}
}

// TestGetHorario_ParseiaDiasReais usa uma resposta modelada na real (docs/response_horarios_by_id.json):
// array com 1 horário, cada dia com Entrada1/Saida1/Entrada2/Saida2 em "HH:MM" e Carga em minutos.
func TestGetHorario_ParseiaDiasReais(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("numero"); got != "8" {
			t.Errorf("query numero = %q, quer 8", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"Id": 8,
				"Dias": [
					{"DiaSemana": 0, "Entrada1": null, "Saida1": null, "Entrada2": null, "Saida2": null, "Carga": 0},
					{"DiaSemana": 1, "Entrada1": "08:00", "Saida1": "12:00", "Entrada2": "13:50", "Saida2": "17:10", "Carga": 440}
				]
			}
		]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	tenant := &domain.Tenant{SecullumDatabaseID: 82720}

	schedules, err := client.GetHorario(tenant, 8)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("esperava 2 dias, veio %d", len(schedules))
	}

	domingo := schedules[0]
	if domingo.DiaSemana != 0 || domingo.CargaMinutos != 0 || domingo.Entrada1 != "" {
		t.Errorf("domingo (folga) mapeado incorretamente: %+v", domingo)
	}

	segunda := schedules[1]
	if segunda.DiaSemana != 1 || segunda.CargaMinutos != 440 {
		t.Errorf("segunda: DiaSemana/CargaMinutos incorretos: %+v", segunda)
	}
	if segunda.Entrada1 != "08:00" || segunda.Saida1 != "12:00" || segunda.Entrada2 != "13:50" || segunda.Saida2 != "17:10" {
		t.Errorf("segunda: horários incorretos: %+v", segunda)
	}
}

func TestGetHorario_RespostaVazia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	schedules, err := client.GetHorario(&domain.Tenant{}, 999)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if schedules != nil {
		t.Errorf("esperava nil para número de horário inexistente, veio %+v", schedules)
	}
}

// TestGetDailyPunches_ParseiaMemoriaEFolga usa registros reais de um domingo
// (docs/payload_domingo_example.json): a jornada do dia vem nos campos Memoria* e é a
// fonte da carga esperada; marcadores de abono (FERIAS) não são batidas.
func TestGetDailyPunches_ParseiaMemoriaEFolga(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"FuncionarioId": 36, "Data": "2026-08-02T00:00:00",
			 "Entrada1": "06:56", "Saida1": "09:05", "Entrada2": "09:20", "Saida2": "13:38",
			 "Entrada3": null, "Saida3": null, "Entrada4": null, "Saida4": null,
			 "MemoriaEntrada1": "08:00", "MemoriaSaida1": "09:00",
			 "MemoriaEntrada2": "09:15", "MemoriaSaida2": "14:15",
			 "Folga": false, "Neutro": false},
			{"FuncionarioId": 78, "Data": "2026-08-02T00:00:00",
			 "Entrada1": "FERIAS", "Saida1": "FERIAS", "Entrada2": "FERIAS", "Saida2": "FERIAS",
			 "MemoriaEntrada1": "", "MemoriaSaida1": "",
			 "Folga": true, "Neutro": false}
		]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	punches, err := client.GetDailyPunches(&domain.Tenant{}, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(punches) != 2 {
		t.Fatalf("esperava 2 registros, veio %d", len(punches))
	}

	trabalhou := punches[0]
	worked, ok, err := trabalhou.WorkedMinutes()
	if err != nil || !ok || worked != 387 { // 2h09 + 4h18
		t.Errorf("trabalhado = %d ok=%v err=%v, quer 387", worked, ok, err)
	}
	expected, ok, err := trabalhou.ExpectedMinutes()
	if err != nil || !ok || expected != 360 { // 1h + 5h = 6h previstas
		t.Errorf("previsto = %d ok=%v err=%v, quer 360", expected, ok, err)
	}
	if intervalo, ok, _ := trabalhou.FirstBreak(); !ok || intervalo != 15 {
		t.Errorf("intervalo = %d ok=%v, quer 15", intervalo, ok)
	}

	abono := punches[1]
	if !abono.Folga {
		t.Errorf("registro de férias deveria vir com Folga=true")
	}
	if abono.PunchCount() != 0 {
		t.Errorf("marcador de abono não é batida: PunchCount = %d, quer 0", abono.PunchCount())
	}
	if _, ok, _ := abono.ExpectedMinutes(); ok {
		t.Errorf("Memoria vazia não deveria produzir carga prevista")
	}
}

// TestGetDailyPunches_MapeiaFonteDadosID cobre a chave de correlação usada no
// enriquecimento de equipamento/motivo: os campos FonteDadosIdEntradaN/SaidaN da
// resposta real de Batidas (docs/response_batidas_single_day_single_colaborator.json).
func TestGetDailyPunches_MapeiaFonteDadosID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"FuncionarioId": 7, "Data": "2026-07-01T00:00:00",
			 "Entrada1": "08:08", "Saida1": "11:42",
			 "FonteDadosIdEntrada1": 11911, "FonteDadosIdSaida1": 11914}
		]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	punches, err := client.GetDailyPunches(&domain.Tenant{}, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(punches) != 1 {
		t.Fatalf("esperava 1 registro, veio %d", len(punches))
	}

	par := punches[0].Marcacoes[0]
	if par.FonteDadosIDEntrada == nil || *par.FonteDadosIDEntrada != 11911 {
		t.Errorf("FonteDadosIDEntrada = %v, quer 11911", par.FonteDadosIDEntrada)
	}
	if par.FonteDadosIDSaida == nil || *par.FonteDadosIDSaida != 11914 {
		t.Errorf("FonteDadosIDSaida = %v, quer 11914", par.FonteDadosIDSaida)
	}
}

func TestGetCollaborators_MapeiaAdmissaoEDemissao(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Id": 1, "Nome": "Ativo", "Admissao": "2023-01-11T00:00:00", "Demissao": null},
			{"Id": 2, "Nome": "Demitido", "Admissao": "2020-05-01T00:00:00", "Demissao": "2025-05-12T00:00:00"}
		]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	collabs, err := client.GetCollaborators(&domain.Tenant{})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(collabs) != 2 {
		t.Fatalf("esperava 2 colaboradores, veio %d", len(collabs))
	}

	ativo := collabs[0]
	if ativo.Demitido || ativo.Demissao != nil {
		t.Errorf("colaborador sem Demissao deveria vir Demitido=false, veio %+v", ativo)
	}
	if ativo.Admissao == nil || ativo.Admissao.Format("2006-01-02") != "2023-01-11" {
		t.Errorf("Admissao mapeada incorretamente: %+v", ativo.Admissao)
	}

	demitido := collabs[1]
	if !demitido.Demitido {
		t.Errorf("colaborador com Demissao preenchida deveria vir Demitido=true")
	}
	if demitido.Demissao == nil || demitido.Demissao.Format("2006-01-02") != "2025-05-12" {
		t.Errorf("Demissao mapeada incorretamente: %+v", demitido.Demissao)
	}
}

func TestGetEquipamentos_Mapeia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Id": 1, "Descricao": "CONTROL ID MATRIZ", "EnderecoIP": "192.168.0.31"},
			{"Id": 6, "Descricao": "Control IDFace Matriz", "EnderecoIP": null}
		]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	equipments, err := client.GetEquipamentos(&domain.Tenant{ID: 5})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(equipments) != 2 {
		t.Fatalf("esperava 2 equipamentos, veio %d", len(equipments))
	}
	if equipments[0].SecullumID != 1 || equipments[0].Descricao != "CONTROL ID MATRIZ" || equipments[0].TenantID != 5 {
		t.Errorf("equipamento mapeado incorretamente: %+v", equipments[0])
	}
	if equipments[0].EnderecoIP == nil || *equipments[0].EnderecoIP != "192.168.0.31" {
		t.Errorf("EnderecoIP = %v, quer 192.168.0.31", equipments[0].EnderecoIP)
	}
	if equipments[1].EnderecoIP != nil {
		t.Errorf("EnderecoIP deveria ser nil para aparelho sem IP, veio %v", *equipments[1].EnderecoIP)
	}
}

func TestGetFonteDados_Mapeia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("DataInicio"); got != "2026-08-24" {
			t.Errorf("DataInicio = %q, quer 2026-08-24", got)
		}
		if got := r.URL.Query().Get("DataFim"); got != "2026-08-24" {
			t.Errorf("DataFim = %q, quer 2026-08-24", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Id": 347372, "EquipamentoId": 6, "Motivo": null}
		]`))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	items, err := client.GetFonteDados(&domain.Tenant{}, day, day)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("esperava 1 item, veio %d", len(items))
	}
	if items[0].ID != 347372 || items[0].EquipamentoID == nil || *items[0].EquipamentoID != 6 {
		t.Errorf("item mapeado incorretamente: %+v", items[0])
	}
	if items[0].Motivo != nil {
		t.Errorf("Motivo deveria ser nil, veio %v", *items[0].Motivo)
	}
}
