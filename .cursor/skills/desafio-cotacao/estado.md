# Estado do desafio

etapa_atual: 3
etapa_nome: Portão de decisões
decisoes_confirmadas: nao

Atualizar este arquivo ao fechar cada etapa. Não citar este caminho no SAD: não faz parte da arquitetura entregue.

## Feito

- [x] 0 Ferramentas (Go 1.25.14, Compose v2.31.0, motor Docker 27.4.0). `make help` exige `awk`, que está em `C:\Program Files\Git\usr\bin` e fora do PATH; `make test` e `make up` não usam `awk`. Go em `%LOCALAPPDATA%\Programs\go1.25.14\bin`, no PATH do usuário.
- [x] 1 Leitura dos quatro buracos. Contrato confirmado pelo usuário. Resumo na seção Leitura.

## Leitura

Contrato de hoje: `POST /quotes` exige `X-Tenant-Id` (`internal/quotation/handler.go`). Corretora no cabeçalho; CPF e placa no corpo (`internal/quotation/request.go`). Sucesso 200 devolve `tenant_id`, `quotes` ordenadas por `premium_cents` e `elapsed_ms`. Qualquer falha de parceira vira 502 `partner insurer unavailable` e descarta o que já chegou. Prêmio só sai da parceira. `valid_for_seconds` do mock é 300 s; teto comercial é 24 h. Auditoria SUSEP: 5 anos, por cotação apresentada. LGPD: Prumo operadora, corretora controladora.

Quatro buracos:

1. Sem timeout — `internal/partner/client.go`, `NewClient`: `http.Client` só com `Transport`, `Timeout` zero. `partner-degrading` nunca devolve erro; afunda até 6000 ms.
2. Agregação em série — `internal/quotation/service.go`, laço `for` chama uma parceira e espera a próxima. Latência da cotação é a soma. Paralelizar é bônus, fora desta etapa.
3. Tudo ou nada — o mesmo laço: `err != nil` devolve `Response{}`. `handler.go` responde 502 e joga fora as cotações já obtidas.
4. Redis sobe (`docker-compose.yml`, serviço `redis`) e `internal/platform/config.go` não tem endereço de Redis. `internal/platform/telemetry.go` emite HTTP de entrada, HTTP de saída e runtime. Sem métrica de breaker, cache ou latência de domínio.

Parceiras no compose, semente `20260729`: `partner-slow` :9001 (1500 ms ± 200 ms, nunca falha); `partner-flaky` :9002 (150 ms ± 50 ms, 40% de 503); `partner-degrading` :9003 (120 ms ± 40 ms; acima de 5 simultâneas soma 300 ms, teto 6000 ms, nunca falha). Rajada presa em `cmd/partner-mock/feasibility_test.go`: 9 falhas nas sequências 49 a 57. Contêineres que o C4 repete: `quotation-api`, `partner-slow`, `partner-flaky`, `partner-degrading`, `redis`, `otel-collector`, `jaeger`, `prometheus`.

Conta do cenário, para a seção 8: R$ 0,04 por consulta, 3 consultas por cotação, 120.000 cotações por dia útil, 22 dias úteis, R$ 0,25 por cotação entregue.
- [x] 2 Evidência do antes nesta máquina. Relatório em `docs/evidencias/antes-reproduce.txt`. Traces em `docs/evidencias/antes-trace-longo.json` e `docs/evidencias/antes-trace-erro.json`. Nota em `docs/evidencias/antes.md`.
- [ ] 3 Decisões confirmadas pelo usuário
- [ ] 4 SAD seções 1 a 4
- [ ] 5 Timeout e circuit breaker
- [ ] 6 Cache
- [ ] 7 Fallback
- [ ] 8 Instrumentação
- [ ] 9 Evidências do depois
- [ ] 10 SAD seções 5 a 8 e README do processo
- [ ] 11 Revisão de aceite

## Medição do antes

Medido por `make reproduce` depois de `make down`, nesta máquina (Windows 11 Education, Intel Core i5-12450H, Docker Engine 27.4.0, Compose v2.31.0). O relatório imprime UTC.

- comando: `make down` e em seguida `make reproduce`
- quando: 2026-11-17 22:12:31 UTC a 22:13:14 UTC (carga 22:12:50 UTC a 22:13:14 UTC)
- baseline sucesso / p95: 5 de 10 (50%), p95 2.04 s, 0.5 req/s; 5 falhas HTTP 502 da partner-flaky
- carga sucesso / p95 / vazão: 120 de 200 (60%), p95 8.01 s, máximo 8.05 s, 8.0 req/s em 24.85 s (3.9× o p95 do baseline); 80 falhas HTTP 502 da partner-flaky
- artefato: `docs/evidencias/antes-reproduce.txt`, `docs/evidencias/antes.md`, `docs/evidencias/antes-trace-longo.json` (trace `251f53502c68ca6036455b872b7e795e`), `docs/evidencias/antes-trace-erro.json` (trace `4feb557ea0aeb06e78eabc953eebfc59`). Consulta PromQL opcional em `docs/evidencias/antes-prometheus.txt`, avaliada em 22:13:14 UTC.

## Decisões

Preencher na etapa 3. Números daqui são os do SAD e os do código.

### Pressupostos

1. valor / origem / se for falso:
2. valor / origem / se for falso:
3. valor / origem / se for falso (recotação; alimenta o hit rate):

### Circuit breaker

- biblioteca / alternativa descartada:
- escopo:
- abrir com / janela / tempo aberto / provas no meio aberto:
- timeout ms / estouro conta como falha:
- onde o teste determinístico abre:

### Cache

- chave literal:
- por parceira ou agregada:
- TTL:
- com breaker aberto:
- invalidação e descarte:

### Fallback

- opção:
- como a resposta mostra degradação:
- status e corpo quando não há o que entregar:

### Hospedagem

- escolha / alternativa descartada / motivo regulatório e financeiro:

### Métricas (etapa 8)

- breaker:
- hit / miss:
- latência por parceira:
- marcas de trace:
