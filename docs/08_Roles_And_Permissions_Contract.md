# Papéis e permissões — contrato backend ↔ frontend

Hoje o sistema tem **dois** níveis de acesso (`users.is_super_admin`: super admin ou
membro comum), e quem tem vínculo com um tenant **pode tudo** dentro dele — ver
`docs/05_Auth_Backend_Contract.md`, seção 7, que já registrava isso como próximo passo.

Este documento especifica os **quatro** papéis pedidos, o modelo de dados, o enforcement
no backend e o que o frontend precisa receber para esconder o que o usuário não pode
fazer. É o contrato a implementar — nada aqui está no código ainda.

---

## 1. Os quatro papéis

| Papel | Escopo | Para quem | Resumo |
|---|---|---|---|
| **Super Admin** | Global (`users.is_super_admin`) | Equipe Geti | O de hoje. Único com Moderação e Empresa. |
| **RH** | Por tenant (`user_tenants.role`) | Operação do cliente | Tudo dentro do tenant, menos Moderação e Empresa. |
| **Gestor** | Por tenant | Chefia de área | Indicadores, colaboradores, ocorrências e advertências. Não mexe em configuração. |
| **Diretoria** | Por tenant | Alto escalão / demonstração | Só leitura. A única ação permitida é disparar uma auditoria do fechamento. |

### Por que o papel vive no vínculo, não no usuário

`Super Admin` é global e continua onde está. Os outros três vão em **`user_tenants.role`**,
não em `users`, porque a mesma pessoa pode ter posições diferentes em clientes
diferentes — a Geti opera vários tenants, e um consultor pode ser RH num cliente e
Diretoria em outro. Papel no usuário tornaria isso impossível sem duplicar conta.

Consequência importante para o frontend (seção 4): **o papel não cabe no JWT**. O token é
global e o papel é por tenant, então ele viaja junto do tenant, não da sessão.

---

## 2. Hierarquia — os papéis são estritamente aninhados

Diretoria ⊂ Gestor ⊂ RH ⊂ Super Admin. Tudo que a Diretoria pode, o Gestor também pode;
tudo que o Gestor pode, o RH também pode. Não existe permissão que um papel de baixo
tenha e um de cima não.

Isso permite tratar permissão como **nível ordenado** em vez de matriz de permissões — a
checagem vira uma comparação, e não há combinação inválida possível:

```go
// backend/internal/domain/Role.go (novo)
package domain

type Role string

const (
    RoleDiretoria Role = "diretoria"
    RoleGestor    Role = "gestor"
    RoleRH        Role = "rh"
)

// Level ordena os papéis para a checagem de "papel mínimo". Super admin não entra
// aqui: ele é global (users.is_super_admin) e passa por cima de qualquer nível.
func (r Role) Level() int {
    switch r {
    case RoleRH:
        return 3
    case RoleGestor:
        return 2
    case RoleDiretoria:
        return 1
    }
    return 0 // papel desconhecido: nega tudo, nunca libera por omissão
}

func (r Role) Valid() bool { return r.Level() > 0 }

// AtLeast responde se este papel satisfaz o papel mínimo exigido por uma rota.
func (r Role) AtLeast(min Role) bool { return r.Level() >= min.Level() }
```

Se um dia aparecer um pedido que quebre o aninhamento ("pode tudo, menos conectar o
WhatsApp"), aí sim vira matriz de permissões — e estes quatro papéis passam a ser
*presets* de conjuntos de permissões, sem quebrar o que estiver implementado.

---

## 3. Modelo de dados

Um campo novo na tabela de vínculo. Nenhuma tabela nova.

```go
// backend/internal/infrastructure/database/models/UserTenant.go
type UserTenant struct {
    // ... campos atuais ...
    Role string `gorm:"type:varchar(20);not null;default:'rh'" json:"role"`
}
```

```sql
-- Migration
ALTER TABLE user_tenants
  ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'rh';

ALTER TABLE user_tenants
  ADD CONSTRAINT user_tenants_role_check
  CHECK (role IN ('rh', 'gestor', 'diretoria'));
```

**`DEFAULT 'rh'` é deliberado:** todo vínculo que já existe hoje dá acesso total ao
tenant. Migrar para `'rh'` preserva exatamente o comportamento atual — ninguém perde
acesso no dia do deploy. Rebaixar quem for Gestor/Diretoria é decisão do super admin,
feita depois pela tela.

O default **não** deve valer para vínculos novos criados pela API: ali o papel é
obrigatório no corpo (seção 7), justamente para forçar a escolha consciente na tela de
cadastro.

### Contrato de domínio

```go
// backend/internal/domain/UserTenant.go
type UserTenantRepository interface {
    AddUserToTenant(userID uint, tenantID int, role Role) error   // ganha role
    RemoveUserFromTenant(userID uint, tenantID int) error
    HasAccess(userID uint, tenantID int) (bool, error)
    GetRole(userID uint, tenantID int) (Role, error)              // NOVO
    UpdateRole(userID uint, tenantID int, role Role) error        // NOVO
    ListTenantsForUser(userID uint) ([]*Tenant, error)
    ListUsersForTenant(tenantID int) ([]UserWithRole, error)      // passa a devolver o papel
}

// UserWithRole é o usuário junto do papel que ele tem NAQUELE tenant — o mesmo usuário
// aparece com papéis diferentes em tenants diferentes.
type UserWithRole struct {
    User User `json:"user"`
    Role Role `json:"role"`
}
```

`GetRole` para um super admin deve devolver um papel efetivo de acesso total mesmo sem
linha em `user_tenants` — ver seção 9.

---

## 4. Como o papel chega ao frontend

O JWT continua carregando **só** `user_id`, `email` e `is_super_admin`. Nada muda em
`auth/jwt.go`.

O papel vem junto do tenant, em dois lugares:

### 4.1 `POST /api/v1/auth/login`

A resposta hoje devolve `tenant_ids` para o front montar o seletor sem uma segunda
chamada. Passa a devolver os tenants **com o papel em cada um**:

```jsonc
{
  "token": "eyJ...",
  "user": { "id": 4, "name": "Ana", "email": "ana@cliente.com", "is_super_admin": false },
  "tenants": [
    { "id": 1, "name": "Cliente A", "role": "rh" },
    { "id": 7, "name": "Cliente B", "role": "diretoria" }
  ]
}
```

Para um **super admin**, `tenants` vem com todos os tenants e `"role": "super_admin"` em
cada um — assim o front tem um campo só para ler, sem precisar cruzar com
`is_super_admin` em cada tela.

> Manter `tenant_ids` na resposta durante a transição evita quebrar o front antes do
> deploy conjunto; pode sair depois.

### 4.2 `GET /api/v1/tenants`

Mesma coisa: cada item da lista ganha `role`, com o papel do usuário autenticado naquele
tenant. É o que o `AppShell` usa quando o usuário troca de cliente no seletor — o papel
muda junto com o tenant ativo, sem novo login.

```jsonc
{ "tenants": [ { "id": 1, "name": "Cliente A", "active": true, "role": "gestor" } ] }
```

**Regra para o front:** o papel efetivo é sempre o do **tenant ativo**. Trocar de tenant
no seletor pode mudar o que a tela mostra — inclusive esconder itens de menu que estavam
visíveis no cliente anterior. O `AppShell` já é o dono do tenant ativo, então é ele que
deve expor o papel para as páginas (mesmo caminho do `useTenant()` de hoje).

---

## 5. Matriz de permissões por rota

"Papel mínimo" = quem tem esse papel **ou superior** passa. Super admin passa em tudo.

### 5.1 Rotas globais (fora de um tenant)

| Método | Rota | Papel mínimo | Observação |
|---|---|---|---|
| POST | `/auth/login` | público | — |
| POST | `/auth/register` | **Super Admin** | inalterado |
| GET | `/users` | **Super Admin** | inalterado |
| GET | `/users/:id` | próprio ou Super Admin | inalterado |
| GET | `/users/:id/tenants` | próprio ou Super Admin | inalterado |
| PUT | `/users/:id/email` | próprio ou Super Admin | inalterado |
| PUT | `/users/:id/password` | próprio ou Super Admin | inalterado |
| PATCH | `/users/:id/activate` | **Super Admin** | inalterado |
| PATCH | `/users/:id/deactivate` | **Super Admin** | inalterado |
| PATCH | `/users/:id/super-admin` | **Super Admin** | **NOVO** — seção 7.3 |
| DELETE | `/users/:id` | **Super Admin** | inalterado |
| GET | `/tenants` | Diretoria | filtrado por vínculo, agora com `role` |
| POST | `/tenants` | **Super Admin** | inalterado |
| PUT | `/tenants/:id` | **Super Admin** | inalterado — é a tela Empresa |
| PATCH | `/tenants/:id/activate` | **Super Admin** | inalterado |
| PATCH | `/tenants/:id/deactivate` | **Super Admin** | inalterado |
| DELETE | `/tenants/:id` | **Super Admin** | inalterado |
| POST | `/tenants/:id/users` | **Super Admin** | corpo ganha `role` — seção 7.1 |
| PATCH | `/tenants/:id/users/:userId/role` | **Super Admin** | **NOVO** — seção 7.2 |
| DELETE | `/tenants/:id/users/:userId` | **Super Admin** | inalterado |
| POST | `/audit/trigger` | Diretoria* | *restrição de forma — ver 5.4 |

### 5.2 Rotas sob `/tenants/:id` (tenant explícito na rota)

| Método | Rota | Papel mínimo | Tela |
|---|---|---|---|
| GET | `/tenants/:id` | Diretoria | AppShell (todos precisam) |
| POST | `/tenants/:id/sync` | Gestor | sincronizar colaboradores |
| GET | `/tenants/:id/settings` | **RH** | Avisos |
| PUT | `/tenants/:id/settings` | **RH** | Avisos |
| GET | `/tenants/:id/staffs` | **RH** | Gestores |
| POST | `/tenants/:id/staffs` | **RH** | Gestores |
| GET | `/tenants/:id/reports` | Diretoria | Indicadores, Situação por dia |
| GET | `/tenants/:id/reports/history` | Diretoria | Registro de execuções |
| GET | `/tenants/:id/occurrences` | Diretoria | Incidentes, histórico do colaborador |
| GET | `/tenants/:id/branches` | Diretoria | **filtro** de Incidentes e Colaboradores |
| POST | `/tenants/:id/branches` | **RH** | Filiais |
| GET | `/tenants/:id/warnings` | Diretoria | painel de advertências (leitura) |
| POST | `/tenants/:id/warnings` | Gestor | criar advertência |
| GET | `/tenants/:id/collaborators` | Diretoria | Colaboradores |
| GET | `/tenants/:id/collaborators/:secullumId/prefill` | Diretoria | leitura pura |
| GET | `/tenants/:id/whatsapp/status` | **RH** | WhatsApp |
| POST | `/tenants/:id/whatsapp/instance` | **RH** | WhatsApp |
| DELETE | `/tenants/:id/whatsapp/instance` | **RH** | WhatsApp |
| GET | `/tenants/:id/users` | **RH** | quem tem acesso a este cliente |

> `GET /branches` fica em Diretoria de propósito: a Diretoria **não** vê a tela Filiais,
> mas os filtros por filial em Incidentes e Colaboradores dependem dessa lista. Permissão
> de API e item de menu são coisas diferentes — ver seção 8.

### 5.3 Rotas por recurso (o tenant sai do registro, não da URL)

Estas hoje checam acesso **dentro do handler**; a checagem de papel entra no mesmo ponto
(seção 6.2).

| Método | Rota | Papel mínimo |
|---|---|---|
| PUT | `/staffs/:staffId` | **RH** |
| DELETE | `/staffs/:staffId` | **RH** |
| GET | `/occurrences/:occurrenceId/events` | Diretoria |
| PATCH | `/occurrences/:occurrenceId/ignore` | Gestor |
| GET | `/branches/:branchId` | Diretoria |
| PUT | `/branches/:branchId` | **RH** |
| DELETE | `/branches/:branchId` | **RH** |
| POST | `/branches/:branchId/devices` | **RH** |
| DELETE | `/branches/:branchId/devices/:deviceId` | **RH** |
| POST | `/branches/:branchId/payroll-numbers` | **RH** |
| DELETE | `/branches/:branchId/payroll-numbers/:payrollNumberId` | **RH** |
| GET | `/warnings/:warningId` | Diretoria |
| PUT | `/warnings/:warningId` | Gestor |
| PATCH | `/warnings/:warningId/status` | Gestor |
| DELETE | `/warnings/:warningId` | Gestor |

### 5.4 `POST /audit/trigger` — a exceção da Diretoria

A Diretoria pode "solicitar uma nova auditoria naquele momento", mas **só na forma sem
parâmetros** (fechamento de D-1, o botão "Auditar agora"):

| Forma do corpo | Papel mínimo |
|---|---|
| `{ "tenant_id": 1 }` — fechamento de D-1 | **Diretoria** |
| `{ "tenant_id": 1, "date": "..." }` — um dia específico | **Gestor** |
| `{ "tenant_id": 1, "start_date": ..., "end_date": ... }` — período | **Gestor** |

Motivo: uma auditoria de período grava até 62 relatórios e reconcilia 62 dias de
ocorrências de uma vez. Está longe de "mostrar o sistema sem risco de bagunçar a
operação", que é o propósito do papel. A checagem cabe em uma condição no
`AuditHandler.TriggerAudit`, depois de `resolvePeriod`.

---

## 6. Enforcement no backend

### 6.1 Rotas com `:id` de tenant na URL

Um middleware novo, irmão do `RequireTenantAccess` atual (que continua existindo para as
rotas de papel mínimo Diretoria — acesso e papel mínimo são checagens distintas):

```go
// backend/internal/interface/http/middleware/tenant_access.go

// RequireTenantRole exige vínculo com o tenant da rota E papel mínimo. Super admin
// passa direto, como em RequireTenantAccess.
func RequireTenantRole(repo domain.UserTenantRepository, param string, min domain.Role) gin.HandlerFunc {
    return func(c *gin.Context) {
        if isSuperAdmin(c) {
            c.Next()
            return
        }

        tenantID, err := strconv.Atoi(c.Param(param))
        if err != nil { /* 400, igual ao RequireTenantAccess */ }

        role, err := repo.GetRole(currentUserID(c), tenantID)
        if err != nil { /* 500 */ }
        if !role.Valid() {
            forbidden(c, "você não tem acesso a este tenant")
            return
        }
        if !role.AtLeast(min) {
            forbidden(c, "seu perfil de acesso não permite esta ação")
            return
        }

        c.Set(ContextRoleKey, role) // handlers podem ler o papel efetivo
        c.Next()
    }
}
```

No `router.go`, o grupo `tenantScoped` continua com `RequireTenantAccess` (piso
Diretoria) e as rotas que exigem mais ganham o middleware específico:

```go
tenantScoped.PUT("/settings", middleware.RequireTenantRole(userTenantRepo, "id", domain.RoleRH), settingsHandler.Update)
tenantScoped.POST("/warnings", middleware.RequireTenantRole(userTenantRepo, "id", domain.RoleGestor), warningHandler.Create)
```

### 6.2 Rotas por recurso (`/warnings/:warningId` e afins)

Aqui o tenant só é conhecido depois de carregar o registro, então a checagem fica no
handler — no mesmo ponto onde hoje se confere o acesso. Um helper evita repetir:

```go
// backend/internal/interface/http/handlers/common.go

// requireRole confere papel mínimo para um tenant descoberto dentro do handler.
// Substitui a checagem de acesso simples nas rotas que exigem mais que leitura.
func requireRole(c *gin.Context, repo domain.UserTenantRepository, op string, tenantID int, min domain.Role) error {
    if isSuperAdminCtx(c) {
        return nil
    }
    role, err := repo.GetRole(currentUserIDCtx(c), tenantID)
    if err != nil {
        return domain.NewInternal(op, "falha ao verificar perfil de acesso", err)
    }
    if !role.Valid() {
        return domain.NewForbidden(op, "você não tem acesso a este tenant")
    }
    if !role.AtLeast(min) {
        return domain.NewForbidden(op, "seu perfil de acesso não permite esta ação")
    }
    return nil
}
```

> **Ponto de atenção:** hoje existem ~15 handlers que fazem a checagem de acesso
> internamente. Cada um precisa trocar a chamada atual pelo `requireRole` com o papel da
> matriz 5.3. Um handler esquecido continua aceitando qualquer membro do tenant — e
> falha em silêncio, sem erro de compilação. Vale um teste por rota da 5.3 verificando
> que Diretoria recebe 403 onde deve.

---

## 7. Endpoints novos e alterados

### 7.1 `POST /api/v1/tenants/:id/users` — papel obrigatório

```jsonc
// Requisição (super admin)
{ "user_id": 4, "role": "gestor" }

// 201
{ "message": "usuário vinculado ao tenant", "user_id": 4, "tenant_id": 1, "role": "gestor" }
```

`role` é **obrigatório** e precisa ser um de `rh` | `gestor` | `diretoria`. Valor ausente
ou inválido → `400 VALIDATION`, sem cair no default da coluna: é aqui que a tela de
cadastro força a escolha consciente do papel.

### 7.2 `PATCH /api/v1/tenants/:id/users/:userId/role` — trocar o papel (NOVO)

```jsonc
// Requisição (super admin)
{ "role": "diretoria" }

// 200
{ "message": "perfil de acesso atualizado", "user_id": 4, "tenant_id": 1, "role": "diretoria" }
```

Sem endpoint de troca, mudar alguém de papel exigiria desvincular e vincular de novo.

### 7.3 `PATCH /api/v1/users/:id/super-admin` — promover/rebaixar (NOVO)

Resolve a pendência já registrada no doc 05 (hoje só dá para virar super admin via seed
ou UPDATE no banco).

```jsonc
// Requisição (super admin)
{ "is_super_admin": true }

// 200
{ "message": "usuário atualizado", "user": { "id": 4, "is_super_admin": true } }
```

Duas regras: um super admin **não pode rebaixar a si mesmo** (evita o sistema ficar sem
nenhum), e a alteração só vale no **próximo login** do alvo — o `is_super_admin` está
dentro do JWT já emitido, e não há revogação de token hoje. O front precisa avisar isso
na tela.

### 7.4 Respostas que ganham `role`

- `POST /auth/login` → `tenants[].role` (seção 4.1)
- `GET /tenants` → `tenants[].role` (seção 4.2)
- `GET /tenants/:id/users` → `users[].role`, para a tela listar quem é o quê

---

## 8. O que o frontend faz com isso

**Permissão de API e item de menu são decisões separadas.** A API libera `GET /branches`
para Diretoria porque os filtros dependem dele; o menu Filiais mesmo assim não aparece,
porque a tela é de edição e não teria nada de útil em modo leitura.

### 8.1 Navegação por papel

| Item de menu | Super Admin | RH | Gestor | Diretoria |
|---|:--:|:--:|:--:|:--:|
| Indicadores | ✅ | ✅ | ✅ | ✅ |
| Situação por dia | ✅ | ✅ | ✅ | ✅ |
| Registro de execuções | ✅ | ✅ | ✅ | ✅ |
| Colaboradores | ✅ | ✅ | ✅ | ✅ |
| Filiais | ✅ | ✅ | — | — |
| Gestores | ✅ | ✅ | — | — |
| Avisos | ✅ | ✅ | — | — |
| WhatsApp | ✅ | ✅ | — | — |
| Empresa | ✅ | — | — | — |
| Moderação | ✅ | — | — | — |

`/incidents` não tem item de menu (é destino dos atalhos de Indicadores) e segue a mesma
regra da tela de origem: visível para todos os papéis.

### 8.2 Ações dentro das telas que todos enxergam

| Ação | Onde | RH | Gestor | Diretoria |
|---|---|:--:|:--:|:--:|
| Auditar agora (D-1) | Situação por dia | ✅ | ✅ | ✅ |
| Auditar um dia | Situação por dia | ✅ | ✅ | — |
| Auditar um período | Situação por dia | ✅ | ✅ | — |
| Sincronizar colaboradores | Colaboradores | ✅ | ✅ | — |
| Ignorar ocorrência | Histórico do colaborador | ✅ | ✅ | — |
| Criar/editar advertência | WarningPanel | ✅ | ✅ | — |
| Mudar status de advertência | WarningPanel | ✅ | ✅ | — |

**Esconder, não desabilitar.** Botão desabilitado sem explicação gera chamado de suporte;
para a Diretoria, cuja proposta é justamente "mostrar o sistema sem risco", um botão
cinza que não faz nada é pior que botão nenhum. As telas de leitura devem parecer telas
de leitura.

### 8.3 Contrato mínimo que o front precisa

1. `role` em cada tenant no login e em `GET /tenants` — sem isso não dá para montar
   menu nenhum.
2. O `AppShell` expõe o papel do tenant ativo junto do tenant (`useTenant()`), e reavalia
   ao trocar de cliente no seletor.
3. `403` com `code: "FORBIDDEN"` e mensagem legível continua sendo a rede de segurança —
   o front esconde a ação, mas **o backend é quem decide**. Esconder botão é UX; a
   autorização de verdade é a da seção 6.

---

## 9. Regras de borda

**Super admin sem vínculo.** Passa por cima de qualquer checagem, como hoje. `GetRole`
para um super admin deve devolver acesso total mesmo sem linha em `user_tenants` — as
respostas de API usam o valor `"super_admin"` para o front ler um campo só.

**Usuário sem vínculo com o tenant.** Continua `403 FORBIDDEN` com a mensagem atual
("você não tem acesso a este tenant"). O papel só entra na conta depois que o vínculo
existe: primeiro *se* tem acesso, depois *quanto*.

**Papel desconhecido no banco.** `Level() == 0` → nega tudo. Uma linha corrompida ou de
uma versão futura nunca deve virar acesso liberado por omissão.

**Último RH de um tenant.** Rebaixar o único RH deixa o cliente sem ninguém que configure
Avisos e WhatsApp — mas o super admin ainda resolve tudo, então isso é um aviso na tela,
não um bloqueio no backend. Bloquear criaria um beco sem saída no dia em que o RH sair da
empresa.

**Usuário desativado** (`users.active = false`). Já é barrado no login, antes de qualquer
checagem de papel. Nada muda.

**Papel e token.** O papel **não** está no JWT, então trocar o papel de alguém vale na
hora — diferente do `is_super_admin` (7.3), que só vale no próximo login. Vale manter
essa assimetria explícita para o front não prometer o que não acontece.

---

## 10. Erros

| Situação | HTTP | `code` | Mensagem |
|---|---|---|---|
| Sem token / token inválido | 401 | `UNAUTHORIZED` | (as de hoje) |
| Sem vínculo com o tenant | 403 | `FORBIDDEN` | `você não tem acesso a este tenant` |
| Vínculo ok, papel insuficiente | 403 | `FORBIDDEN` | `seu perfil de acesso não permite esta ação` |
| Não é super admin | 403 | `FORBIDDEN` | `apenas o super admin pode realizar esta ação` |
| `role` ausente ou inválido no corpo | 400 | `VALIDATION` | `role deve ser rh, gestor ou diretoria` |

As duas mensagens de 403 são propositalmente diferentes: "não tem acesso" e "não pode
fazer isso" são problemas distintos, e o suporte precisa distinguir sem abrir o log.

---

## 11. Decisões que valem confirmar antes de implementar

1. **Gestor e Diretoria não veem Filiais/Gestores/Avisos/WhatsApp** — nem em leitura. A
   descrição do papel falava em "não conseguir alterar"; optei por esconder, porque essas
   telas são inteiramente de edição e não têm valor informativo em modo leitura. Se a
   ideia era deixar visível e travado, muda a seção 8.1 (a matriz de API na 5.2 não muda).
2. **Diretoria não dispara auditoria de dia específico nem de período** (5.4). Se
   "solicitar uma nova auditoria naquele momento" incluía escolher o dia, é só baixar
   `date` para Diretoria e manter só o período em Gestor.
3. **Gestor pode sincronizar colaboradores** (`POST /sync`). É leitura da Secullum, mas
   reescreve o espelho local — encaixei em Gestor por ser operação, não configuração.
4. **`GET /tenants/:id/users` em RH.** Deixei RH ver quem tem acesso ao próprio cliente.
   Se essa lista for considerada dado de moderação, sobe para Super Admin.
