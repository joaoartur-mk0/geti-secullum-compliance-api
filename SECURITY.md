# Política de Segurança

## Versões suportadas

Enquanto o projeto estiver em desenvolvimento ativo (série `0.x`), apenas a versão mais
recente publicada em [`VERSION`](./VERSION) recebe correções de segurança. Não há suporte
retroativo a versões anteriores nesta fase.

## Reportando uma vulnerabilidade

**Por favor, não abra uma issue pública para relatar uma vulnerabilidade de segurança.**
Este sistema audita dados de ponto eletrônico e conformidade trabalhista de empresas
clientes — uma vulnerabilidade divulgada publicamente antes de ser corrigida expõe dados
sensíveis de terceiros.

Em vez disso, entre em contato diretamente com o responsável pelo repositório:

- **E-mail:** joaoartur655@gmail.com

### O que incluir no relato

Para que a vulnerabilidade possa ser avaliada e corrigida rapidamente, inclua:

1. **Descrição do problema** — o que está errado e por quê é uma vulnerabilidade (não só
   "X está quebrado").
2. **Passos para reproduzir** — sequência exata de ações, requisições ou entradas que
   disparam o problema. Um exemplo concreto (payload, comando `curl`, captura de tela)
   vale mais que uma descrição abstrata.
3. **Impacto** — o que um atacante consegue fazer explorando isso: ler dado de outro
   tenant, escalar privilégio, executar código, negar serviço, etc.
4. **Versão/commit afetado** — qual `VERSION` ou hash de commit foi usado para reproduzir.
5. **Sugestão de correção**, se tiver uma — opcional, mas ajuda.

### O que esperar

- Confirmação de recebimento em até alguns dias úteis.
- Comunicação sobre o andamento da investigação e, quando aplicável, o prazo estimado
  para uma correção.
- Crédito pelo relato (se desejado) quando a correção for publicada, respeitando o
  princípio de *responsible disclosure*: o relato permanece privado até a correção estar
  disponível.

## Escopo

Este sistema integra com APIs de terceiros (Secullum, Evolution API/WhatsApp). Problemas
de segurança nessas plataformas em si devem ser reportados diretamente a elas — este
canal cobre vulnerabilidades no código deste repositório (backend, frontend, infra de
deploy) e na forma como ele consome essas integrações.
