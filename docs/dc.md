# Documento de Especificação Técnica e Funcional
## Sistema de Auditoria Diária de Jornada – API SecullomWEB

### 1. Visão Geral do Projeto
Este documento define as diretrizes iniciais para o desenvolvimento de um microsserviço de automação projetado para auditar registros de ponto eletrônico diariamente. O sistema consumirá dados da API do SecullomWEB, aplicará regras de negócio específicas para identificar inconsistências e infrações de jornada em horários predefinidos ao longo do dia, fornecendo dados estruturados para tomada de decisão ou alertas.

### 2. Objetivos Críticos
* Automatizar a extração de marcações de ponto do dia corrente através da API SecullomWEB.
* Analisar instantaneamente os dados extraídos com base em quatro regras críticas de compliance trabalhista.
* Disponibilizar os resultados das inconsistências de forma clara e estruturada para integrações ou relatórios.
* Servir como base de contexto precisa para ferramentas de IA auxiliarem no desenvolvimento do código.

### 3. Stack Tecnológica
* **Linguagem de Programação:** Golang (Go)
* **Framework Web/API:** Gin Gonic
* **Tipo de Integração:** Cliente HTTP para consumo de API REST (SecullomWEB)

---

### 4. Regras de Negócio e Pontos de Atenção

O sistema deve avaliar o comportamento dos funcionários no dia corrente e apontar desconformidades com base nos seguintes critérios analíticos:

#### 4.1. Interjornada Curta
* **Critério:** Tempo de descanso entre o término da jornada anterior e o início da jornada do dia corrente inferior a **6 horas**.
* **Lógica de validação:** Calcular a diferença de tempo entre a última batida de saída do dia anterior e a primeira batida de entrada do dia de hoje.

#### 4.2. Intervalo de Almoço Reduzido
* **Critério:** Período de intervalo para refeição e descanso menor do que **1 hora (60 minutos)**.
* **Lógica de validação:** Identificar o par de batidas que compreende a saída e o retorno do almoço e calcular o delta de tempo. Se for menor que 60 minutos, registrar ocorrência.

#### 4.3. Batidas Esquecidas (Incompletas)
* **Critério:** Ausência de marcações essenciais ou número ímpar de batidas no momento da checagem.
* **Lógica de validação:** Verificar se o funcionário possui marcações ímpares (ex: apenas entrada, sem saída para o almoço após o horário previsto) ou se faltam marcações esperadas para o turno padrão do colaborador naquele horário do dia.

#### 4.4. Hora Extra Excedente
* **Critério:** Tempo de trabalho extraordinário acumulado no dia que ultrapasse o limite de alerta de **1 hora**, ou o limite crítico de **2 horas**.
* **Lógica de validação:** Calcular o tempo total trabalhado no dia, subtrair a jornada contratual padrão e avaliar se o saldo positivo excede 1 hora (alerta preventivo) ou 2 horas (limite máximo legal).

---

### 5. Arquitetura Inicial do Código (Diretrizes para IA/DEV)

O projeto em Golang deverá seguir uma estrutura limpa e modular. Os seguintes componentes devem ser criados:

* `secullom_client.go`: Responsável pela autenticação na API do SecullomWEB, renovação de tokens e busca dos dados brutos de marcações do dia.
* `auditor_service.go`: Contém as funções lógicas puras que recebem os dados da API e aplicam os cálculos de interjornada, almoço, batidas esquecidas e horas extras.
* `router.go` / `handlers.go`: Utilização do Gin Gonic para expor endpoints REST.

#### Endpoints Mínimos Sugeridos:
* `GET /health` - Verificação de integridade da aplicação.
* `POST /api/v1/audit/trigger` - Dispara manualmente a rotina de busca na API Secullom, processamento das regras e retorno do relatório de inconsistências do momento em formato JSON.

### 6. Fluxo de Execução Esperado
1. O serviço é acionado (via cronjob externo ou chamada de endpoint).
2. O Client em Go autentica-se na API SecullomWEB.
3. O serviço busca o espelho de ponto/marcações em tempo real dos funcionários para o dia atual.
4. Os dados passam pela esteira de validação (`Interjornada` -> `Almoço` -> `Batidas Esquecidas` -> `Horas Extras`).
5. O sistema gera um payload JSON consolidado contendo a lista de funcionários e suas respectivas inconsistências detectadas.