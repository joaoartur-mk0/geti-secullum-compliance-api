# Documento de Especificação Técnica e Funcional
## Sistema de Auditoria Diária de Jornada – API SecullomWEB

### 1. Visão Geral do Projeto
Este documento define as diretrizes iniciais para o desenvolvimento de um microsserviço de automação projetado para auditar registros de ponto eletrônico diariamente. O sistema consumirá dados da API do SecullomWEB, aplicará regras de negócio específicas para identificar inconsistências e infrações de jornada em horários predefinidos ao longo do dia, fornecendo dados estruturados para tomada de decisão ou alertas.

### 2. Objetivos Críticos
* Criar um serviço escalável, configurável e customizável para ser comercializado.
* Automatizar a extração de marcações de ponto do dia corrente através da API SecullomWEB.
* Analisar instantaneamente os dados extraídos com base nas regras de negócioscríticas de compliance trabalhista.
* Disponibilizar os resultados das inconsistências de forma clara e estruturada para integrações ou relatórios.
* Registrar dados de inconsistências para ter todos as informações necessárias para auditorias. 
* Servir como base de contexto precisa para ferramentas de IA auxiliarem no desenvolvimento do código.

## 3. Stack Tecnológica
* **Linguagem Principal:** Golang (Go 1.22+) – Justificativa: Alta concorrência nativa (*goroutines*), baixo consumo de memória e velocidade de execução.
* **Framework HTTP:** Gin Gonic – Justificativa: Roteamento de alta performance e baixo overhead.
* **Banco de Dados:** PostgreSQL 16+ – Justificativa: Suporte avançado a operações JSONB, indexação robusta e integridade ACID.
* **Mensageria / Broker:** RabbitMQ 3.12+ – Justificativa: Suporte a filas de mensagens persistentes, roteamento avançado (Direct/Topic Exchanges) e políticas de DLQ (*Dead Letter Queue*).
* **Frontend de Gestão:** Vite + React + TypeScript (Interface Administrativa do Tenant).

---

## 4. Regras de Negócio e Algoritmos de Validação

O motor de auditoria (`auditor_service.go`) deve processar as marcações coletadas e aplicar as seguintes regras de validação analítica:

### 5.1. Interjornada Curta (Art. 66 da CLT)
* **Critério:** Período de descanso entre duas jornadas de trabalho inferior a **11 horas consecutivas**.
* **Lógica de Validação:**
    1. Buscar a última batida de saída registrada do colaborador no dia anterior ($D_{-1}$).
    2. Buscar a primeira batida de entrada registrada no dia corrente ($D_0$).
    3. Calcular Delta: $\Delta t = t(\text{Entrada}_{D_0}) - t(\text{Saída}_{D_{-1}})$.
    4. Se $\Delta t < 11\text{ horas}$, gerar ocorrência de nível **Crítico**.
* **Regra de Exceção:** Se o funcionário não possuir registro de entrada no dia corrente até o horário limite do seu turno, a validação fica em estado suspenso.

### 5.2. Intervalo Intrajornada Reduzido (Almoço) (Art. 71 da CLT)
* **Critério:** Período destinado a repouso e alimentação inferior a **1 hora (60 minutos)** para jornadas superiores a 6 horas, com tolerância máxima de 10 minutos diários se houver convenção coletiva.
* **Lógica de Validação:**
    1. Identificar o par de batidas correspondente ao intervalo (Saída Almoço e Retorno Almoço).
    2. Calcular Delta: $\Delta t = t(\text{Retorno Almoço}) - t(\text{Saída Almoço})$.
    3. Se $\Delta t < 60\text{ minutos}$, registrar ocorrência de nível **Alerta** ou **Crítico** (conforme parametrização do Tenant).

