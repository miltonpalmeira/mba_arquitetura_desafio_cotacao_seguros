# Etapas

Uma etapa termina quando o critério de saída está cumprido e o usuário autoriza a próxima. Não antecipe arquivo de etapa futura.

## 0. Ferramentas

Critério de saída: Go 1.25 e Docker Compose v2 respondem no terminal. Nada de código, nada de `docs/sad.md`.

1. `go version` precisa anunciar go1.25. O `go.mod` pede `go 1.25.0`.
2. `docker compose version` precisa ser Compose v2.
3. `make help` lista os alvos. Se não houver `make`, os alvos do `Makefile` são uma linha de `docker compose` ou de `go`; registre o substituto em `estado.md`.
4. Não rode `make up` ainda se o usuário só pediu a checagem.

## 1. Leitura, sem escrever solução

Critério de saída: `estado.md` resume os quatro buracos com caminho real e o usuário confirma que entendeu o contrato.

Ler, nesta ordem, só o que falta:

- `README.md` seções "Comece por aqui", "Entrega 1", "Entrega 2", "Critérios de aceite", "O cenário".
- `docs/guia-sad.md` por completo.
- `internal/partner/client.go` — `http.Client` sem `Timeout`.
- `internal/quotation/service.go` — laço em série; erro de uma parceira aborta e devolve `Response{}`.
- `internal/quotation/request.go` — `Response` hoje é `tenant_id`, `quotes`, `elapsed_ms`.
- `internal/quotation/handler.go` — `X-Tenant-Id`; 502 `partner insurer unavailable`.
- `internal/platform/config.go` e `internal/platform/telemetry.go` — Redis ainda não entra na config; OTel já sobe.
- `cmd/partner-mock/feasibility_test.go` — molde do teste determinístico. Rajada de 9 falhas nas sequências 49 a 57 da `partner-flaky`, semente `20260729`.
- `docker-compose.yml` — nomes que o C4 tem de repetir: `quotation-api`, `partner-slow`, `partner-flaky`, `partner-degrading`, `redis`, `otel-collector`, `jaeger`, `prometheus`.

Os quatro buracos, todos de propósito:

1. Sem timeout — `internal/partner/client.go`.
2. Agregação em série — `internal/quotation/service.go`. Opcional paralelizar; não é esta etapa.
3. Tudo ou nada — a mesma `service.go`. Uma falha vira 502 e joga fora as respostas já obtidas.
4. Redis sobe e a API não fala com ele; OTel emite só HTTP de entrada, HTTP de saída e runtime.

Parceiras, para usar nas decisões e não redescobrir depois:

| Parceira | Porta | Comportamento |
|---|---|---|
| partner-slow | 9001 | 1500 ms ± 200 ms, nunca falha |
| partner-flaky | 9002 | 150 ms ± 50 ms, 40% de 503 |
| partner-degrading | 9003 | 120 ms ± 40 ms; acima de 5 simultâneas soma 300 ms por chamada extra, teto 6000 ms; nunca falha |

Conta do cenário, para a seção 8 mais tarde: R$ 0,04 por consulta, 3 consultas por cotação, 120.000 cotações por dia útil, 22 dias úteis, preço cobrado R$ 0,25 por cotação entregue. Teto comercial do prêmio: 24 h. `valid_for_seconds` do mock vale 300 s e não é o teto comercial. Auditoria SUSEP: 5 anos, por cotação apresentada, inclusive cache e fallback. LGPD: Prumo é operadora; corretora é controladora.

Não proponha biblioteca nem TTL nesta etapa. Entregue um resumo de uma página e pare.

## 2. O antes, medido nesta máquina

Critério de saída: texto completo do `make reproduce` salvo em `docs/evidencias/`, mais nota do que se viu no Jaeger. Números do roteiro ficam de fora.

