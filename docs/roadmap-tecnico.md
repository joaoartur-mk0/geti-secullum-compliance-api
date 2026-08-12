# Roadmap Técnico — Secullum Compliance

**Contexto e Objetivo**
Atue como um Engenheiro de Software Sênior. Precisamos implementar novas funcionalidades de compliance e auditoria no sistema Secullum, atualizando nosso backend e nosso frontend. A principal mudança de regra de negócio é que pararemos de retornar uma lista de auditorias a cada atualização; passaremos a trabalhar com uma máquina de estados onde os dados se atualizam ou se resolvem sozinhos, mantendo os logs.

**Backend**
- [ ] Identidade estável por ocorrência (colaborador + data + tipo) com estados: `aberta`, `atualizada`, `resolvida_automatica`, `resolvida_manual`
- [ ] Lógica de comparação a cada sync: ocorrência repetida não duplica; ocorrência que some vira `resolvida_automatica`; ocorrência com valor novo atualiza e reavalia
- [ ] Esse mecanismo cobre também a reclassificação de trocas na escala mensal variável (gestor troca colaboradores de dia sem atualizar o sistema) — nasce como categoria operacional e se resolve sozinha se a escala for corrigida
- [ ] Modelo de dados de filiais: `filial 1—N aparelho`, `filial 1—1 gestor (nome, telefone)`, aparelho pertence a uma única filial
- [ ] Endpoints de CRUD de filial (vínculo aparelho/N° folha, dados do gestor)
- [ ] Endpoint de "ignorar ocorrência" (marca `resolvida_manual`)
- [ ] Endpoint retornando horário fixo (Secullum) e filial (via N° folha/aparelho) prontos para autopreenchimento
- [ ] Endpoint de registro de advertência: criar/atualizar status (`draft` / `enviada` / `assinada`) — sem hash por enquanto
- [ ] Rodar bateria de sync múltiplos no mesmo dia e validar no banco que não duplica
- [ ] Endpoint para fazer auditoria em dias de escolha, também retornando horário fixo e filial

**Frontend**
- [ ] Painel de Indicadores — **UI/UX 1:** melhorar visualização do resumo de inconsistências por tipo (hoje é lista simples; organizar por categoria/contagem)
- [ ] **UI/UX 2:** novas categorias de aviso com cores próprias além de Crítico/Alerta/Operacional — ex.: *Alteração de escala*, *Ocorrência não confirmada* ⏳ *depende dos novos estados do backend (`aberta`/`atualizada`/etc.) para saber o que exibir em cada cor*
- [ ] Início da tela de filiais (CRUD básico: listar, cadastrar, vincular aparelho/N° folha, gestor) ⏳ *depende dos endpoints de filial*
- [ ] Botão de ignorar na tela de colaborador, consumindo o endpoint novo
- [ ] Autopreenchimento de horário fixo e filial na página de colaborador
- [ ] Form de advertência (draft/send/sign) — UI de controle simples
- [ ] **UI/UX 3:** indicador de advertências enviadas x confirmadas (contagem/status na tela de colaborador ou indicadores)
- [ ] **UI/UX 4:** visualização por filial no painel (filtro/seleção de filial nos indicadores e na listagem) ⏳ *depende da tela de filiais estar minimamente funcional*