### 5.3. Batidas Incompletas / Anômalas
* **Critério:** Ausência de marcações obrigatórias ou inconsistência lógica temporal no momento da checagem.
* **Lógica de Validação:**
    1. Contar o número total de batidas efetuadas pelo colaborador até o timestamp da checagem.
    2. Cruzar com a matriz de horários esperados do turno (armazenada localmente).
    3. Se o horário atual for superior ao horário previsto para a saída do almoço em +30 minutos e o funcionário possuir apenas 1 batida (entrada), marcar como **Inconsistência de Batida Esquecida**.
    4. Ao final do dia, qualquer contagem de batidas **ímpar** dispara um alerta crítico estruturado.

### 5.4. Hora Extra Excedente Limite (Art. 59 da CLT)
* **Critério:** Extrapolação do limite legal de horas extraordinárias diárias.
* **Lógica de Validação:**
    1. Calcular o tempo total trabalhado líquido (soma dos blocos de jornada realizada).
    2. Subtrair a carga horária contratual padrão prevista para o dia.
    3. Se o saldo positivo ($\text{Horas Extras}$) for:
        * $> 1\text{ hora}$ e $\le 2\text{ horas}$: Disparar **Alerta Preventivo**.
        * $> 2\text{ horas}$: Disparar **Infração Crítica**.

---

### 5. Sincronização de Dados e Operações da Automação
Para mitigar a dependência contínua de requisições síncronas à API SecullumWEB, o ciclo de vida dos dados será gerenciado em duas etapas:

#### 5.1. Onboarding e Sincronização de Metadados (Provisionamento de Tenant)
Ao cadastrar ou ativar um novo Tenant no sistema:
1. Uma mensagem é enviada para a fila `tenant.provisioning`.
2. O sistema consome os endpoints da API SecullumWEB para extrair:
    * Lista de Colaboradores (`ID`, `Nome`, `PIS/CPF`).
    * Matriz de Turnos e Horários Contratuais associados.
3. Esses dados são persistidos no PostgreSQL local.
4. **Estratégia de Sincronismo Diário:** Um Job agendado rodará todas as madrugadas (02:00 AM) para atualizar os metadados dos colaboradores, garantindo que alterações cadastrais feitas no SecullumWEB sejam refletidas localmente.

#### 5.2. Pipeline de Execução de Auditoria
O processamento de auditoria não deve travar a API do microsserviço. O fluxo correto é:
1. **Gatilho Temporal:** Um mecanismo de agendamento interno (ou cron externo) dispara requisições para os horários estratégicos configurados na interface (ex: 12:00, 14:00, 18:30).
2. **Enfileiramento:** O endpoint publica um evento `AuditTriggeredEvent` contendo o `tenant_id` no RabbitMQ.
3. **Ingestão (Worker 1):** O worker consome a mensagem, faz o request seguro à API SecullumWEB trazendo apenas os bilhetes/marcações do dia atual para aquele Tenant e salva o estado bruto no banco de dados.
4. **Análise (Worker 2):** O worker de auditoria lê o estado bruto armazenado, aplica as regras contidas na Seção 4, gera o relatório de inconsistências e envia para a tabela de logs de auditoria e fila de alertas.

#### 5.3. Estratégia de Notificações Híbrida (Intra-dia e Noturna)
O sistema adota uma abordagem mista para garantir que infrações sejam evitadas a tempo (ação preventiva), sem comprometer a geração do histórico oficial consolidado no banco de dados.
* **Varredura Intra-dia (Alertas Preventivos):** * Um Cron Job interno do Golang desperta nos horários estratégicos (ex: 12:00, 14:00, 18:30) e enfileira uma ordem de auditoria parcial.
    * O motor avalia inconsistências em andamento (ex: atraso na volta do almoço).
    * Caso detecte anomalias, dispara mensagens instantâneas para a fila `notifications.whatsapp`.
    * *Nota:* O relatório oficial não é gravado no banco de dados nesta etapa, pois o expediente ainda não foi encerrado.
