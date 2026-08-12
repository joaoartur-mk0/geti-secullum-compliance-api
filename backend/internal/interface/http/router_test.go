package http

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSetupRouter_RegistraRotasSemConflito garante que a árvore de rotas do gin é
// montada sem panic (conflitos de wildcard acontecem no registro, não na compilação)
// e que os endpoints do CRUD estão presentes. db/publisher nulos bastam: o registro
// de rotas não toca no banco nem no broker.
func TestSetupRouter_RegistraRotasSemConflito(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := SetupRouter(nil, nil, nil, "tenant", nil)

	got := make(map[string]bool)
	for _, ri := range router.Routes() {
		got[ri.Method+" "+ri.Path] = true
	}

	want := []string{
		"POST /api/v1/audit/trigger",
		"GET /api/v1/tenants",
		"POST /api/v1/tenants",
		"GET /api/v1/tenants/:id",
		"PUT /api/v1/tenants/:id",
		"PATCH /api/v1/tenants/:id/deactivate",
		"POST /api/v1/tenants/:id/sync",
		"GET /api/v1/tenants/:id/settings",
		"PUT /api/v1/tenants/:id/settings",
		"GET /api/v1/tenants/:id/staffs",
		"POST /api/v1/tenants/:id/staffs",
		"PUT /api/v1/staffs/:staffId",
		"DELETE /api/v1/staffs/:staffId",
		"GET /api/v1/tenants/:id/reports",
		"GET /api/v1/tenants/:id/collaborators",
		"GET /api/v1/tenants/:id/whatsapp/status",
		"POST /api/v1/tenants/:id/whatsapp/instance",
		"DELETE /api/v1/tenants/:id/whatsapp/instance",

		// Ocorrências (máquina de estados) e autopreenchimento
		"GET /api/v1/tenants/:id/occurrences",
		"PATCH /api/v1/occurrences/:occurrenceId/ignore",
		"GET /api/v1/occurrences/:occurrenceId/events",
		"GET /api/v1/tenants/:id/collaborators/:secullumId/prefill",

		// Filiais
		"GET /api/v1/tenants/:id/branches",
		"POST /api/v1/tenants/:id/branches",
		"GET /api/v1/branches/:branchId",
		"PUT /api/v1/branches/:branchId",
		"DELETE /api/v1/branches/:branchId",
		"POST /api/v1/branches/:branchId/devices",
		"DELETE /api/v1/branches/:branchId/devices/:deviceId",
		"POST /api/v1/branches/:branchId/payroll-numbers",
		"DELETE /api/v1/branches/:branchId/payroll-numbers/:payrollNumberId",

		// Advertências
		"GET /api/v1/tenants/:id/warnings",
		"POST /api/v1/tenants/:id/warnings",
		"GET /api/v1/warnings/:warningId",
		"PUT /api/v1/warnings/:warningId",
		"PATCH /api/v1/warnings/:warningId/status",
		"DELETE /api/v1/warnings/:warningId",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("rota ausente: %s", w)
		}
	}
}
