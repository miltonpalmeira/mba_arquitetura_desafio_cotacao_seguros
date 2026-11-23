# Estado do desafio

etapa_atual: 6
etapa_nome: Cache
decisoes_confirmadas: sim

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
- [x] 4 SAD seções 1 a 4 em `docs/sad.md`. Seções 5 a 8 só com o título. Quatro diagramas Mermaid.
- [x] 5 Timeout de 2500 ms e circuit breaker por parceira em `internal/partner/client.go`. Estouro conta como falha. Teste abre na chamada 42 da semente 20260729; a chamada 43 não avança a sequência do mock.
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

1. Sem cache: 120.000 cotações por dia útil, 22 dias úteis, 3 consultas, R$ 0,04 por consulta. Custo mensal = 120.000 × 22 × 3 × 0,04 = R$ 316.800. Receita = 120.000 × 22 × 0,25 = R$ 660.000. Origem: cenário do enunciado. Se o volume cair pela metade, a economia cai pela metade e a seção 8 pode mudar de veredito, porque a auditoria de 5 anos não diminui com o cache.
2. Nesta máquina (Windows 11 Education, Intel Core i5-12450H, Docker Engine 27.4.0, Compose v2.31.0): carga 120/200 (60%), p95 8,01 s, 8,0 req/s; baseline 5/10 (50%), p95 2,04 s; trace longo com a partner-degrading em 6149 ms. Janela 2026-11-17 22:12:31–22:13:14 UTC. Origem: medição da etapa 2. Se uma medição nova não repetir esse p95 de carga, a coluna hoje do SAD fica inválida.
3. Hipótese explícita de recotação: 40% das cotações da mesma corretora e do mesmo risco se repetem dentro de 900 s. Origem: hipótese declarada; o cenário diz que o corretor recota na mesma conversa e não dá percentual. Economia = 0,40 × 316.800 = R$ 126.720 por mês. Se a repetição for 10%, a economia cai para R$ 31.680 por mês e a seção 8 muda de veredito.

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

- breaker:
- hit / miss:
- latência por parceira:
- marcas de trace:
