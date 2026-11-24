# Prumo Cota

SAD: [docs/sad.md](docs/sad.md). Evidências: [docs/evidencias/](docs/evidencias/). O enunciado do desafio está em [docs/enunciado.md](docs/enunciado.md).

## Subir e reproduzir

Go 1.25, Docker Engine e Compose v2. `make test` roda a suíte. `make up` sobe o ambiente. A reprodução da versão entregue, do zero, é:

```
make down
make reproduce
```

`make down` remove os volumes e zera Jaeger, Prometheus e Redis. O relatório da carga sai no texto do comando. As medições já feitas estão em `docs/evidencias/antes-reproduce.txt` e `docs/evidencias/depois-reproduce.txt`.

## Implementado

Timeout de 2500 ms e circuit breaker por parceira em `internal/partner/client.go`, com `github.com/sony/gobreaker/v2`. Cache por parceira no Redis, TTL de 900 s, chave com a corretora, em `internal/quotation/cache.go`. Resposta parcial: HTTP 200 com `degraded` e `unavailable_partners`. Nenhuma proposta: HTTP 503 `{"error":"no_quote","unavailable_partners":[...],"retry_after_seconds":30}`.

## Proposta

Arquivo de auditoria SUSEP por cinco anos, em região brasileira, com segunda cópia. Produção em nuvem no Brasil. O PoC permanece no Compose local. Regra de alerta que pagina sozinha: `deploy/prometheus/prometheus.yml` não tem `rule_files`. Recusa da cotação quando a auditoria não grava.

## Métricas e atributos

| Instrumento | Prometheus | Atributos |
|---|---|---|
| `partner.breaker.state` | `partner_breaker_state` | `partner`. 0 fechado, 1 meio-aberto, 2 aberto |
| `partner.breaker.transitions` | `partner_breaker_transitions_total` | `partner`, `from_state`, `to_state` (`closed`, `half-open`, `open`) |
| `quote.cache.hit` | `quote_cache_hit_total` | `partner` |
| `quote.cache.miss` | `quote_cache_miss_total` | `partner` |
| `http.client.request.duration` | `http_client_request_duration_seconds` | `server_address` |

Eventos de trace: `circuit.short_circuit`, com `partner` e `breaker.state`; `cache.hit`, com `partner` e `tenant_id`. CPF, placa e `quote_id` não entram em atributo. `tenant_id` fica só no evento de cache, não na métrica.

## Com mais tempo

Limite de chamadas simultâneas por parceira, breaker compartilhado entre réplicas, invalidação do cache antes dos 900 s quando o prêmio muda ou o titular pede exclusão, e isolamento de capacidade por corretora. A agregação continua em série. Chamar as três parceiras em paralelo baixaria o p95 do caminho frio, que hoje soma as latências. A auditoria de cinco anos seria gravada antes de a resposta sair.

Protegi a cotação contra uma parceira que falha ou afunda. O timeout de 2500 ms e um breaker por parceira cortam a chamada quando o circuito abre. O cache de 900 s, com a corretora na chave, deixa de comprar a consulta repetida na mesma conversa. A resposta parcial entrega o prêmio que já chegou. No pressuposto de 40% de recotação dentro desses 900 s, o gasto com parceiras cai de R$ 316.800 para R$ 190.080 por mês. O custo é um Redis efêmero, um prêmio que pode ficar até 15 minutos desatualizado, e o CPF nesse Redis pelo mesmo prazo. A auditoria de cinco anos não fica mais barata: cotação servida de cache continua cotação apresentada.
