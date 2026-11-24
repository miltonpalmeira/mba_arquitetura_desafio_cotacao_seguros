# Estado do desafio

etapa_atual: 11
etapa_nome: Revisão de aceite
decisoes_confirmadas: sim
aceite: fechado
bonus: nao pedido

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
- [x] 3 Decisões confirmadas pelo usuário em 2026-11-22. Números na seção Decisões.
- [x] 4 SAD seções 1 a 4 em `docs/sad.md`. Quatro diagramas Mermaid. A seção 6 já nomeia as séries e a consulta de latência; alertas, runbook e as seções 5, 7 e 8 ficam na etapa 10.
- [x] 5 Timeout de 2500 ms e circuit breaker por parceira em `internal/partner/client.go`. Estouro conta como falha. Teste abre na chamada 42 da semente 20260729; a chamada 43 não avança a sequência do mock.
- [x] 6 Cache por parceira em `internal/quotation/cache.go` e `internal/quotation/service.go`. Chave literal da seção 4, TTL 900 s (`CACHE_TTL_SECONDS`). O valor guarda `partner` e `quote_id`. Redis no boot: `cmd/quotation-api/main.go` faz ping; o compose passa `REDIS_ADDR=redis:6379` e espera o Redis saudável. Entrada dentro do TTL é servida e a parceira não é chamada, inclusive com o circuito aberto. Entrada vencida não é servida. `degraded`, `unavailable_partners` e o 503 `no_quote` ficam na etapa 7: até lá, falha sem entrada viva continua derrubando a cotação inteira.
- [x] 7 Fallback de resposta parcial. `Response` em `internal/quotation/request.go` traz `degraded` e `unavailable_partners` (lista vazia quando as três chegam). Uma parceira nomeada falha e as outras ficam: HTTP 200, ordem por `premium_cents`. Nenhuma proposta: HTTP 503, corpo `no_quote`, `unavailable_partners` na ordem configurada e `retry_after_seconds` 30, em `internal/quotation/handler.go`. O texto do 502 `partner insurer unavailable` permanece para erro que o serviço não classificou como `no_quote`. Testes com quoter falso, sem rede. SAD: RF-06 e a escolha da seção 4 são fato. Linha para o README do processo: nenhuma proposta → 503 `{"error":"no_quote","unavailable_partners":[...],"retry_after_seconds":30}`.
- [x] 8 Instrumentação. Gauge `partner.breaker.state` e contador `partner.breaker.transitions` em `internal/partner/observe.go`. Contadores `quote.cache.hit` e `quote.cache.miss` em `internal/quotation/observe.go`. Latência por parceira reaproveita `http.client.request.duration` (`http_client_request_duration_seconds`, atributo `server_address`); a consulta está na seção 6 de `docs/sad.md`. Eventos de trace `circuit.short_circuit` e `cache.hit`. Sem CPF, placa ou `quote_id`. Conferido com o ambiente de pé: Prometheus em `partner_breaker_state` (flaky foi 2 e voltou a 0) e `partner_breaker_transitions_total` (`closed→open`, `open→half-open`, `half-open→closed`); Jaeger trace `fe813eaf6e27cc1c98fa23225326ac72` só com `POST /quotes` e três `cache.hit`; trace `6f96a127825bfaee2bf4adb19e731387` com `circuit.short_circuit` da `partner-flaky` e sem span `HTTP POST` para ela.
- [x] 9 Evidências do depois. Índice em `docs/evidencias/depois.md`. Carga padrão 2026-11-23 02:14:14–02:14:25 UTC: baseline 10/10 (100%), p95 2,04 s, 1,0 req/s; carga 200/200 (100%), p95 243 ms, 579,5 req/s. O breaker não abriu nessa carga. A prova sequencial que abre está na seção Medição do depois.
- [x] 10 SAD seções 5 a 8 em `docs/sad.md`. README do processo na raiz. Enunciado em `docs/enunciado.md`. A medição não desmentiu limiar: timeout 2500 ms, aberto 30 s e os alvos da seção 3 permanecem. A carga padrão não abriu o breaker; a prova de 40 POST permanece a evidência de abertura. O veredito da seção 8 paga nos hit rates de 40% e de 10%. O negativo começa se o custo fixo mensal passar da economia (R$ 31.680 no caso de 10%).
- [x] 11 Revisão de aceite em 2026-11-23. Checklist abaixo, todos marcados. Nenhum item voltou de etapa. Sem commit.

