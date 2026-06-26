# Information about the Secullum API

## Duração de Token de Acesso

Para consumir a API, você precisa de um token de acesso válido. Esse token é gerado através de uma chamada para a API de autenticação. O token tem uma duração de 1 hora.

### Payload da Chamada de Autenticação

A chamada de autenticação utiliza um payload no formato JSON com as credenciais do usuário:

```json
{
    "username": "elmer",
    "password": "123456"
}
```

### Resposta da Chamada de Autenticação

A resposta da chamada de autenticação é um token de acesso válido:

```json
{
    "access_token": "token"
}
```

### Exemplo de Chamada de Autenticação

Aqui está um exemplo de como fazer uma chamada de autenticação usando o `curl`:

```bash
curl -X POST -H "Content-Type: application/json" -d '{"username": "elmer", "password": "123456"}' http://localhost:5000/auth
```

## Rate Limit

O rate limiting é aplicado para evitar sobrecarga no servidor, e o rate limit da Secullum API é de 100 requisições por minuto hora.
