# Evidências do depois

Sete linhas. A carga padrão é `make down` e em seguida `make reproduce` (10 + 200, 50 em voo), nesta máquina. Gráfico e trace foram capturados com o ambiente de pé.

1. Antes e depois, texto completo. Comando: `make down` e `make reproduce`. Artefatos: `antes-reproduce.txt` (2026-11-17 22:12:31–22:13:14 UTC, carga 120/200, p95 8,01 s) e `depois-reproduce.txt` (2026-11-23 02:14:14–02:14:25 UTC, carga 200/200, p95 243 ms, 579,5 req/s).
2. Suíte. Comando: `make test`. Artefato: `make-test.txt`. Legenda: exit 0 em 2026-11-23 02:12:28–02:13:04 UTC, testes antigos e novos verdes.
3. Breaker aberto. Artefato: `depois-trace-breaker.json`, trace `99e2e3a474b06db7472daf239464ce31` (2026-11-23 02:19:03 UTC). Legenda: evento `circuit.short_circuit` da partner-flaky com `breaker.state=open`; os spans `HTTP POST` são só de partner-slow e partner-degrading.
4. Cache. Artefato: `depois-trace-cache.json`, trace `0ccf339739aef01ecc02b3e8115559ac` (2026-11-23 02:14:24.965 UTC, 325 µs, HTTP 200). Legenda: três eventos `cache.hit` e nenhum span de saída para parceira.
5. Estado do breaker. Consulta: `partner_breaker_state`. Janela: 2026-11-23 02:13:30–02:22:30 UTC, passo 5 s. Artefato: `depois-breaker.txt`. Legenda: partner-flaky fica em 0 até 02:19:10 UTC e sobe para 2; slow e degrading permanecem em 0.
6. Hit rate. Consulta: `sum(quote_cache_hit_total) / (sum(quote_cache_hit_total) + sum(quote_cache_miss_total))`. Janela: 2026-11-23 02:14:00–02:16:00 UTC, passo 5 s. Artefato: `depois-hitrate.txt`. Legenda: a razão sai de 0 às 02:14:30 UTC para 0,910 às 02:14:40 UTC e permanece nesse patamar.
7. p95 de `POST /quotes`. Consulta: `histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[1m])))`. Escala comum: 0–10 s. Artefato: `depois-p95.txt`; o antes está em `antes-prometheus.txt`. Legenda: antes 9,387 s na avaliação de 2026-11-17T22:13:14Z; depois entre 0,331 s e 0,343 s de 02:14:40 a 02:15:30 UTC.

A abertura do breaker não saiu da carga padrão. A prova extra, depois do `make reproduce` e antes de qualquer `make down`, foram 40 `POST http://localhost:8080/quotes` sequenciais, cabeçalho `X-Tenant-Id: corretora-b`, um risco novo por chamada, 2026-11-23 02:18:46–02:19:58 UTC. O SAD tem de registrar que essa prova rodou.
