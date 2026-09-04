# Geti Secullum Compliance

**O radar de compliance trabalhista que o Secullum não tem.** Camada de auditoria sobre o
Secullum Ponto Web (ERP de RH / ponto eletrônico): consome as marcações de ponto via API,
aplica regras de conformidade da CLT diariamente, aponta inconsistências e avisa o gestor
responsável pelo WhatsApp — no mesmo dia, antes de virar passivo trabalhista.

Versão atual: veja [`VERSION`](./VERSION). Histórico de mudanças em
[`CHANGELOG.md`](./CHANGELOG.md).

---

## Sumário

- [O que este sistema faz](#o-que-este-sistema-faz)
- [Para quem](#para-quem)
- [Arquitetura, em uma frase](#arquitetura-em-uma-frase)
- [Stack](#stack)
- [Rodando localmente](#rodando-localmente)
- [Estrutura do repositório](#estrutura-do-repositório)
- [Documentação](#documentação)
- [Licença](#licença)
- [Segurança](#segurança)

## O que este sistema faz

O Secullum é a fonte da verdade das marcações de ponto — mas ele não avalia se essas
marcações violam a legislação trabalhista, nem avisa ninguém quando isso acontece. Este
sistema faz a parte que falta:

1. **Sincroniza** colaboradores, horários, equipamentos e batidas da Secullum para um
   espelho local.
2. **Audita** diariamente (e sob demanda, para qualquer período) contra regras da CLT:
   interjornada mínima, intervalo intrajornada, limite de horas extras, batida esquecida,
   trabalho em dia de folga/DSR — sete tipos de inconsistência, com severidade
   configurável por tenant.
3. **Persiste** o resultado como ocorrências com identidade estável e ciclo de vida
   próprio (aberta → atualizada/resolvida automaticamente/ignorada/tratada) — não como
   uma lista que se redesenha do zero a cada varredura.
4. **Avisa** o gestor responsável via WhatsApp (Evolution API) quando algo muda.
5. **Registra o tratamento**: o produto não é só diagnóstico — permite dar desfecho real a
   uma inconsistência (tratativa, com justificativa e anexo), consultar histórico,
   comparar ranking de exposição entre colaboradores e filiais, e conduzir a revisão
   mensal antes de considerar uma competência encerrada.

## Para quem

Gestores de RH e administradores de empresas clientes da Geti Soluções que já usam o
Secullum Ponto Web — perfil não técnico, painel autoexplicativo, uso tanto no escritório
(desktop) quanto em movimento (celular). Multi-tenant: a mesma instância atende vários
clientes, com quatro papéis de acesso (Super Admin, RH, Gestor, Diretoria) que definem o
que cada pessoa vê e pode fazer.

## Arquitetura, em uma frase

Backend em **Go** (arquitetura em camadas: domínio → casos de uso → infraestrutura →
interface HTTP) mais um worker assíncrono via **RabbitMQ**, um frontend em **React + Vite
+ TypeScript**, persistência em **PostgreSQL** via GORM, e duas integrações externas: a API
do **Secullum** (fonte dos dados de ponto) e a **Evolution API** (envio de WhatsApp).
Detalhe completo em [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md).

## Stack

| Camada | Tecnologia |
|---|---|
| Backend | Go, [Gin](https://gin-gonic.com/) (HTTP), [GORM](https://gorm.io/) (ORM) |
| Banco de dados | PostgreSQL |
| Mensageria | RabbitMQ (auditoria assíncrona, notificações, provisionamento) |
| Autenticação | JWT |
| Frontend | React, TypeScript, Vite |
| Integrações externas | API do Secullum Ponto Web, Evolution API (WhatsApp) |
| Deploy | Docker Compose (`infrastructure/`), Traefik em produção |

## Rodando localmente

Pré-requisitos: Docker e Docker Compose.

```bash
cd infrastructure
cp .env.example .env
# preencha SECULLUM_USERNAME/PASSWORD (ou SECULLUM_API_TOKEN), JWT_SECRET e
# SEED_ADMIN_EMAIL/PASSWORD no .env antes de subir — ver comentários no próprio arquivo

docker compose -f docker-compose.local.yml up -d --build
```

- Painel: <http://localhost:5500>
- API: <http://localhost:8080>
- RabbitMQ (management): <http://localhost:15672>

Para desenvolver o frontend com hot reload, rode `npm run dev` dentro de `frontend/`
(porta 5173) em vez de usar o container do painel.

O backend roda `AutoMigrate` do GORM na subida — não há passo manual de migration. Um
usuário super admin é criado automaticamente a partir de `SEED_ADMIN_EMAIL`/
`SEED_ADMIN_PASSWORD`, já que o cadastro (`POST /auth/register`) exige estar autenticado.

## Estrutura do repositório

```
backend/          API em Go (domain / usecase / infrastructure / interface)
frontend/         Painel administrativo (React + Vite + TS)
infrastructure/   Docker Compose (local e produção), variáveis de ambiente
docs/             Contratos de backend/frontend, arquitetura, decisões de domínio
CONTEXT.md        Glossário de domínio (termos do negócio, Secullum vs. sistema)
```

## Documentação

- [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md) — arquitetura do sistema, camadas,
  fluxo de dados, integrações.
- [`docs/ROUTES.md`](./docs/ROUTES.md) — todas as rotas HTTP expostas pela API.
- [`docs/SERVICES.md`](./docs/SERVICES.md) — os serviços de domínio (`usecase`) e o que
  cada um orquestra.
- [`docs/CONTROLLERS.md`](./docs/CONTROLLERS.md) — os handlers HTTP e as rotas que cada
  um atende.
- [`CONTEXT.md`](./CONTEXT.md) — glossário de domínio: os termos do negócio e onde a
  terminologia da Secullum diverge da terminologia deste sistema.
- [`docs/documento-funcional-compliance.md`](./docs/documento-funcional-compliance.md) —
  o documento funcional do ciclo de evolução atual (features em construção), cruzado com
  o estado real do código.
- Os demais arquivos numerados em `docs/` (`00_` a `12_`) são contratos específicos de
  cada área (auth, papéis, colaboradores, ocorrências, revisão mensal etc.), escritos à
  medida que cada feature foi especificada.

## Licença

[GNU Affero General Public License v3.0](./LICENSE.md). Resumo: você pode usar, estudar,
modificar e redistribuir livremente — mas se modificar e oferecer como serviço via rede
(mesmo sem distribuir um binário), os usuários desse serviço têm direito ao código-fonte
correspondente, incluindo suas modificações.

## Segurança

Encontrou uma vulnerabilidade? **Não abra uma issue pública.** Veja
[`SECURITY.md`](./SECURITY.md) para o canal de contato e o que incluir no relato.
