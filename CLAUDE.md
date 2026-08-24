# smarthome-hub

## O produto

Hub de automação residencial modular e self-hosted, rodando em hardware do
próprio usuário, sem conta obrigatória na nuvem.

A premissa que tudo o mais serve: **os dispositivos continuam funcionando quando
o hub cai, e o hub continua funcionando quando a internet cai.** A nuvem é
melhoria, nunca dependência. Qualquer proposta que transforme a nuvem em caminho
crítico contradiz o produto.

Repositório **público por design** (princípio de Kerckhoffs).

## Stack e restrições

| Item | Decisão |
|---|---|
| Linguagem / runtime | Go 1.26 — binário estático único, ~15 MB, sem runtime a instalar |
| Persistência | TimescaleDB (série temporal de telemetria) |
| Mensageria | MQTT sobre TLS, credencial + ACL **por dispositivo** |
| Bus interno | `bus.InProcess` — síncrono hoje, preparado para NATS |
| Alvo de deploy | **Raspberry Pi numa casa** |
| Restrição dura | Memória e CPU do Pi; sobreviver a queda de energia e Wi-Fi instável |
| Clientes | Web + celular (REST/WS) |

O alvo Raspberry Pi é o que descarta opções antes da discussão começar — foi ele
que eliminou .NET e JVM no ADR-0001.

## Arquitetura em uma tela

```
ESP32/sensores ──MQTT/TLS──▶ Broker ──▶ Ingestion
                                            │
                                    ╔═══════▼═══════╗
                                    ║   EVENT BUS   ║
                                    ╚══╤═══╤═════╤══╝
                              Device ──┘   │     └── Command
                              Registry  Telemetry   Dispatcher
                              +Shadow   & Metrics   (arbitração,
                                            │        ACK, auditoria)
                                       TimescaleDB
                                            │
                                   MODULES (irrigation, lighting) ──REST/WS──▶ clientes
```

**Regra de dependência:** `platform` não conhece ninguém. `core` conhece
`platform` e `contracts`. `modules` conhecem `platform`, `contracts` e
*interfaces* de core — **nunca outro módulo**. Toda comunicação entre módulos
passa pelo event bus.

Isso é imposto pelo **compilador**: o domínio de cada módulo vive sob um
`internal/` aninhado (`internal/modules/irrigation/internal/...`), que o Go
proíbe outros módulos de importar. Violar fronteira é erro de build, não
observação de code review.

### Hierarquia de autoridade de controle

```
Nível 3  USUÁRIO (web/mobile)  → expressa intenção, com prazo
Nível 2  HUB (rule engine)     → automação entre dispositivos, agendamento
Nível 1  DISPOSITIVO (firmware)→ autonomia local, funciona com o hub fora do ar
Nível 0  FAILSAFE (hw/firmware)→ watchdog. Sobrepõe todos. Sempre.
```

Níveis altos expressam **intenção**; níveis baixos garantem **segurança**. Um
comando do usuário pode suspender a automação — nada suspende o failsafe.

Controle manual tem duas naturezas, **nunca confundidas**:

- **Pulse** — ação pontual (`regar 30s agora`). A automação continua ativa.
- **Mode change** — suspende a automação por um TTL explícito e **volta sozinho**
  ao automático.

## Decisões já tomadas (não reabrir sem fato novo)

| ADR | Decisão | Status |
|---|---|---|
| 0001 | Monolito modular event-driven em Go | Accepted |
| 0002 | MQTT como protocolo de dispositivo | Accepted |
| 0003 | PostgreSQL + TimescaleDB como store único | Accepted |
| 0004 | Topologia híbrida hub-first, nuvem como módulo futuro | Accepted |
| 0006 | Hierarquia de autoridade de controle e arbitração auto/manual | Accepted |
| 0005, 0007 | Simuladores como estratégia de dispositivo; postura open-source | 🚧 a escrever |