* **Consolidação Noturna (O Fechamento):** * O Cron Job dispara de madrugada (ex: 01:00 AM), avaliando as marcações definitivas do dia anterior ($D_{-1}$).
    * O sistema calcula o saldo final de horas, consolida todas as infrações, gera o objeto JSON estruturado e insere uma linha definitiva na tabela `reports`.
    * Após a gravação, um resumo consolidado do fechamento diário é enviado via WhatsApp para o gestor.

#### 5.4. Integração Assíncrona com WhatsApp (Evolution API)
Para evitar que a latência de APIs externas de mensageria cause gargalos no motor de auditoria, o envio de notificações utiliza o RabbitMQ em conjunto com a **Evolution API**.
1. **Postagem na Fila:** O motor de auditoria identifica a infração e envia o alerta em formato texto para a fila `notifications.whatsapp` no RabbitMQ.
2. **Consumo pelo Worker:** Um worker exclusivo, escrito em Go, consome essas mensagens de forma controlada.
3. **Requisição HTTP:** O worker processa a mensagem e faz uma requisição `POST` para o endpoint `/message/sendText` da Evolution API.
4. **Entrega Humanizada:** A Evolution API emula uma conexão do WhatsApp Web, aplica o estado de "escrevendo" (`presence: composing`) e entrega a notificação no dispositivo do staff.

