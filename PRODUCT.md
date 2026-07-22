# Product

## Register

product

## Platform

web

## Users

Gestores de RH e administradores de empresas clientes da Geti Soluções que já usam o Secullum Ponto Web. Usam o painel durante o expediente, no escritório (desktop) e em movimento (celular), para conferir alertas de batidas de ponto irregulares e configurar quem recebe os avisos via WhatsApp. Perfil não técnico: o painel precisa ser autoexplicativo. Audiência secundária imediata: a própria equipe da Geti, que verá o painel como prévia comercial do produto.

## Product Purpose

Painel administrativo do Secullum Compliance — o "adicional" que a Geti vende sobre o Secullum Ponto Web. O backend (Go) audita as batidas de ponto diariamente contra regras da CLT (interjornada, intervalo de almoço, horas extras, batidas esquecidas) e dispara alertas via WhatsApp (Evolution API). O painel permite: autenticar (provisório, single-tenant por ora), cadastrar gestores que recebem alertas, configurar quais regras geram avisos (flags, severidades, horários de varredura), conectar a instância do WhatsApp e consultar os relatórios de auditoria. Sucesso: a Geti aprovar a prévia e a primeira empresa piloto operar o dia a dia sem treinamento.

## Positioning

O radar de compliance trabalhista que o Secullum não tem: irregularidades de ponto detectadas no mesmo dia e avisadas no WhatsApp do gestor antes de virarem passivo.

## Brand Personality

Confiável, direto, operacional. Tom de ferramenta de trabalho séria (compliance trabalhista tem consequência jurídica), sem frieza: mensagens em português claro, sem jargão de RH nem de programador. Três palavras: vigilante, claro, profissional.

## Anti-references

- Dashboard SaaS genérico de template (grid de cards idênticos com ícone + métrica + gradiente).
- Painéis administrativos "de sistema antigo" (tabelas cinzas sem hierarquia, formulários crus estilo intranet 2010) — é o que o mercado de ponto eletrônico já oferece; o diferencial visual importa na venda.
- Excesso de lúdico/emoji: alerta de infração da CLT não é lugar de mascote.

## Design Principles

1. **Severidade é hierarquia** — a diferença entre ALERTA e CRÍTICO deve ser legível de relance, em qualquer tela; cor semântica só para estado, nunca decoração.
2. **Configurar sem manual** — cada regra de aviso explica em uma frase o que dispara e quando; o gestor de RH entende sem ajuda.
3. **Celular é cidadão de primeira classe** — o gestor confere alerta no corredor; fluxos completos em mobile, não versão encolhida.
4. **Prévia com cara de produto** — mesmo provisório, cada tela parece pronta para vender; estados vazios, carregamento e erro tratados.

## Accessibility & Inclusion

Contraste WCAG AA (≥4.5:1 em texto corrente), alvos de toque ≥44px em mobile, `prefers-reduced-motion` respeitado. Severidades nunca comunicadas só por cor (sempre com rótulo/ícone).
