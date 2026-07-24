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

	router := SetupRouter(nil, nil)

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
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("rota ausente: %s", w)
		}
	}
}
