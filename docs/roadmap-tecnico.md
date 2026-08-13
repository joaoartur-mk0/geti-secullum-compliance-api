# Roadmap Técnico — Secullum Compliance
**Backend:** João · **Frontend:** Sérgio
**Última revisão:** 13/08 — status conferido item a item contra o repositório (commits até `ec9a04c`)

> Este documento nasceu como plano fixo da semana de 30/07–05/08 (seção "Ciclo original", no fim,
> mantida como histórico com cada item marcado). A partir daqui ele passa a ser **status vivo +
> plano de ação**: é a primeira coisa a ler ao retomar o trabalho numa janela de contexto nova.

---

## Situação em 13/08

**22 de 27 itens do ciclo original entregues.** O que falta não são 5 itens soltos — são **2
bloqueadores raiz**; todo o resto pendente depende de um dos dois.

### Entregue desde o plano original

- **Segurança de acesso** — login real (JWT + bcrypt), toda a API protegida por token.
  `59fda88` · `docs/05_Auth_Backend_Contract.md`
- **Isolamento multi-tenant** — `User.IsSuperAdmin` (global) + vínculo N:N `user_tenants`;
  `RequireTenantAccess`/`RequireSuperAdmin` cobrem toda rota de tenant. Estava listado como
  "fora deste ciclo" e entrou antecipado. **Nota:** saiu binário (super admin vs. usuário comum),
  não os 3 níveis RH/Gestor/Admin do plano original — ver Bloqueador 2.
  `bc5bc34` `517495d`
- **Máquina de estados de ocorrência** — identidade estável (tenant+colaborador+data+tipo),
  estados `aberta/atualizada/resolvida_automatica/resolvida_manual`, log de eventos, dedup
  testado com Postgres real sob concorrência.
  `70ed981` · `docs/06_Occurrences_Backend_Contract.md`
- **Filiais** — model `filial 1—N aparelhos`, `filial 1—1 gestor`, N° folha, CRUD completo +
  tela [Filiais.tsx](../frontend/src/pages/Filiais.tsx).
- **Advertências** — fluxo `draft → enviada → assinada` (mão única), painel
  [WarningPanel.tsx](../frontend/src/components/WarningPanel.tsx) com contagem por status.
- **As 4 melhorias de UI/UX planejadas** — resumo por tipo, categorias/cores novas
  (`ALTERACAO_ESCALA`/`NAO_CONFIRMADA`), indicador de advertências, filtro por filial em
  Indicadores. Todas entregues.
- **Correções de consistência** — seletor de dia em Indicadores, badge "OK" preso em
  Colaboradores, aba Auditorias (histórico completo separado do Painel).
  `05c0b8d`
- **Bônus fora do plano** — agendamento automático da auditoria diária corrigido: o horário
  configurado em Avisos era salvo mas nunca lido; nenhum worker disparava sozinho. Bug real,
  não pendência do roadmap.
  `ec9a04c`

### Bloqueador 1 — campo de desligamento do colaborador

Colaboradores desligados continuam aparecendo em métricas e listagens. **Não é falta de
filtro** — o dado nunca chega ao nosso sistema: `secullumFuncionarioResponse` em
[client.go](../backend/internal/infrastructure/secullum/client.go) só mapeia
`Id/Nome/Cpf/Celular/NumeroFolha/Horario.Numero`; qualquer outro campo do JSON da Secullum é
descartado silenciosamente pelo `json.Unmarshal`. `docs/01_Secullum_API_Info.md` também não
documenta qual campo indica desligamento — a investigação prevista pro dia 30/07 nunca
avançou.

- [ ] **Backend:** inspecionar a resposta real do endpoint `Funcionarios` da Secullum e achar
      o campo de status/situação
