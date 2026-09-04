# Contexto do domínio

Glossário do projeto. Só vocabulário — escopo, decisões de ciclo e contratos estão em
`docs/`.

Regra geral: a Secullum é a fonte da verdade das marcações. Vários termos existem **dos
dois lados** com significados diferentes. Quando isso acontece, a tabela diz qual é qual —
é a origem da maior parte da confusão neste domínio.

---

## Ocorrência e ciclo de vida

| Termo | Definição |
|---|---|
| **Ocorrência** | A inconsistência com identidade estável e ciclo de vida próprio. É o que recebe desfecho e o que é contado em qualquer agregação. |
| **Inconsistência** | O apontamento bruto dentro de um relatório de varredura. Não tem identidade entre varreduras. **Não confundir com Ocorrência.** |
| **Tipo** | A regra violada (batida esquecida, interjornada curta, …). São sete, fixos. |
| **Severidade** | `CRITICO`, `ALERTA` ou `OPERACIONAL`. **Três valores, nunca quatro.** |
| **Desfecho** | O estado terminal de uma ocorrência. |
| **Tratativa** | Ação humana registrada que dá desfecho a uma ocorrência, com justificativa e, quando o tipo exige, anexo. |

### Estados

| Estado (sistema) | Rótulo na tela | Significa |
|---|---|---|
| `aberta` | Em aberto | Detectada, sem desfecho |
| `atualizada` | A reconferir | O valor mudou desde a última varredura |
| `resolvida_automatica` | Corrigida na origem | O dado foi ajustado no Secullum |
| `resolvida_manual` | Ignorada | Alguém decidiu que o apontamento não procedia |
| `tratada` | Tratado | Houve ação humana sobre um problema real |

Três distinções que não podem ser borradas:

- **Ignorada ≠ Tratada.** Ignorar diz que o apontamento não procedia. Tratar diz que havia
  um problema e alguém agiu.
- **Corrigida na origem ≠ Tratada.** A primeira é o dado consertado no Secullum, a segunda é
  trabalho humano registrado aqui. Fundir as duas apaga a distinção que motivou o produto a
  ter estado próprio.
- **`OPERACIONAL` não é infração.** É sinal de investigação — provável troca de escala não
  comunicada. Não pontua em ranking, mas nunca é omitido: aparece em contagem própria.

---

## Ciclo temporal

| Termo | Definição |
|---|---|
| **Fechamento** | A varredura **diária** de D-1. **Nunca** usar este nome para o ciclo mensal. |
| **Revisão mensal** | O ciclo **mensal**: conferir o que falta e encerrar a competência. |
| **Competência** | Mês calendário, `YYYY-MM`. |
| **Período** | Intervalo livre de datas. Não confundir com Competência. |

"Fechamento" para o mensal é o erro de nomenclatura mais fácil de cometer neste projeto, e
colide com uma feature que já existe há tempos.

---

## Unidade organizacional — Secullum vs. nosso lado

O ponto mais escorregadio do domínio. Quatro termos, dois donos.

| Termo | Dono | Definição |
|---|---|---|
| **Empresa** | Secullum | A pessoa jurídica (CNPJ) à qual o colaborador está vinculado. Um mesmo local físico pode ter colaboradores de várias empresas. |
| **Estrutura** | Secullum | Uma **árvore configurável pelo cliente**, sem semântica garantida. Um cliente a usa para lojas; outro, para organograma. **Não é sinônimo de filial.** |
| **Departamento** | Secullum | O setor do colaborador, como cadastrado. O nome pode embutir a unidade (`AÇOUGUE SC`) — o cadastro é do cliente, e o painel reflete a fonte sem corrigi-la. |
| **Função** | Secullum | O cargo do colaborador. |
| **Filial** | Nosso | Unidade física do cadastro local. Existe para saber **quem cobrar**: carrega o gestor e o telefone que recebe o alerta de WhatsApp — informação que a Secullum não tem. |

Três armadilhas registradas:

1. **`Filial := Estrutura` é falso.** A semântica de Estrutura varia por tenant, e o produto
   é multi-tenant. A equivalência quebraria pintando dado errado, sem dar erro.
2. **Comentários no código afirmam que a Secullum não modela unidade organizacional.** Isso
   é falso — ela modela, via `Estrutura`. Ver `docs/documento-funcional-compliance.md` §8.
3. **"Sem filial" é uma linha de primeira classe.** Colaborador sem unidade resolvida é
   comum e esperado. Nenhuma agregação pode descartá-lo em silêncio: se descartar, a soma
   das partes deixa de bater com o total e o painel mente.

---

## Acesso

| Termo | Definição |
|---|---|
| **Tenant** | O cliente. Todo dado é escopado a um tenant. |
| **Papel** | Nível de acesso **dentro de um tenant**: Diretoria ⊂ Gestor ⊂ RH. Vive no vínculo usuário↔tenant, não no usuário — a mesma pessoa pode ter papéis diferentes em clientes diferentes. |
| **Super Admin** | Global, no usuário. Passa por cima de qualquer checagem. |

Papel limita **ações**, não **linhas**: nenhum papel esconde dados de filial de ninguém.
Filial é recorte de leitura, não fronteira de segurança.