**Contrato de Integração (Payload de Exemplo):**
```json
{
  "number": "5531999999999",
  "options": {
    "delay": 1500,
    "presence": "composing"
  },
  "textMessage": {
    "text": "⚠️ *Alerta Preventivo de Jornada*\n\n🏢 *Tenant:* Empresa Exemplo S.A.\n👤 *Colaborador:* João Silva\n⏱️ *Ocorrência:* Hora Extra excedeu 1 hora.\n\nPor favor, verifique o painel para mais detalhes."
  }
}

## 6. Sincronização de Dados e Operações da Automação

Para mitigar a dependência contínua de requisições síncronas à API SecullumWEB, o ciclo de vida dos dados será gerenciado em duas etapas:

### 6.1. Onboarding e Sincronização de Metadados (Provisionamento de Tenant)
Ao cadastrar ou ativar um novo Tenant no sistema:
1. Uma mensagem é enviada para a fila `tenant.provisioning`.
2. O sistema consome os endpoints da API SecullumWEB para extrair:
    * Lista de Colaboradores (`ID`, `Nome`, `PIS/CPF`).
    * Matriz de Turnos e Horários Contratuais associados.
3. Esses dados são persistidos no PostgreSQL local.
4. **Estratégia de Sincronismo Diário:** Um Job agendado rodará todas as madrugadas (02:00 AM) para atualizar os metadados dos colaboradores, garantindo que alterações cadastrais feitas no SecullumWEB sejam refletidas localmente.

### 6.2. Pipeline de Execução de Auditoria
O processamento de auditoria não deve travar a API do microsserviço. O fluxo correto é:

1. **Gatilho Temporal:** Um mecanismo de agendamento interno (ou cron externo) dispara requisições para os horários estratégicos configurados na interface (ex: 12:00, 14:00, 18:30).
2. **Enfileiramento:** O endpoint publica um evento `AuditTriggeredEvent` contendo o `tenant_id` no RabbitMQ.
3. **Ingestão (Worker 1):** O worker consome a mensagem, faz o request seguro à API SecullumWEB trazendo apenas os bilhetes/marcações do dia atual para aquele Tenant e salva o estado bruto no banco de dados.
4. **Análise (Worker 2):** O worker de auditoria lê o estado bruto armazenado, aplica as regras contidas na Seção 5, gera o relatório de inconsistências e envia para a tabela de logs de auditoria e fila de alertas.

---

## 7. Estrutura Arquitetural do Código (Golang)

O projeto adotará uma versão adaptada da *Clean Architecture*, garantindo isolamento total das regras de negócio contra fatores externos (banco, frameworks web, brokers).

cmd/
└─ api/
└─ main.go         # Ponto de entrada da aplicação (inicialização do Gin, DB e RabbitMQ)
internal/
├─ domain/             # Entidades de negócio puras (models) e interfaces de repositórios
│   ├─ tenant.go
│   ├─ employee.go
│   └─ audit.go
├─ infrastructure/     # Implementações tecnológicas de infraestrutura
│   ├─ database/       # Conexão PostgreSQL e queries (GORM / sqlc)
│   ├─ messaging/      # Produtores e Consumidores RabbitMQ
│   └─ secullum/       # Cliente HTTP estruturado para a API SecullumWEB
├─ usecase/            # Orquestração das regras de negócio (Casos de Uso)
│   ├─ auditor.go      # Implementação matemática e lógica das validações trabalhistas
│   └─ synchronizer.go # Lógica de sincronização de metadados do tenant
└─ interface/http/     # Camada de entrega da API Rest (Handlers e Routers Gin)
├─ middleware/     # Validação de JWT e Injeção do Tenant ID
├─ handlers/
└─ router.go

### Endpoints Mínimos Homologados

#### 1. `GET /health`
Verificação de integridade da infraestrutura (Conexão DB e Broker).

#### 2. `POST /api/v1/tenants`
Cadastra e inicia o provisionamento assíncrono de um novo cliente.

> **Modelo de credenciais (chave global).** As credenciais de acesso à API SecullumWEB
> **não** são armazenadas por tenant. Existe **uma única credencial global** (token/usuário),
> configurada por variável de ambiente no microsserviço, usada para todas as requisições.
> O que identifica o cliente na Secullum é o `secullum_database_id` (banco selecionado),
> enviado por requisição. Por isso o cadastro de tenant guarda apenas dados de negócio
> (nome, id do banco Secullum e o responsável/staff que receberá os alertas).

* **Payload:**
```json
{
  "name": "Empresa Exemplo S.A.",
  "secullum_database_id": 123,
  "staff_name": "Fulano de Tal",
  "staff_contact": "5531999999999"
}
```
* **Comportamento:** cria o tenant, o responsável (staff) e as configurações de regras
  (`Tenants_Settings`) já com todas as flags em `false` (o cliente as habilita depois pela
  interface administrativa). As severidades de cada regra também são configuráveis por tenant.

## Banco de Dados

Table tenants {
  id integer [primary key]
  secullum_database_id integer unique [not null]
  name varchar [not null]
}

Table collaborators {
  id integer [primary key]
  tenant_id integer [ref: > tenants.id, not null]
  secullum_id integer [not null]
  cpf varchar unique [not null]
  celular varchar unique
}

Table staffs {
  id integer [primary key]
  tenant_id integer [ref: > tenants.id, not null]
  name varchar [not null]
  celular varchar unique [not null]
}

Table reports {
  id integer [primary key]
  tenant_id integer [ref: > tenants.id, not null]
  report jsonb [not null]
  data_generated timestamp [not null]
  date date [not null]
}

Table collaborators_schedulle {
  id integer [primary key]
  collaborator_id ir unique [ref: > collaborators.id, not null]
  entrada_1 time
  saida_1 time
  entrada_2 time 
  saida_2 time
}

Table Tenants_Settings {
  id integer [primary key]
  tenant_id integer [ref: > tenants.id, not null]
  almoco boolean [not null, default: false]
  interjornada boolean [not null, default: false]
  hextras boolean [not null, default: false]
  esquecimento boolean [not null, default: false]
  almoco_severity varchar [not null, default: 'CRITICO']        // ALERTA | CRITICO (configurável por tenant)
  interjornada_severity varchar [not null, default: 'CRITICO']  // ALERTA | CRITICO
  esquecimento_severity varchar [not null, default: 'CRITICO']  // ALERTA | CRITICO
  horarios jsonb [not null]  // ex: ["12:00","14:00","18:30"]
}

// Nota: a severidade de "hora extra" não é configurável — segue os limiares legais
// (Art. 59 CLT): > 1h e <= 2h => ALERTA; > 2h => CRITICO.
