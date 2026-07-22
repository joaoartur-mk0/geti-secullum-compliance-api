# Painel de Testes (frontend)

Frontend estático (HTML + CSS + JS puro, **sem build/npm**) para demonstrar os cadastros
e operações do backend: tenants, responsáveis (staff), configurações, relatórios e disparo
de auditoria.

## Como rodar

Sirva a pasta com qualquer servidor estático e abra no navegador. Exemplos:

```bash
# opção 1: Python
cd frontend
python3 -m http.server 5500
# abra http://localhost:5500

# opção 2: VS Code -> extensão "Live Server" -> "Go Live"
```

No topo da página, ajuste a **API base URL** (padrão `http://localhost:8080`) e clique em
**Testar /health**.

## Pré-requisitos no backend

1. **Backend no ar** (`docker compose up -d` em `infrastructure/`).
2. **CORS liberado** para desenvolvimento. Como o frontend roda em outra porta
   (ex.: 5500) e chama a API na 8080, o navegador exige cabeçalhos CORS. Sem eles,
   as requisições `POST/PUT` falham no preflight. Há um middleware de CORS de
   desenvolvimento no backend para isso (`interface/http/middleware`).

> Alternativa sem CORS: servir estes arquivos estáticos pelo próprio backend (mesma
> origem). Hoje o demo assume frontend e backend em origens separadas.

## O que dá para testar

- **Tenants:** cadastrar, listar (com/sem inativos), abrir, editar, desativar.
- **Responsáveis:** adicionar, listar, editar, excluir.
- **Configurações:** carregar, alterar flags/severidades/horários e salvar.
- **Relatórios:** listar os relatórios de auditoria do tenant.
- **Auditoria:** disparar (`POST /audit/trigger`).

Todas as requisições e respostas aparecem no **Log** no rodapé — útil para a apresentação.