## Aceite

SAD:

- [x] 6 RF (RF-01 a RF-06). Criados pela arquitetura: RF-04, RF-05, RF-06. RF-03 é proposta.
- [x] 5 RNF com métrica, valor, unidade, método e origem. Hoje medido nesta máquina: RNF-01 a RNF-05 (`docs/evidencias/antes-reproduce.txt` e o trace `251f53502c68ca6036455b872b7e795e`, 6148691 µs na partner-degrading).
- [x] 3 pressupostos com valor, origem e consequência. Seção 1 de `docs/sad.md`.
- [x] 4 diagramas C4 Mermaid: contexto, contêineres antes, contêineres depois, fatia.
- [x] 4 decisões com contexto, opções, escolha, consequências ruins e alternativa descartada: breaker, cache, fallback, hospedagem.
- [x] 4 limites com gatilho e efeito, sem implementação: concorrência, breaker na memória, TTL único, uma corretora ocupando as outras.
- [x] 3 alertas com métrica, limiar e ação. A página automática é proposta: `deploy/prometheus/prometheus.yml` não tem `rule_files`.
- [x] 1 runbook da partner-degrading aberta há dez minutos, até o escalonamento, com o que o plantonista não faz.
- [x] 3 cenários: Redis perdido, parceira fora seis horas, perda da região. Em cada um, o que a corretora vê, o que degrada, o que para, o custo e a volta.
- [x] 3 linhas de parceiro: hit 0%, 40% e 10%.
- [x] Auditoria no mês 12 (30,2 GiB, R$ 3,62) e no mês 60 (151,1 GiB, R$ 18,13), por cotação apresentada.

PoC:

- [x] Estados `closed`, `open`, `half-open` em `internal/partner/client.go`. Aberto devolve `gobreaker.ErrOpenState` antes de `http.Client.Do`. Escopo: um breaker por parceira.
- [x] Timeout 2500 ms. Estouro conta como falha. `context.Canceled` não conta.
- [x] Chave `quote:{tenant_id}:{partner}:{document}:{birth_year}:{plate}:{model}:{year}:{value_cents}:{coverage}` em `internal/quotation/cache.go`. Tenant vem do cabeçalho. Invalidação: só o TTL de 900 s.
- [x] Fallback no contrato: `degraded` e `unavailable_partners` sempre presentes. Nenhuma proposta: 503 `no_quote`, `retry_after_seconds` 30.
- [x] Métricas no README: `partner.breaker.state`, `partner.breaker.transitions`, `quote.cache.hit`, `quote.cache.miss`, e `http.client.request.duration`. Marcas: `circuit.short_circuit` e `cache.hit`.
- [x] Testes determinísticos: `TestOpenCircuitDoesNotAdvanceTheFlakySequence` abre na chamada 42; cache e fallback em `service_test.go` e `handler_test.go`, sem sleep de latência.
- [x] Sem CPF, placa ou `quote_id` em atributo. `tenant_id` só no evento `cache.hit`. Conferido nos traces do depois.

Evidências e repositório:

- [x] 7 linhas em `docs/evidencias/depois.md`, cada uma com comando ou consulta e legenda.
- [x] Antes e depois em texto, carga padrão, nesta máquina. O antes não é o bloco do roteiro: a janela é 2026-11-17 22:12:31 UTC e a carga durou 24,85 s. O exemplo do roteiro é 30/07/2026 16:17:53 UTC e 24,90 s.
- [x] Gráficos com PromQL e janela: `depois-breaker.txt`, `depois-hitrate.txt`, `depois-p95.txt`, `antes-prometheus.txt`.
- [x] `docs/evidencias/make-test.txt`, `go test ./...`, exit 0 em 2026-11-23 02:12:28–02:13:04 UTC.
- [x] Caminho citado existe, ou está marcado como proposta. Trace do breaker `99e2e3a474b06db7472daf239464ce31` tem `circuit.short_circuit` da partner-flaky e HTTP POST só de slow e degrading. Trace de cache `0ccf339739aef01ecc02b3e8115559ac` tem três `cache.hit` e nenhum span de saída.
- [x] TTL 900, 5 falhas, janela 60 s, aberto 30 s, 1 prova, timeout 2500 ms e os nomes de métrica batem com `internal/platform/config.go` e com os instrumentos.
- [x] Hit rate da seção 8 é 40% e 10%, o pressuposto da seção 1 com TTL de 900 s. O 0,910 da carga não entra na planilha.
- [x] README na raiz com SAD, evidências, como reproduzir, implementado e proposta, métricas, o que faria com mais tempo, e o parágrafo do que se protegeu.
- [x] `docs/enunciado.md`, `docs/sad.md` e `docs/evidencias/` no lugar.

Reprova sozinho: não apareceu. Chave leva a corretora. Há SAD e há PoC.

## Medição do antes

Medido por `make reproduce` depois de `make down`, nesta máquina (Windows 11 Education, Intel Core i5-12450H, Docker Engine 27.4.0, Compose v2.31.0). O relatório imprime UTC.

- comando: `make down` e em seguida `make reproduce`
- quando: 2026-11-17 22:12:31 UTC a 22:13:14 UTC (carga 22:12:50 UTC a 22:13:14 UTC)
- baseline sucesso / p95: 5 de 10 (50%), p95 2.04 s, 0.5 req/s; 5 falhas HTTP 502 da partner-flaky
- carga sucesso / p95 / vazão: 120 de 200 (60%), p95 8.01 s, máximo 8.05 s, 8.0 req/s em 24.85 s (3.9× o p95 do baseline); 80 falhas HTTP 502 da partner-flaky
- artefato: `docs/evidencias/antes-reproduce.txt`, `docs/evidencias/antes.md`, `docs/evidencias/antes-trace-longo.json` (trace `251f53502c68ca6036455b872b7e795e`), `docs/evidencias/antes-trace-erro.json` (trace `4feb557ea0aeb06e78eabc953eebfc59`). Consulta PromQL opcional em `docs/evidencias/antes-prometheus.txt`, avaliada em 22:13:14 UTC.

## Medição do depois

Medido por `make down` e em seguida `make reproduce`, nesta máquina (Windows 11 Education, Intel Core i5-12450H, Docker Engine 27.4.0, Compose v2.31.0). O relatório imprime UTC. Índice das sete linhas: `docs/evidencias/depois.md`.

- comando: `make down` e em seguida `make reproduce`; `make test` no mesmo dia
- quando: carga 2026-11-23 02:14:14 UTC a 02:14:25 UTC; suíte 02:12:28 UTC a 02:13:04 UTC
- baseline sucesso / p95: 10 de 10 (100%), p95 2,04 s, 1,0 req/s
- carga sucesso / p95 / vazão: 200 de 200 (100%), p95 243 ms, máximo 274 ms, 579,5 req/s em 345 ms
- histogram_quantile do `POST /quotes`, mesma consulta do antes, escala 0–10 s: 0,331 s a 0,343 s entre 02:14:40 e 02:15:30 UTC. O antes, na avaliação de 2026-11-17T22:13:14Z, foi 9,387 s
- hit rate da carga, razão acumulada, três parceiras: 0 às 02:14:30 UTC (7 misses, hit ainda ausente) e 0,910 às 02:14:40 UTC (573 hits, 57 misses). Por parceira: slow 0,976, degrading 0,976, flaky 0,776. Não alimenta a seção 8
- o breaker não abriu na carga padrão. partner-flaky, partner-slow e partner-degrading ficaram em 0. O cache e as 50 em voo não produziram 5 falhas consecutivas. A partner-degrading não foi ao teto: a carga de 345 ms saiu do cache
- prova extra, obrigatória no SAD: 40 POST sequenciais em `http://localhost:8080/quotes`, `X-Tenant-Id: corretora-b`, um risco novo por chamada, 2026-11-23 02:18:46–02:19:58 UTC. `partner_breaker_state` da partner-flaky passa de 0 para 2 às 02:19:10 UTC e segue em 2 até 02:22:30 UTC. Transições: `closed → open` às 02:19:10; `open → half-open` e `half-open → open` às 02:19:40. Slow e degrading permanecem em 0
- artefatos: `docs/evidencias/depois-reproduce.txt`, `docs/evidencias/make-test.txt`, `docs/evidencias/depois-trace-cache.json` (trace `0ccf339739aef01ecc02b3e8115559ac`), `docs/evidencias/depois-trace-breaker.json` (trace `99e2e3a474b06db7472daf239464ce31`), `docs/evidencias/depois-breaker.txt`, `docs/evidencias/depois-hitrate.txt`, `docs/evidencias/depois-p95.txt`

## Decisões

