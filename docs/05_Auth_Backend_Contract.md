# Login/Auth — Contrato para o Frontend

Este documento descreve a feature de autenticação adicionada ao backend, para o
frontend implementar a tela de login de acordo. Para o contrato formal (schemas,
todos os status possíveis), veja o **Swagger** (`/swagger`, tag **Autenticação**).

---

## 1. Como funciona

- **Mecanismo**: usuário/senha (`email` + `password`) → **JWT** assinado com HMAC-SHA256,
  válido por **24h**. Não há refresh token nesta primeira versão — expirado, é preciso
  logar de novo.
- **Senha**: hash com **bcrypt** (`golang.org/x/crypto/bcrypt`, custo padrão). Nunca
  trafega nem é persistida em texto puro; a API nunca devolve o campo `password`.
- **Onde entra o token**: em **toda** rota de `/api/v1`, exceto `/api/v1/auth/login`, o
  frontend deve enviar:
  ```
  Authorization: Bearer <token>
  ```
  Sem esse header (ou com token inválido/expirado), a API responde `401 UNAUTHORIZED`
  com o formato de erro padrão (ver seção 4).
- **`/health` também é público** (não exige token).
- **Claims do token**: `user_id`, `email` e `is_super_admin` (ver seção 2). O middleware
  `RequireAuth` injeta os três no contexto da requisição para os handlers/middlewares
  seguintes usarem — inclusive o de isolamento por tenant (seção 3).

## 2. Papéis: super admin vs. usuário vinculado a tenants

O modelo tem **dois níveis de acesso**, controlados pelo campo `User.IsSuperAdmin`:

- **Super admin** (`is_super_admin: true`): enxerga e gerencia **todos** os tenants,
  sem precisar de vínculo explícito. É quem cadastra tenants e usuários no sistema.
- **Usuário comum** (`is_super_admin: false`): só acessa os tenants aos quais foi
  **explicitamente vinculado** (tabela `user_tenants`, N:N — um usuário pode ter acesso
  a vários tenants, e um tenant pode ter vários usuários). Fora desses tenants, qualquer
  tentativa de acesso responde `403 FORBIDDEN` (ver seção 4) — é essa checagem que
  garante que os dados de um tenant nunca vazem para outro.

O **primeiro usuário** (super admin) é criado por **seed no backend**, via variáveis de
ambiente na subida do servidor (`SEED_ADMIN_EMAIL`, `SEED_ADMIN_PASSWORD`, opcionalmente
`SEED_ADMIN_NAME`). O seed é idempotente: se o e-mail já existir, não faz nada. Sem essas
variáveis definidas, o seed é pulado silenciosamente (logado no console).

### Cadastro de usuários e tenants exige super admin

- `POST /auth/register` (cadastro de usuário), `POST /tenants` (cadastro de tenant),
  `PUT /tenants/:id`, `PATCH /tenants/:id/deactivate` e a gestão do vínculo
  (`POST`/`DELETE /tenants/:id/users`) são ações administrativas globais — só um super
  admin autenticado pode chamá-las. Um usuário comum recebe `403 FORBIDDEN`.
- Novos usuários nascem sem tenant nenhum vinculado; é o super admin quem os associa via
  `POST /tenants/:id/users`.

### Vínculo usuário↔tenant

| Método | Rota | Auth? | Descrição |
|--------|------|:---:|-----------|
| POST | `/api/v1/tenants/:id/users` | Super admin | `{ user_id }` → vincula usuário ao tenant |
| DELETE | `/api/v1/tenants/:id/users/:userId` | Super admin | Remove o vínculo |
| GET | `/api/v1/tenants/:id/users` | Membro do tenant ou super admin | Lista usuários com acesso ao tenant |
| GET | `/api/v1/users/:id/tenants` | O próprio usuário ou super admin | Lista tenants aos quais o usuário tem acesso |

`GET /api/v1/tenants` também respeita o vínculo: super admin vê todos os tenants; um
usuário comum só vê os que lhe foram associados (nunca a lista completa).

## 3. Endpoints

| Método | Rota                        | Auth? | Descrição                          |
|--------|-----------------------------|-------|-------------------------------------|
| POST   | `/api/v1/auth/login`        | Não   | `{ email, password }` → `{ token, user, tenant_ids }` |
| POST   | `/api/v1/auth/register`     | Super admin | `{ name, email, password }` → cria usuário |
| GET    | `/api/v1/users`             | Super admin | Lista usuários                      |
| GET    | `/api/v1/users/:id`         | Próprio usuário ou super admin | Busca usuário por id |
| GET    | `/api/v1/users/:id/tenants` | Próprio usuário ou super admin | Lista tenants do usuário |
| PUT    | `/api/v1/users/:id/email`   | Próprio usuário ou super admin | `{ email }` → atualiza e-mail |
| PUT    | `/api/v1/users/:id/password`| Próprio usuário ou super admin | `{ password }` → atualiza senha |
| DELETE | `/api/v1/users/:id`         | Super admin | Exclui usuário                      |

`password` no cadastro/atualização exige **mínimo de 8 caracteres** (validação no
backend, `binding:"min=8"`). Rotas de vínculo usuário↔tenant estão na seção 2.

### Exemplo — login

```
POST /api/v1/auth/login
{ "email": "admin@empresa.com", "password": "minhasenha123" }

200 OK
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": { "id": 1, "name": "Super Admin", "email": "admin@empresa.com", "is_super_admin": true },
  "tenant_ids": []
}
```

`tenant_ids` vem vazio para um super admin (ele acessa todos os tenants sem precisar de
vínculo); para um usuário comum, traz os ids dos tenants vinculados — é com essa lista
que o frontend monta o seletor de tenant sem precisar de uma chamada extra logo após o
login.

O frontend deve guardar o `token` (ex.: `localStorage`/cookie, a decidir) e reenviá-lo
em todas as chamadas subsequentes ao backend.

## 4. Formato de erro (401 / 403)

Segue o mesmo padrão dos demais erros da API:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "token de autenticação ausente" } }
```

Outras mensagens possíveis: `"token de autenticação inválido ou expirado"` (token
malformado, assinatura inválida ou `exp` vencido). Em ambos os casos o frontend deve
tratar como "sessão expirada" e mandar o usuário de volta para o login.

Já o **403** aparece quando o token é válido mas o usuário não tem permissão para a
ação (não é super admin) ou não tem vínculo com o tenant acessado:

```json
{ "error": { "code": "FORBIDDEN", "message": "você não tem acesso a este tenant" } }
```

Outras mensagens possíveis: `"apenas o super admin pode realizar esta ação"` (rotas
administrativas globais) e `"você só pode acessar os seus próprios dados"` (rotas de
`/users/:id/...` de outro usuário). Diferente do 401, o frontend **não** deve deslogar
nesse caso — a sessão continua válida, só a ação não é permitida.

## 5. Peças no backend (para referência)

- `internal/auth/password.go` — hash/checagem de senha (bcrypt).
- `internal/auth/jwt.go` — geração/validação do token (`GenerateToken`/`ParseToken`),
  incluindo a claim `is_super_admin`.
- `internal/interface/http/middleware/auth.go` — `RequireAuth()`, o middleware do Gin
  que valida o header `Authorization` e injeta `user_id`/`email`/`is_super_admin` no
  contexto da request.
- `internal/interface/http/middleware/tenant_access.go` — `RequireSuperAdmin()`,
  `RequireSelfOrSuperAdmin()` e `RequireTenantAccess()`, os middlewares que aplicam as
  regras de permissão/isolamento descritas acima.
- `internal/domain/UserTenant.go` / `internal/infrastructure/database/repositories/user_tenant_repository.go`
  — o vínculo N:N entre `User` e `Tenant` (tabela `user_tenants`).
- `internal/interface/http/handlers/user_handler.go` — handlers de `/auth/*` e `/users`.
- `internal/interface/http/handlers/tenant_handler.go` — handlers de `/tenants/*`,
  incluindo a gestão do vínculo (`AddUser`/`RemoveUser`/`ListUsers`).
- `cmd/api/main.go` (`seedSuperAdmin`) — criação do usuário inicial via env vars
  (sempre com `IsSuperAdmin: true`).

## 6. Configuração (env vars)

| Variável              | Obrigatória? | Efeito                                          |
|------------------------|:---:|--------------------------------------------------|
| `JWT_SECRET`            | Recomendado em produção | Chave de assinatura do token. Sem ela, usa um valor fixo de desenvolvimento (`dev-secret-change-me`) — **não usar em produção**. |
| `SEED_ADMIN_EMAIL`      | Para criar o 1º usuário | E-mail do super admin criado na subida do servidor. |
| `SEED_ADMIN_PASSWORD`   | Para criar o 1º usuário | Senha do super admin (mín. 8 caracteres). |
| `SEED_ADMIN_NAME`       | Não | Nome do super admin (default: `"Super Admin"`). |

## 7. Em aberto / próximos passos

- **Sem refresh token**: ao expirar (24h), o usuário precisa logar de novo. Se o painel
  precisar de sessões mais longas, isso é o próximo passo natural.
- **Só dois níveis de papel**: super admin (acesso total) ou membro comum (acesso só
  aos tenants vinculados). Não há papel intermediário por tenant (ex.: "admin do
  tenant X, mas não super admin") — se o painel precisar disso, é o próximo passo
  natural (um campo `role` na própria tabela `user_tenants`).
- **Alterar `is_super_admin` de um usuário existente** não tem endpoint próprio hoje —
  só é setado na criação (via seed, sempre `true`) ou diretamente no banco.
- **Sem "esqueci minha senha"**: troca de senha hoje só existe via
  `PUT /users/:id/password`, que já exige estar autenticado.