1. `make down` antes, para zerar a sequência das parceiras.
2. `make reproduce` (sobe com `--wait` e dispara 10 de baseline e depois 200 com 50 em voo). Primeira compilação leva cerca de um minuto a mais. A primeira cotação pode voltar 502 em ~1,9 s: isso é o cenário.
3. Cole a saída completa em `docs/evidencias/antes-reproduce.txt`. Uma linha de legenda no topo: o que a execução mostra (p95, sucesso, vazão).
4. Com o ambiente ainda de pé, Jaeger em http://localhost:16686, serviço `quotation-api`, operação `POST /quotes`, lookback Last Hour. Guardar:
   - trace longo: três `HTTP POST` em cascata, cada um começando quando o anterior termina;
   - trace `error=true`: o span da parceira seguinte à que falhou não existe; a ponta responde 502.
   Formato preferível: export JSON do Jaeger. Screenshot PNG/JPG só se o export não for possível, legível, abaixo de 5 MB. Legenda de uma linha em `docs/evidencias/antes.md` com a janela.
5. Opcional, Prometheus http://localhost:9090, janela explícita:

```promql
histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[1m])))
```

```promql
histogram_quantile(0.95, sum by (server_address, le) (rate(http_client_request_duration_seconds_bucket[1m])))
```

6. Registre em `estado.md` os números **desta** execução: sucesso e p95 do baseline, sucesso e p95 da carga, horário, máquina em uma frase. Não copie o p95 de 8,01 s do roteiro.

Não escreva SAD. Não altere Go. `make down` só depois de capturar gráfico e trace.

## 3. Portão de decisões

Critério de saída: o bloco abaixo preenchido em `estado.md` e confirmado pelo usuário, palavra por palavra nos números. Sem isso, a etapa 4 não começa.

Apresente cada bloco, recomende uma opção e pare. Pode agrupar as perguntas num único formulário, mas não preencha o SAD ainda.

### 3.1 Três pressupostos

Cada um com valor, origem e consequência se for falso. O de recotação alimenta o hit rate da seção 8. Exemplo de forma, não de número a copiar: "assumo que X% das cotações da mesma corretora e da mesma placa se repetem dentro do TTL; se for Y%, a seção 8 muda de veredito".

Origem aceitável: o cenário do enunciado, a medição da etapa 2, ou declaração explícita de hipótese. "Porque é razoável" não serve.

### 3.2 Alvos não funcionais

Pelo menos cinco, no formato métrica, valor, unidade, método, origem do alvo. Pelo menos três com a coluna "hoje" igual à etapa 2. O alvo de p95 não pode ser "um bom número": vem do tempo que o corretor espera com o cliente na frente, de um SLA declarado, ou da soma das parceiras no melhor caso (lenta ~1,5 s mais as outras duas em repouso).

### 3.3 Circuit breaker

Decidir e registrar:

- biblioteca Go que exponha os três estados pelo nome no código (fechado, aberto, meio aberto), e uma alternativa descartada;
- escopo: um por parceira, um por par (parceira, corretora), ou um global. Diga o efeito se a `partner-flaky` abrir o circuito;
- falhas para abrir, janela, tempo aberto, quantas provas o meio aberto deixa passar, o que fecha e o que reabre;
- timeout do cliente, em ms, e se estouro conta como falha. Recomendação a defender, não a impor: contar estouro como falha, senão a `partner-degrading` não é vista;
- em qual sequência da `partner-flaky` esse limiar abre, usando a rajada 49–57 como teste, sem depender de sorte.

Breaker global é defendível e quase sempre a pior opção num agregador de três fornecedores independentes.

### 3.4 Cache

Decidir e registrar a chave **literal**, com `tenant_id` vindo do cabeçalho. Mínimo: corretora e identificação normalizada do risco. Dizer se a chave inclui a parceira ou se o valor é a cotação agregada.

- Cache por parceira sobrevive a uma parceira fora.
- Cache agregado economiza as três consultas e invalida inteiro.

TTL em segundos, menor ou igual a 24 h. Dizer por que parar antes do teto, ou por que usar os 300 s de `valid_for_seconds`. Esse número é o mesmo da planilha.

Política quando o breaker abre: a entrada vencida ainda é servida, por quanto tempo, e como a corretora fica sabendo. O que invalida antes do TTL. Como o dado pessoal sai quando tem de sair (Redis deste compose é efêmero; prazo de descarte é obrigação, não volume Docker).

### 3.5 Fallback

Uma só:

- resposta parcial, com a parceira que faltou nomeada, ordem por prêmio crescente mantida;
- cotação anterior do cache, marcada, com a idade na resposta;
- recusa explícita, status e corpo distintos do 502 genérico `partner insurer unavailable`, dizendo o que a corretora faz.

Nos três casos o cliente distingue degradação de sucesso lendo a resposta. O campo novo entra em `Response` (`internal/quotation/request.go`). Prêmio inventado é proibido.

### 3.6 Quatro limites que não se implementam

Já são exigidos pelo aceite. Registrar gatilho e efeito na corretora, sem código:

1. sem limite de chamadas simultâneas por parceira;
2. breaker na memória do processo, cada réplica aprende sozinha;
3. TTL como única invalidação do cache;
4. uma corretora consumindo a capacidade das outras.

## 4. SAD, seções 1 a 4

Critério de saída: `docs/sad.md` com as seções 1 a 4, quatro diagramas Mermaid, e nenhum número diferente do `estado.md`. Seções 5 a 8 ficam como título mais a frase "fecha depois da evidência do PoC".

Escrever para três leitores: CTO da Prumo (a conta), quem opera às 3h (o que fazer), encarregado de dados (ANPD e SUSEP, cache rastreável, corretora nunca vê dado de outra). Frase que não serve a nenhum dos três sai.

Conteúdo mínimo, detalhe em `docs/guia-sad.md`:

1. Introdução: propósito, escopo (plataforma e a fronteira com as seguradoras; não redesenha o motor delas nem o CRM), restrições separadas das decisões, três pressupostos.
2. Visão: C4 nível 1; C4 nível 2 do antes; C4 nível 2 do depois; delta em lista. Nomes iguais aos do `docker-compose.yml`. Seta com o que trafega e a tecnologia. Contêiner que não existe nem no compose nem na entrega, e não está marcado como proposta, reprova.
3. Seis RF com id e frase testável, pelo menos dois criados pela arquitetura (breaker, cache ou fallback). Cinco RNF na tabela do guia, três com "hoje" da etapa 2.
4. Quatro decisões no formato contexto, opções, escolha, consequências ruins: breaker, cache, fallback e, se a hospedagem ainda não estiver fechada, deixar a quarta para a seção 5 e não contar esta seção como completa até ela existir. Chave de cache por extenso. C4 nível 3 só da fatia `client.go` e `service.go`. Quatro limites da etapa 3.6.

Diagrama: `C4Context` / `C4Container` ou `flowchart` com pessoa, sistema, contêiner e sistema externo distinguíveis. Nível 4 não se pede.

Não implementar código nesta etapa. Se um número da etapa 2 estiver ausente, voltar à etapa 2 em vez de estimar.

## 5. Timeout e circuit breaker

Critério de saída: os três estados acontecem em teste determinístico; aberto não chama a parceira; `make test` verde; SAD seção 4 e código com o mesmo limiar.

1. Timeout no cliente, valor do `estado.md`, via `internal/platform/config.go` (sem número mágico solto se o SAD o trata como parâmetro).
2. Breaker no caminho de `internal/partner/client.go`. Estado aberto retorna antes do `http.Client.Do`.
3. Estouro de timeout entra ou não no contador, conforme a decisão 3.3.
4. Teste no molde de `feasibility_test.go`: a rajada 49–57 (ou a janela escolhida) abre o circuito, e a chamada seguinte não incrementa a sequência do mock. Sem `sleep`.
5. Não acrescentar cache nem fallback neste turno.
6. Se o limiar que parecia certo não abre dentro de uma execução de 210 chamadas, parar e revisar a decisão 3.3 com o usuário antes de "afrouxar" em silêncio. Mudou o número: muda o SAD no mesmo turno.

`make smoke` continua verde e não substitui o teste novo.

## 6. Cache

Critério de saída: chave idêntica à escrita na seção 4, contendo a corretora; teste determinístico de hit sem chamar a parceira; Redis ligado no boot (`cmd/quotation-api/main.go` e compose, com moderação).

1. Cliente Redis. O serviço já existe no compose; a API é que passa a falar com ele.
2. Montar a chave com o `tenant` do cabeçalho, não com o corpo.
3. Gravar só cotação que veio de parceira (ou composição rastreável dessas cotações). Guardar o bastante para a auditoria apontar a consulta de origem (`partner`, `quote_id` no valor, não no rótulo de métrica).
4. Aplicar a política de entrada vencida com breaker aberto decidida em 3.4.
5. Teste: segunda chamada com a mesma corretora e o mesmo risco não chama o quoter; outra corretora com o mesmo corpo não reutiliza a entrada.
6. Não implementar fallback nesta etapa, salvo o que a política de breaker aberto já exigiu e o SAD já descreveu. Se a política e o fallback forem a mesma coisa, implementar juntos na etapa 7 e deixar o cache da etapa 6 só com hit/miss em circuito fechado. Registrar a escolha em `estado.md`.

## 7. Fallback

Critério de saída: a corretora deixa de receber 502 genérico no caso decidido; a degradação é visível no JSON; ordenação por `premium_cents` crescente permanece; teste determinístico; `make test` verde.

1. Alterar `Response` e documentar o campo no SAD (proposta vira fato).
2. Uma parceira falha e as outras já responderam: aplicar a opção 3.5. Não descartar prêmio já obtido se a opção for resposta parcial.
3. Sucesso integral permanece 200 com as três cotações.
4. Mensagens de erro já existentes em `handler.go` e `request.go` continuam a fonte única. Erro novo entra nessa lista no SAD e no README do processo, não num nome inventado na hora.
5. Teste do fallback sem rede sorteada: quoter falso que falha numa parceira nomeada.

## 8. Instrumentação

Critério de saída: três métricas de negócio emitidas e duas marcas de trace visíveis; nomes gravados em `estado.md` para o README do processo; nenhum CPF, placa ou `quote_id` em atributo.

1. Transição ou estado atual do breaker, por parceira, suficiente para desenhar fechado, aberto, meio aberto, fechado. A requisição curto-circuitada marca o trace. Sem o span de saída da parceira.
2. Contadores `hit` e `miss` com atributo que permita hit rate em PromQL. Esse gráfico da carga não alimenta a seção 8.
3. Latência por parceira com significado de domínio, ou reuso explícito de `http_client_request_duration_seconds` com a consulta PromQL escrita no SAD. Reusar é legítimo se estiver declarado.
4. SDK já está em `internal/platform/telemetry.go`. Não reescrever o plumbing. Confirmar no Prometheus (http://localhost:9090) e no Jaeger (http://localhost:16686) com o ambiente de pé.
5. Instrumentar pouco. A seção 6 do SAD vai ter de dizer quem olha cada série.

## 9. Evidências do depois

Critério de saída: as sete linhas da tabela do enunciado existem em `docs/evidencias/`, cada uma com comando ou consulta, artefato e legenda de uma linha.

Carga obrigatória igual à do antes: `make down && make reproduce` (10 + 200 com 50 em voo). Não comparar com outra concorrência. Variação extra é permitida se o SAD disser que rodou.

| Arquivo | O que mostra |
|---|---|
| `antes-reproduce.txt` e `depois-reproduce.txt` | texto completo das duas execuções suas |
| saída de `make test` em texto | suíte inteira verde, testes antigos e novos |
| trace breaker aberto | JSON ou screenshot: não há span de saída da parceira curto-circuitada |
| trace de cache | cotação sem spans de saída para as parceiras atendidas pelo cache |
| estado do breaker | gráfico com PromQL e janela; o degrau, não a métrica parada |
| hit rate | gráfico com PromQL e janela; a curva sobe ao aquecer |
| p95 do `POST /quotes` | um gráfico do antes e um do depois, mesma escala, janela declarada, consulta colada |

Capturar gráfico e trace antes de `make down`. O Prometheus retém cerca de uma hora e não tem volume.

Advertência para o SAD: o loadgen é laço fechado. Com a ponta mais rápida, os mesmos 50 em voo disparam mais chamadas por segundo e a `partner-degrading` pode aparecer pior. Comparar taxa de sucesso, p95 da ponta e vazão.

## 10. SAD 5 a 8 e README do processo

Critério de saída: as oito seções se referenciam; números batem com o código e com a etapa 9; README novo na raiz; enunciado movido.

1. Se a evidência desmentiu um limiar ou um alvo, corrigir o SAD antes de fechar. Divergência perde a entrega; mudar de ideia com medição é o esperado.
2. Seção 5: mapa mecanismo para arquivo real; biblioteca e por que ela; teste determinístico; onde roda (on-premise, cloud ou híbrido) decidido por retenção de 5 anos, residência e segregação LGPD, com uma alternativa descartada e o motivo regulatório e financeiro. Esse custo reaparece na seção 8.
3. Seção 6: três alertas com métrica, limiar e ação; um runbook do sintoma ao escalonamento (cenário natural: breaker da `partner-degrading` aberto há dez minutos), incluindo o que o plantonista não faz; mudança do TTL (quem aprova, como chega em produção, como reverte, como se mede).
4. Seção 7: RTO e RPO por classe de dado (cache não é auditoria). Três cenários: Redis perdido, parceira fora por seis horas, perda do site ou da região. Em cada um: o que a corretora vê, o que degrada, o que para, custo em reais do modo degradado, caminho de volta. Redis perdido recoloca 100% das consultas e empurra a `partner-degrading` para o teto de 6000 ms; dizer qual efeito chega primeiro.
5. Seção 8: três linhas de parceiro (hoje com hit rate zero e pelo menos dois hit rates que o TTL sustenta, coerentes com o pressuposto da seção 1); infraestrutura acrescentada (Redis, retenção de telemetria, auditoria no ano 1 e no ano 5 por cotação apresentada, não por consulta comprada); pessoal para construir e para operar; veredito de payback, inclusive se o cache não se paga.
6. Mover o README atual para `docs/enunciado.md`. O README novo da raiz tem só: link para `docs/sad.md` e `docs/evidencias/`; como subir e reproduzir a versão entregue; o que foi implementado e o que ficou proposta; nomes de métricas e atributos; o que faria diferente com mais tempo; um parágrafo do que se protegeu, com o quê, e o que isso custou.

Cada requisito da seção 3 aparece como mecanismo na 4, arquivo na 5, alerta na 6, cenário na 7 e reais na 8.

## 11. Revisão de aceite

Critério de saída: checklist abaixo marcado no `estado.md`. Item falho volta para a etapa que o produz. Não empurrar para o git até o usuário pedir commit.

SAD:

- 6 RF, pelo menos 2 criados pela arquitetura
- 5 RNF com métrica, valor, unidade, método e origem; 3 com "hoje" medido aqui
- 3 pressupostos com valor, origem e consequência
- 4 diagramas C4 Mermaid: nível 1, nível 2 antes, nível 2 depois, nível 3 da fatia
- 4 decisões contexto, opções, escolha, consequências ruins (breaker, cache, fallback, hospedagem), cada uma com alternativa descartada
- 4 limites conhecidos com gatilho e efeito, nenhum implementado
- 3 alertas com métrica, limiar e ação
- 1 runbook até o escalonamento
- 3 cenários de desastre
- 3 linhas de custo de parceiro
- auditoria no ano 1 e no ano 5 por cotação apresentada

PoC:

- três estados nomeados; aberto não chama a parceira; escopo declarado
- timeout defendido no SAD; efeito do estouro sobre o breaker declarado
- chave por extenso com a corretora; invalidação declarada
- fallback no contrato, degradação visível
- 3 métricas com o nome no README; 2 marcas de trace
- 2 testes determinísticos (breaker abre; cache ou fallback)
- sem CPF, placa ou `quote_id` em atributo

Evidências e repositório:

- 7 linhas com comando ou consulta e legenda
- antes e depois em texto, carga padrão, ambos desta máquina
- gráfico com PromQL e janela
- `make test` colado, suíte verde
- todo caminho citado existe ou está como proposta
- TTL, limiares e métricas iguais no SAD e no código
- hit rate da seção 8 compatível com TTL e com o pressuposto
- README do processo com as cinco coisas, na raiz
- `docs/enunciado.md`, `docs/sad.md`, `docs/evidencias/` no lugar

Reprova sozinho, se aparecer: ficção como fato; chave sem corretora; antes copiado do roteiro; SAD sem PoC ou PoC sem SAD.

## Bônus

Só com a etapa 11 fechada e pedido explícito. Paralelizar as três consultas em `internal/quotation/service.go`. Tratar como decisão no SAD (contexto, opções, escolha, consequências) porque o p95 muda. A `partner-degrading` passa a ver as três chamadas ao mesmo tempo; dizer isso. Não implementar o bônus no meio das etapas 5 a 7.