Negativos do ADR-0001 **aceitos conscientemente** — não são bugs a reportar:
sem isolamento de falha entre módulos, escala acoplada, deploy acoplado,
lock-in em Go, e entrega síncrona de eventos sem durabilidade (aceitável para
telemetria, **deliberadamente não** para comandos, que têm caminho durável
próprio).

**Revisitar o ADR-0001 quando:** passar de 200 dispositivos por hub, ingestão
passar de 1000 msg/s, o perfil de falha ou escala de um módulo divergir
materialmente do resto, ou mais de uma pessoa passar a trabalhar no código.

## Backlog

| Ticket | Escopo | Status |
|---|---|---|
| SH-001 | Fundação: ADRs, esqueleto modular, ambiente local | 🚧 |
| SH-002 | Device Registry + Shadow (desired/reported/mode) | ⏳ |
| SH-003 | Pipeline de ingestão de telemetria | ⏳ |
| SH-004 | Command Dispatcher com ACK e arbitração | ⏳ |
| SH-005 | Camada de failsafe / watchdog | ⏳ |
| SH-006 | Módulo de irrigação (primeira fatia vertical) | ⏳ |
| SH-007 | **TLS no broker MQTT** — listener 8883 com CA local, parar de publicar 1883 | ⏳ 🔒 |

**Ticket atual: SH-001.** Critério de aceite verificável por `make smoke`.

**SH-007 não obedece à ordem numérica.** O gatilho dele não é uma data nem uma
métrica, é um evento: **antes do primeiro dispositivo físico entrar na LAN.**
Hoje o broker roda listener em texto plano em 1883 em todas as interfaces, o que
significa que a credencial MQTT viaja em claro no CONNECT — presença passiva na
Wi-Fi de casa basta para capturá-la e publicar comando em qualquer dispositivo.
É aceito conscientemente **só enquanto o único publisher for simulador em
loopback** (dívida registrada no ADR-0002 com o gatilho de reversão).

Consequência prática de ordenação: SH-007 vem **antes** de SH-002 provisionar
dispositivo real, e obrigatoriamente antes de SH-005 e SH-006, que envolvem
hardware. Controle de replay do ADR-0006 §4 não vale nada enquanto o canal for
esnifável — TLS primeiro.

## Comandos

```bash
make setup   # gera .env com segredos novos + instala git hooks
make up      # sobe broker MQTT e TimescaleDB, aguarda healthy
make run     # roda o hub com .env carregado
make test    # testes com race detector
make lint    # formatação e análise estática
make sec     # varredura de segredos e vulnerabilidades conhecidas
make smoke   # verifica o critério de aceite do SH-001
```

Rode `make test` e `make lint` antes de dar qualquer trabalho por concluído.

## Convenções deste repositório

- Todo artefato do repo em **inglês**: código, comentários, ADRs, README, commits.
- Módulo novo expõe **apenas** `module.go` como entrada pública; o domínio vai
  para `internal/` aninhado.
- Decisão arquitetural aceita vira ADR em `docs/adr/`, seguindo
  `0000-template.md` — incluindo a seção **"Revisit when"**, que é o que
  transforma o ADR num documento vivo.
- Pre-commit roda `gitleaks`. Se falhar, o problema é o commit, não o hook.

## Segurança específica

- **Telemetria doméstica é dado de presença.** Histórico de irrigação revela
  exatamente quando a casa ficou vazia. Isso pesa mais do que parece nas decisões
  de retenção, exportação e exposição de API.
- **Nunca comitar:** credencial, certificado, device ID real, dados de Wi-Fi,
  telemetria real. O repositório é público.
- Sem acesso anônimo ao broker. Credencial e ACL **por dispositivo** — credencial
  compartilhada entre dispositivos é achado crítico.
- Superfícies a vigiar: spoofing de dispositivo, replay de comando, wildcard
  permissivo demais em tópico MQTT, e comando sem ACK autenticado.