- [ ] **Backend:** adicionar ao struct + `domain.Collaborator` + migration (AutoMigrate)
- [ ] **Backend:** expor em `GET /tenants/:id/collaborators`
- [ ] **Frontend:** filtrar desligados da listagem e das métricas (padrão de filtro já existe
      em outras telas — ex. filtro por filial em Indicadores)

*Também destrava:* o item de sexta 31/07 "aplicar filtro de funcionário desligado".

### Bloqueador 2 — modelo de papéis de acesso

Hoje só existe `User.IsSuperAdmin` (global) e o vínculo `user_tenants` sem coluna de papel —
ver [UserTenant.go](../backend/internal/domain/UserTenant.go). Não há RH/Gestor/Admin dentro
do tenant.

- [ ] **Alinhamento:** decidir quantos papéis existem e o que cada um esconde na UI
- [ ] **Backend:** coluna `role` em `user_tenants`; expor no login e em
      `GET /users/:id/tenants`
- [ ] **Frontend:** gating de menus/rotas por papel

Decidido em conversa: **o gating pode ficar só no front por enquanto** — sem middleware de
autorização por rota no back. É proteção de UX, não de segurança; o back só precisa guardar e
expor o valor do papel. Middleware de verdade fica pra quando o risco de alguém contornar via
DevTools/API direta importar.

*Também destrava:* o item de sexta 31/07 "gating de UI por role".

### Pendência sem bloqueio de dado

- [ ] **QA visual** das telas construídas pelo João junto com o backend (Filiais, Moderação,
      Auditorias, WarningPanel) — pelo código seguem o padrão do
      [PRODUCT.md](../PRODUCT.md) e reusam os componentes de
      [ui.tsx](../frontend/src/components/ui.tsx), mas não foram conferidas ao vivo
      (mobile + desktop). Conclusão da revisão de 13/08: não há indício de que precisem ser
      refeitas — só falta olhar rodando.

### Fora de escopo — sem mudança desde o plano original

- Leitura da escala mensal variável em si (a reconciliação já cobre a reclassificação quando
  a escala é corrigida na Secullum — falta só ler a escala)
- Assinatura digital da advertência (hash-256)

---

## Plano de ação — próximos passos

Ordem recomendada (sem data fixa — retomar do primeiro item não concluído):

1. **[Backend]** Campo de desligamento — investigar + implementar (Bloqueador 1)
2. **[Frontend]** Filtrar desligados assim que o campo estiver exposto
3. **[Alinhamento]** Decidir o modelo de papéis — quantos níveis, o que cada um esconde
4. **[Backend]** Expor `role` no vínculo `user_tenants`
5. **[Frontend]** Gating de menus/rotas por papel
6. **[Frontend]** QA visual das 4 telas novas (mobile + desktop) contra o PRODUCT.md
7. **Fechar o ciclo** — replanejar a próxima rodada; é o momento de decidir se leitura de
   escala variável e assinatura digital entram ou continuam no backlog

---

## Ciclo original (30/07 → 05/08) — histórico

> Mantido como registro do plano combinado. Cada item abaixo já está marcado com o status
> real; ver "Situação em 13/08" acima para o porquê de cada pendência.

### Quinta-feira, 30/07 — Segurança de acesso

**Backend (João)**
- [x] Autenticação real: hash de senha (bcrypt/argon2), verificação de credencial, emissão de sessão/token
- [x] Endpoint de login validando credencial de verdade (fim do "qualquer email/senha entra")
- [ ] Investigar no retorno do cadastro de funcionários o campo que indica desligamento — **ainda pendente, é o Bloqueador 1**

**Frontend (Sérgio)**
- [x] Ajustar tela de login para trabalhar com o novo endpoint (tratamento de erro de credencial inválida, guarda de sessão/token no client)
- [x] Revisar rotas protegidas: redirecionar pra login quando sessão expira/inexiste

~~⚠️ Prioridade máxima do ciclo — enquanto isso não sobe, qualquer pessoa com o link vê os dados da empresa em produção.~~ **Resolvido em `59fda88`.**

