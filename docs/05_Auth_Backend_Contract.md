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

## 2. Cadastro de usuários (`/auth/register`) exige login

Diferente do login, o **cadastro de novos usuários é uma rota protegida** — só um
usuário já autenticado pode criar outro. Não existe endpoint público de "criar minha
conta". Isso é proposital: o painel é interno, não um SaaS de auto-cadastro.

O **primeiro usuário** (super admin) é criado por **seed no backend**, via variáveis de
ambiente na subida do servidor (`SEED_ADMIN_EMAIL`, `SEED_ADMIN_PASSWORD`, opcionalmente
`SEED_ADMIN_NAME`). O seed é idempotente: se o e-mail já existir, não faz nada. Sem essas
variáveis definidas, o seed é pulado silenciosamente (logado no console).

> **Não existe diferenciação de papel (role/admin) no modelo hoje** — qualquer usuário
> autenticado pode cadastrar outros usuários via `/auth/register`. Se o painel precisar
> restringir quem pode cadastrar/gerenciar usuários, isso é um próximo passo (adicionar
> um campo `role` em `User` e checar no handler), ainda não implementado.

## 3. Endpoints

| Método | Rota                        | Auth? | Descrição                          |
|--------|-----------------------------|-------|-------------------------------------|
| POST   | `/api/v1/auth/login`        | Não   | `{ email, password }` → `{ token, user }` |
| POST   | `/api/v1/auth/register`     | Sim   | `{ name, email, password }` → cria usuário |
| GET    | `/api/v1/users`             | Sim   | Lista usuários                      |
| GET    | `/api/v1/users/:id`         | Sim   | Busca usuário por id                |
| PUT    | `/api/v1/users/:id/email`   | Sim   | `{ email }` → atualiza e-mail       |
| PUT    | `/api/v1/users/:id/password`| Sim   | `{ password }` → atualiza senha     |
| DELETE | `/api/v1/users/:id`         | Sim   | Exclui usuário                      |

`password` no cadastro/atualização exige **mínimo de 8 caracteres** (validação no
backend, `binding:"min=8"`).

### Exemplo — login

```
POST /api/v1/auth/login
{ "email": "admin@empresa.com", "password": "minhasenha123" }

200 OK
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": { "id": 1, "name": "Super Admin", "email": "admin@empresa.com" }
}
```

O frontend deve guardar o `token` (ex.: `localStorage`/cookie, a decidir) e reenviá-lo
em todas as chamadas subsequentes ao backend.

## 4. Formato de erro (401)

Segue o mesmo padrão dos demais erros da API:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "token de autenticação ausente" } }
```

Outras mensagens possíveis: `"token de autenticação inválido ou expirado"` (token
malformado, assinatura inválida ou `exp` vencido). Em ambos os casos o frontend deve
tratar como "sessão expirada" e mandar o usuário de volta para o login.

## 5. Peças no backend (para referência)

- `internal/auth/password.go` — hash/checagem de senha (bcrypt).
- `internal/auth/jwt.go` — geração/validação do token (`GenerateToken`/`ParseToken`).
- `internal/interface/http/middleware/auth.go` — `RequireAuth()`, o middleware do Gin
  que valida o header `Authorization` e injeta `user_id`/`email` no contexto da request.
- `internal/interface/http/handlers/user_handler.go` — handlers de `/auth/*` e `/users`.
- `cmd/api/main.go` (`seedSuperAdmin`) — criação do usuário inicial via env vars.

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
- **Sem roles/permissões**: todo usuário autenticado tem acesso igual a tudo, inclusive
  a criar outros usuários.
- **Sem "esqueci minha senha"**: troca de senha hoje só existe via
  `PUT /users/:id/password`, que já exige estar autenticado.