Preencher na etapa 3. Números daqui são os do SAD e os do código.

### Pressupostos

1. Sem cache: 120.000 cotações por dia útil, 22 dias úteis, 3 consultas, R$ 0,04 por consulta. Custo mensal = 120.000 × 22 × 3 × 0,04 = R$ 316.800. Receita = 120.000 × 22 × 0,25 = R$ 660.000. Origem: cenário do enunciado. Se o volume cair pela metade, a economia cai pela metade. A auditoria de 5 anos não diminui com o cache. Na seção 8 ela fica em dezenas de reais por mês e não decide o veredito. O veredito muda se o custo fixo mensal ficar entre a economia cheia e a economia pela metade.
2. Nesta máquina (Windows 11 Education, Intel Core i5-12450H, Docker Engine 27.4.0, Compose v2.31.0): carga 120/200 (60%), p95 8,01 s, 8,0 req/s; baseline 5/10 (50%), p95 2,04 s; trace longo com a partner-degrading em 6149 ms. Janela 2026-11-17 22:12:31–22:13:14 UTC. Origem: medição da etapa 2. Se uma medição nova não repetir esse p95 de carga, a coluna hoje do SAD fica inválida.
3. Hipótese explícita de recotação: 40% das cotações da mesma corretora e do mesmo risco se repetem dentro de 900 s. Origem: hipótese declarada; o cenário diz que o corretor recota na mesma conversa e não dá percentual. Economia = 0,40 × 316.800 = R$ 126.720 por mês. Se a repetição for 10%, a economia cai para R$ 31.680 por mês. O TTL de 900 s sustenta os dois, e a seção 8 leva as duas linhas. Com o custo fixo de lá, as duas se pagam. O veredito fica negativo quando o custo fixo mensal passa da economia: R$ 31.680 no caso de 10%, R$ 126.720 no caso de 40%.

### Alvos não funcionais

| ID | Métrica | Hoje | Alvo | Como medir | Origem do alvo |
|---|---|---|---|---|---|
| RNF-01 | p95 do POST /quotes sob carga | 8,01 s | 2,5 s | make reproduce / http_server_request_duration_seconds | soma em repouso 1,50 + 0,15 + 0,12 = 1,77 s, mais margem; o baseline já mediu 2,04 s |
| RNF-02 | p95 do baseline (10 req) | 2,04 s | ≤ 2,5 s | make reproduce | não piorar o caminho feliz já medido |
| RNF-03 | carga com ao menos uma proposta | 60% (120/200) | ≥ 95% | make reproduce | o 40% da partner-flaky zera a resposta hoje; a parcial recupera as outras |
| RNF-04 | vazão da carga | 8,0 req/s com p95 8,01 s | 8,0 req/s com p95 ≤ 2,5 s | make reproduce | a carga oferecida já é 8 req/s |
| RNF-05 | timeout do http.Client | ilimitado (trace da partner-degrading em 6149 ms) | 2500 ms, e estouro conta como falha do breaker | teste do client e span | a partner-slow chega a ~1700 ms; 1500 ms a derrubaria; 6000 ms não veria a partner-degrading |

### Circuit breaker

- biblioteca: github.com/sony/gobreaker/v2, com os estados closed, open e half-open pelo nome. Alternativa descartada: github.com/afex/hystrix-go, que não expõe o meio-aberto como estado nomeado.
- escopo: um breaker por parceira. Com a partner-flaky aberta, ela não é chamada; partner-slow e partner-degrading seguem; a corretora recebe as outras duas. Breaker global descartado: a flaky derrubaria as três.
- abrir com 5 falhas consecutivas / janela de 60 s (a contagem zera no sucesso e no fim da janela) / tempo aberto 30 s / 1 prova no meio-aberto. 1 sucesso fecha. Falha da prova reabre por mais 30 s.
- timeout 2500 ms / estouro conta como falha: sim.
- onde o teste determinístico abre: na 5ª falha consecutiva da semente 20260729, a chamada 42. A chamada 43 não chega ao mock e a sequência permanece em 42. A rajada 49–57 continua no mock; com o circuito aberto por 30 s essa rajada não é consultada.

### Cache