---

### Sexta-feira, 31/07 — Níveis de acesso e primeiros ajustes de dados

**Backend (João)**
- [ ] Modelo de `role` no usuário (RH / Gestor / Admin) + middleware de permissão nas rotas de escrita/configuração/varredura — **saiu binário (super admin vs. comum); ver Bloqueador 2**
- [ ] Aplicar filtro de funcionário desligado na listagem — **bloqueado pelo Bloqueador 1**
- [x] Ajustar auditoria de domingo: não aplicar regra de intervalo mínimo de 15 min nesse dia

**Frontend (Sérgio)**
- [ ] Gating de UI por role: esconder configurações pra Gestor; esconder edição pra Admin (view + disparo de varredura apenas) ⏳ — **bloqueado pelo Bloqueador 2**
- [ ] Conferir que a listagem para de exibir desligados assim que o filtro subir no back — **bloqueado pelo Bloqueador 1**

---

### Segunda e terça-feira, 03 e 04/08 — Reconciliação de ocorrências + base de filiais

**Backend (João)**
- [x] Identidade estável por ocorrência (colaborador + data + tipo) com estados: `aberta`, `atualizada`, `resolvida_automatica`, `resolvida_manual`
- [x] Lógica de comparação a cada sync: ocorrência repetida não duplica; ocorrência que some vira `resolvida_automatica`; ocorrência com valor novo atualiza e reavalia
- [x] Esse mecanismo cobre também a reclassificação de trocas na escala mensal variável — nasce como categoria operacional (`OPERACIONAL`/`ALTERACAO_ESCALA`) e se resolve sozinha se a escala for corrigida
- [x] Modelo de dados de filiais: `filial 1—N aparelho`, `filial 1—1 gestor (nome, telefone)`, aparelho pertence a uma única filial
- [x] Endpoints de CRUD de filial (vínculo aparelho/N° folha, dados do gestor)

**Frontend (Sérgio)**
- [x] Painel de Indicadores — **UI/UX 1:** melhorar visualização do resumo de inconsistências por tipo
- [x] **UI/UX 2:** novas categorias de aviso com cores próprias além de Crítico/Alerta/Operacional
- [x] Início da tela de filiais (CRUD básico: listar, cadastrar, vincular aparelho/N° folha, gestor)

~~⚠️ Maior complexidade técnica do ciclo — se atrasar, é o item que consome a margem de 06–07/08.~~ **Entregue — foi o item de maior risco e saiu completo.**

---

### Quarta-feira, 05/08 — Página de colaborador e testes

**Backend (João)**
- [x] Endpoint de "ignorar ocorrência" (marca `resolvida_manual`)
- [x] Endpoint retornando horário fixo (Secullum) e filial (via N° folha/aparelho) prontos para autopreenchimento
- [x] Endpoint de registro de advertência: criar/atualizar status (`draft` / `enviada` / `assinada`) — sem hash por enquanto
- [x] Rodar bateria de sync múltiplos no mesmo dia e validar no banco que não duplica — coberto por teste de integração com Postgres real e concorrência

**Frontend (Sérgio)**
- [x] Botão de ignorar na tela de colaborador, consumindo o endpoint novo
- [x] Autopreenchimento de horário fixo e filial na página de colaborador
- [x] Form de advertência (draft/send/sign) — UI de controle simples
- [x] **UI/UX 3:** indicador de advertências enviadas x confirmadas
- [x] **UI/UX 4:** visualização por filial no painel (filtro/seleção de filial nos indicadores e na listagem)

---

## Fora deste ciclo (não muda)
- Leitura da escala mensal variável definida pelo gestor (mecanismo de reconciliação já cobre a reclassificação; falta ainda a leitura/verificação da escala em si)
- ~~Isolamento multi-tenant completo~~ **entregue antecipado, ver "Situação em 13/08"**
- Assinatura digital da advertência (hash-256)
