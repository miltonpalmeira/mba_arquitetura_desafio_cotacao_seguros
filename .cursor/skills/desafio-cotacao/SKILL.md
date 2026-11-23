---
name: desafio-cotacao
description: >-
  Conduz o desafio Full Cycle de cotação de seguros (SAD + PoC) em etapas com
  portão de decisão. Use quando o usuário pedir para avançar o desafio, escrever
  o SAD, implementar circuit breaker, cache ou fallback, instrumentar, coletar
  evidências, ou citar docs/sad.md, make reproduce, Prumo Cota ou este repositório.
---

# Desafio de cotação de seguros

Conduz a entrega em etapas. Uma etapa por conversa, salvo pedido explícito de avançar. O documento decide; o código obedece ao documento.

Leia [etapas.md](etapas.md) só da etapa atual e [estado.md](estado.md) no começo de cada turno. Atualize `estado.md` ao fechar a etapa.

## Começo de cada turno

1. Leia `estado.md`. Se a etapa pedida estiver à frente da atual, recuse e diga o que falta.
2. Leia só a seção da etapa atual em `etapas.md`.
3. Não releia o enunciado inteiro se o estado já registrou a leitura. Fonte de verdade continua sendo `README.md`, `docs/guia-sad.md` e `docs/roteiro-cenario-de-falha.md`.

## Regras que não se negociam

- Não altere `cmd/partner-mock/` nem `cmd/loadgen/`. Não mude perfis das parceiras no compose (`PARTNER_SEED`, `PARTNER_FAILURE_RATE`, latências, degradação) na configuração que sustenta a evidência.
- Não reescreva a base. Estenda `internal/partner/client.go`, `internal/quotation/service.go`, `internal/platform/config.go`, `cmd/quotation-api/main.go` e, se o contrato mudar, `internal/quotation/request.go` e `handler.go`. Pacote novo fica em `internal/`.
- Fora de escopo, não implementar: retry com backoff, bulkhead, rate limit, fila, autenticação, Kubernetes, auditoria persistida, UI. Citar no SAD é permitido.
- Prêmio só vem de parceira ou de cache rastreável até a consulta que o originou. Nunca calcular prêmio na plataforma.
- A chave de cache contém a corretora (`X-Tenant-Id`). O corpo do pedido não traz a corretora. Chave sem `tenant_id` reprova sozinho.
- Circuito aberto significa que a parceira não é chamada. Descartar a resposta depois da chamada não conta.
- Timeout do `http.Client` em `internal/partner/client.go` é obrigatório. Declarar se estouro conta como falha do breaker. A `partner-degrading` não devolve erro: ela afunda até 6000 ms. Sem timeout ela não abre o breaker.
- Nenhum CPF, placa ou `quote_id` em atributo de span ou métrica. `tenant_id` pode.
- `make test` permanece verde, inclusive os testes que já vieram. Teste de resiliência é determinístico, no molde de `cmd/partner-mock/feasibility_test.go`: sem `sleep` calibrado na máquina.
- Número do roteiro ou de `docs/smoke-test-factibilidade.md` não entra no SAD como medição sua. A coluna "hoje" sai do `make reproduce` desta máquina.
- Caminho, biblioteca, métrica ou endpoint citado como existente tem de existir. O que ainda não existe fica marcado como **proposta**.
- TTL, limiares do breaker e nomes de métrica são os mesmos no SAD e no código. O hit rate da seção 8 sai do pressuposto de recotação da seção 1 e do TTL da seção 4, não do gráfico da carga padrão (essa carga repete 5 cotações e o hit rate beira 98%).
- Evidência se captura com o ambiente de pé. `make down` zera Jaeger, Prometheus e Redis. Relatório de `make reproduce` vai como texto. Gráfico leva a consulta PromQL e a janela.
- Paralelizar a agregação é bônus. Só depois do aceite obrigatório, e só se o usuário pedir.

## Portão

A etapa 3 trava as decisões. Sem o bloco de decisões preenchido em `estado.md` e confirmado pelo usuário, não se escreve `docs/sad.md` e não se mexe em código de solução.

Em toda etapa com escolha, apresente opções com consequência (latência, dinheiro, LGPD/SUSEP, o que a corretora vê). Recomende uma. Pare e espere. Não escolha no lugar do usuário.

Ao fechar uma etapa, responda só com:

- o que ficou feito e onde está o artefato;
- a decisão travada, se houve;
- o que ainda não se faz;
- a pergunta única para avançar.

## Como o usuário chama

- "etapa 0" … "etapa 11", ou "avançar" para a próxima do `estado.md`.
- "revisar aceite" na etapa 11.
- "bônus" somente com a etapa 11 fechada.