- chave literal: `quote:{tenant_id}:{partner}:{document}:{birth_year}:{plate}:{model}:{year}:{value_cents}:{coverage}`. tenant_id só do cabeçalho X-Tenant-Id. document só dígitos; plate A-Z0-9 maiúsculo; model maiúsculo com espaços colapsados; coverage o enum já validado.
- por parceira. Cache agregado descartado: uma partner-flaky aberta perderia partner-slow e partner-degrading já obtidos.
- TTL: 900 s. Abaixo do teto comercial de 24 h. A conversa do corretor cabe em 15 min e o CPF sai do Redis nesse prazo. O valid_for_seconds de 300 s do mock não é o teto.
- com breaker aberto: entrada dentro do TTL é servida e a parceira não é chamada. Entrada vencida não é servida.
- invalidação e descarte: nenhuma invalidação antes do TTL. Descarte no TTL. O Redis efêmero também descarta no restart. A auditoria de 5 anos não mora nesse Redis.

### Fallback

- opção: resposta parcial.
- como a resposta mostra degradação: HTTP 200 se houver ao menos uma proposta; quotes ordenadas por premium_cents crescente; campos degraded (bool) e unavailable_partners (lista). Prêmio só de parceira ou de cache dentro do TTL.
- status e corpo quando não há o que entregar: HTTP 503, corpo no_quote, lista das parceiras e retry_after_seconds 30. Distinto do 502 partner insurer unavailable.

### Hospedagem

- escolha: nuvem com região no Brasil. O PoC permanece no Compose local, sem custo.
- alternativa descartada: nuvem fora do Brasil, porque CPF e auditoria de 5 anos viram transferência internacional (ANPD); on-premise, porque 120.000 cotações por dia e o pico medido de 8 req/s não pagam um datacenter, e o storage de auditoria em região brasileira custa menos que um site próprio.
- motivo regulatório e financeiro: residência do CPF e da auditoria SUSEP de 5 anos no Brasil; o volume medido não justifica site próprio.

### Limites que não se implementam

1. Sem limite de chamadas simultâneas por parceira. Gatilho: mais de 5 chamadas simultâneas na partner-degrading. Efeito: ela afunda até o timeout de 2500 ms, o breaker dela abre, e toda corretora perde esse preço por 30 s.
2. Breaker na memória do processo, cada réplica aprende sozinha. Gatilho: duas réplicas e só uma viu as 5 falhas. Efeito: a outra continua chamando, e a corretora que cair nela espera o timeout. No PoC há uma réplica.
3. TTL como única invalidação do cache. Gatilho: a seguradora muda o prêmio antes de 900 s, ou o titular pede exclusão. Efeito: a corretora vê o prêmio antigo até o TTL; a exclusão não é imediata. A janela de CPF no cache é 900 s.
4. Uma corretora consumindo a capacidade das outras. Gatilho: um tenant sustenta as 50 chamadas em voo. Efeito: a partner-degrading afunda para todos os tenants, o p95 deles vai ao timeout e o breaker abre para todo mundo.

### Métricas (etapa 8)

Nomes do instrumento no código. O Collector troca o ponto por sublinhado e acrescenta `_total` no contador. É esse nome do Prometheus que a consulta usa.

- breaker: `partner.breaker.state` (gauge, atributo `partner`; 0 closed, 1 half-open, 2 open). Prometheus: `partner_breaker_state`. Contador `partner.breaker.transitions`, atributos `partner`, `from_state`, `to_state`. Prometheus: `partner_breaker_transitions_total`. A prova do meio-aberto dura uma chamada, menos que o export de 10 s; o contador guarda `open → half-open → closed`. O gauge desenha o aberto (2) pelos 30 s.
- hit / miss: `quote.cache.hit` e `quote.cache.miss`, atributo `partner`. Prometheus: `quote_cache_hit_total` e `quote_cache_miss_total`. Hit rate: `sum(rate(quote_cache_hit_total[5m])) / (sum(rate(quote_cache_hit_total[5m])) + sum(rate(quote_cache_miss_total[5m])))`. Esse gráfico da carga não alimenta a seção 8.
- latência por parceira: reuso de `http.client.request.duration`. Prometheus: `http_client_request_duration_seconds`, atributo `server_address`. Consulta na seção 6 do SAD: `histogram_quantile(0.95, sum by (server_address, le) (rate(http_client_request_duration_seconds_bucket[1m])))`. Cache hit e circuito aberto não geram ponto.
- marcas de trace: `circuit.short_circuit` (atributos `partner`, `breaker.state`), sem span de saída da parceira; `cache.hit` (atributos `partner`, `tenant_id`). Sem CPF, placa ou `quote_id`.
