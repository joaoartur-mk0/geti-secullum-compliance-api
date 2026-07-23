# Frontend — Interface Administrativa

Painel de gestão do Secullum Compliance (Vite + React + TypeScript + Tailwind v4), conforme a
stack definida em `docs/00_Automation_Engineering_Documentation.md`. Substituiu o painel
estático de testes do backend que vivia nesta pasta (o histórico dele fica no git).

## Rodar em desenvolvimento (HMR)

```bash
cd frontend
npm install
npm run dev
# abre http://localhost:5173
```

Pré-requisito: backend no ar (`docker compose -f docker-compose.local.yml up -d` em
`infrastructure/`). A base URL padrão é `http://localhost:8080` e pode ser trocada na tela de
login em "Configuração avançada" (fica salva no navegador).

## Servir pelo compose local (porta 5500)

O serviço `frontend` do `docker-compose.local.yml` builda a imagem sozinho (multi-stage:
Node compila o Vite, nginx serve o resultado com fallback de SPA — ver `Dockerfile` e
`nginx.conf` nesta pasta):

```bash
cd infrastructure
docker compose -f docker-compose.local.yml up -d --build
# abre http://localhost:5500
```

Depois de mudar o código, repita o `up -d --build` para reconstruir a imagem.

## O que está implementado

- **Login** — provisório (aceita qualquer credencial e guarda sessão no `localStorage`).
  Troca por autenticação real quando o backend ganhar o modo multiempresa.
- **Primeiro acesso** — se não há tenant cadastrado, o painel guia o cadastro da empresa
  (`POST /tenants`).
- **Painel** — saúde da infra (`/health`), disparo de auditoria (`POST /audit/trigger`) e
  relatórios com inconsistências e severidades (`GET /tenants/{id}/reports`).
- **Gestores** — CRUD completo de responsáveis (`/tenants/{id}/staffs`, `/staffs/{id}`).
- **Avisos** — flags, severidades e horários de varredura (`/tenants/{id}/settings`).
- **Empresa** — corrigir nome e ID do banco Secullum (`PUT /tenants/{id}`), evitando reset de
  banco se o dado foi cadastrado errado no setup.
- **WhatsApp** — fluxo de conexão com a Evolution API **simulado** (o backend ainda não expõe
  esses endpoints); o estado vive só no navegador, marcado como prévia na própria tela.

## Estrutura

```
src/
├─ lib/          # api.ts (client tipado do swagger), types.ts, session.ts, format.ts
├─ components/   # ui.tsx (Button, Field, Toggle, badges, toasts, estados)
├─ layouts/      # AppShell.tsx (sidebar desktop + nav inferior mobile, contexto do tenant)
└─ pages/        # Login, Painel, Gestores, Avisos, WhatsApp
```

Design tokens (OKLCH) em `src/index.css`; diretrizes estratégicas em `../PRODUCT.md`.
