<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Contexto detalhado de continuidade

Preparado em 2026-10-13. Repositório: https://github.com/oaovito/MNE_Lab.
Ponte para alternância de agentes; não distribuir com o produto. Leia primeiro
MASTER_SPECIFICATION.md inteiro e USER_DIRECTIVES.md. Revise o código e as
evidências antes de alterar a implementação.

## Estado, bases e autoridade

- Branch de desenvolvimento: `codex/mne-lab-checkpoint-20261008`.
- Último código funcional com CI completo: `7985a57ff835699f2643687b2a80bbceb9b5e779`.
- Checkpoint documental anterior: `4b6fd314d0d0d809f2911720ce7e0e67162c1dff`.
- `main` observada em `9c5a06f3122a89a4e47d707a77043b1a8da3d53d`; permanece protegida.
- Esta mudança adiciona ponte e controles de distribuição/auditoria, sem alterar
  código da aplicação, dependências ou algoritmos científicos.
- O HEAD desta ponte deve ser obtido pelo Git. CHECKPOINT.json registra bases e
  escopo; VALIDATION.json registra checks executados e sua revisão. Evitar gravar
  hash autorreferente ou atribuir CI de uma base a uma revisão nova.
- `PROJECT_COMPLETE=false`, `STABLE=false`. A ponte pronta é um checkpoint.

O mestre disponível possui 44 seções e não é a transcrição histórica até a
seção 422. A conversa integral, transcrições antigas e um arquivo original com
outro nome não foram recuperados literalmente. A versão de requisitos no Git
preserva o mestre consolidado, minimizando somente identificadores de corpus;
diretrizes posteriores resolvem criação inicial, permissões científicas e
coordenação. Não fabricar história ou afirmar leitura de material ausente.

O histórico antigo de `main` contém corpus privado; a branch pública de
checkpoint tem raiz independente auditada. GitHub compare informou ausência de
ancestral comum. Não houve PR/merge, force-push ou integração na `main`.
O trabalho histórico do Claude, stash e propostas de integração foram preservados
privadamente. Não publicar essas referências nem usá-las sem revisar estado e
escopo de privacidade. A ponte pública não replica esse histórico privado.

## Produto e arquitetura que já existem

Nome visual **MNE Lab**, linha `0.1.0` em desenvolvimento. Núcleo Go, interface
Preact/TypeScript embutida por `web/dist`, sessão local autenticada e janela do
navegador Chromium instalado. Runtime estatístico webR/R local e offline.
Isso não comprova bootstrap em host limpo ou independência completa de browser.

- Accounts/Profiles cifrados, máximo cinco Profiles; vault/store/blobstore,
  backups, recovery, sync queue, journal/lifecycle e updater/LockedBuild.
- File Library com original byte-for-byte, parsed/normalized separados,
  metadata/provenance, soft delete e relações com Graph/Cycle/Statistics.
- Graph/Cycle Libraries, gráficos/distribuições/trends, replicatas, estatística,
  presentation/mobile local, Turbo e localização EN/PT/ES.
- Smart Import review/confirm, worksheet/range selection, recipes versionadas
  cifradas por Profile e receipts HMAC vinculados ao conteúdo e ao escopo.
- Exportação de dados/figuras, pacotes de pesquisa, colisões explícitas e
  publicação concorrente exclusiva/atômica.

Esses pontos descrevem implementação existente, não conformidade completa do
mestre. Help, discovery, acessibilidade, todas as opções de exportação,
providers, clean-machine e assinatura completa ainda precisam de revisão própria.

## Implementações concluídas ou parciais que não devem ser refeitas

| Área | Comportamento atual | Limite |
|---|---|---|
| NanoBrook DLS | Parser `1.4.0`, spec `ls-spec/0.3-provisional`; summaries/distributions e Lognormal/Multimodal separados | Sem validação científica nativa completa |
| Smart Import | Inspect/preview somente leitura, seleção explícita, parse completo ao confirmar, receipts scoped | Não tratar preview truncado como fonte integral |
| CSV/TSV mapping | Cinco escalares DLS com units declaradas e receita/revisão | PARTIAL / UNVALIDATED; sem inferência/conversão |
| XLSX mapping | Sheet/range/coluna/linha/token bruto e decimal choice preservados; schema/profile versionados | Sem execução de formulas/macros ou distribuição genérica |
| ODS 1.3 | Preview literal readonly limitado; display/source value separados | Sem scientific import/mapping/receipt |
| XLS | Trabalho pendente | Reader bounded e contrato/referência necessários |
| Statistical Analysis | One-Way, Welch, Two-Way completo, RM clássico balanceado, Tukey/simple effects, Dunnett restrito, Mixed Gaussian ML random intercept restrito, CIs documentados | Outros desenhos/métodos continuam não suportados |
| Statistical snapshots | Experimental Unit ID, fonte versionada, Source Changed e recálculo novo | Não sobrescrever análise histórica |
| DTS | CFB/envelopes/metadata, preservação e File Library/provenance | Não normaliza quantities/arrays científicos desconhecidos |
| Research package | `/2` cobre README; `/1` informa gap, verifier bounded ZIP/index/CRC/path | `Authenticated=false`; UI/package signing pendentes |
| CMS | Backend SignExport/Verify RSA/ECDSA SHA2 e painel global detached Verify readonly | Sem produto PFX, PAdES, revogação ou timestamp confiável |
| Producer origin | Unknown ou declaração `instrument_export`/`user_created_table`; independente do transporte/parser | Declaração não verificada, não autentica fabricante |

Producer origin tem campos opcionais manufacturer/model/software/build/reference
note e UTC da declaração, não da aquisição. Scope Account/Profile, bounds e
guards de Trash preservam original/hash/status científico. Graph/Cycle/research/
JSON/TXT/XLSX/Statistics e metadata DTS transportam provenance. CSV/TSV continuam
tabelas de valores. Alteração de fonte invalida análise/review atual, sem reescrever
resultados antigos.

Consulte [STATISTICAL_ANALYSIS.md](../../docs/STATISTICAL_ANALYSIS.md) para desenhos,
df, tolerâncias, effects, intervals, diagnósticos e pós-hoc suportados. Dunnett
é two-sided independente One-Way com controle explícito e até oito tratamentos;
Mixed é ML Gaussian random-intercept restrito. CI eta² populacional cobre um
fator fixo clássico independente; CI Mixed é individual aproximado, não
cobertura empírica garantida, simultâneo ou variance-component interval.

## Matriz científica obrigatória

| FEATURE | IMPLEMENTED | TESTED | SCIENTIFICALLY VALIDATED | BLOCKER |
|---|---|---|---|---|
| STATISTICAL ANALYSIS | Sim no escopo documentado | Engine real, fixtures/oráculos independentes, persistência/UI offline | IMPLEMENTED / TESTED / VALIDATED somente nos desenhos e tolerâncias documentados | Métodos/desenhos/intervalos adicionais requerem implementação e referência próprias |
| DLS / LIGHTSCATTERING | Sim, parcial | Synthetic tests e comparação literal de 54 relatórios privados | PARTIAL / PROVISIONAL | Build/configuração real, field/unit/acquisition/G-C/métodos e resultados oficiais pareados |
| Generic CSV/TSV/XLSX mapping | PARTIAL | Bounds, scopes, recipes/receipts, Graph/Cycle/Statistics/export/UI | UNVALIDATED | Interpretação instrumental, distribuição/time mapping e evidência específica |
| DTS | PARTIAL container/envelopes/metadata | Estrutura independente, limites/malformed/fuzz/import/export | UNVALIDATED; quantities científicas desconhecidas | Serialização/versão/record role e exports oficiais pareados |
| ZETA | IMPLEMENTATION ALLOWED | Estrutura do workbook privado revisada | SCIENTIFIC VALIDATION PENDING | Docs específicas potential/mobility/quality/units/settings/model e summaries/distributions pareados |
| NTA | ARCHITECTURE / PREPARATION ALLOWED | Sem fixture NTA real suficiente | SCIENTIFIC VALIDATION PENDING | NanoSight schema/build/settings, exports/resultados confiáveis; trajectories/timebase/calibration/algorithm se tracking for requerido |
| CMS/checksums/producer declaration | PARTIAL ou implementado no escopo | Testes de software/interoperabilidade | Não validam ciência | Assinatura ou declaração não substituem referência científica |

Blockers exatos de ZETA/NTA:
[TECHNICAL_CONTEXT.md](../../docs/TECHNICAL_CONTEXT.md), seção Remaining evidence.
Workbook e DTS ficam privados. Malvern “Zetasizer Nano Analysis Methods Explained”
é referência DLS/size, não potencial Zeta. Não inventar unidades/métodos/fields.
VALIDATED exige documentação adequada + resultado reproduzível + referência
confiável, com tolerâncias; abrir arquivo ou desenhar gráfico não satisfaz isso.

## Regressões já corrigidas

1. Restart/transação: replacement assume após handoff confirmado; filho falho é
   encerrado/coletado antes de rollback de locks/store/preferences/window.
2. Recovery/sync: gerações antigas e dados do Account fechado não são descartados;
   falta de remote confirmation nunca autoriza apagar a única cópia válida.
3. Current/Average Count Rate são quantities separadas; raw tokens/units mantidos.
   Não inferir photon-to-particle conversion.
4. Timestamp raw/zone/ambiguity/confirmation/source, offsets/frações preservados;
   wall clock sem zone não recebe offset inventado.
5. Cycle preview/save/Plan deduplicam IDs sem modificar a lista do chamador.
6. Account.ID usa proteção da troca de passphrase; não reintroduzir race.
7. Receipt liga todos os choices a content/hash/Account/Profile/parser/spec/expiry;
   recipe selection canônica não depende da ordem de campos JSON.
8. XLSX valida sheets excluídas, shared/repeated strings e payload expandido;
   subset/preview não pode desviar os bounds.
9. Build frontend/runtime/package image antes de Go/frontend worker tests;
   não depender de outputs gerados por checkpoint anterior.
10. Keygen exclusivo sem symlink/overwrite; export keep-both/ask nunca substitui
    saída concorrente por fallback inseguro.
11. Package `/2` cobre README; `/1` preserva coverage gap explícito.
12. CMS rejeita signer Public typed-nil/unavailable e autentica signed attributes
    antes de concluir content mismatch. Trust/time/digest são estados distintos.
13. Menus E2E usam texto acessível independente do badge New; não índice posicional.
14. Producer-origin não reescreve originals/science ou snapshots antigos.
15. Gofmt de uma linha causou CI inicial; successor `7985a57` corrigiu. Shell sem
    `set -e` havia mascarado um status. Não desligar assertion para fabricar verde.

## Evidências existentes e o que não demonstram

CI funcional exato: [run 38091538654](https://github.com/oaovito/MNE_Lab/actions/runs/38091538654),
commit `7985a57`, jobs check/macos/windows-export/e2e aprovados.

- Go: 249 testes principais, 554 casos/subcasos, 27 pacotes, quatro skips;
  race e vet. Frontend: 52 testes, typecheck/build.
- Browser local: 26 reports, 699 passes/0 fails; producer-origin específico 42/0.
  Fonte local funcional em `d08f11b`; `7985a57` alterou apenas alinhamento gofmt,
  e hosted CI executou o successor exato.
- Builds: Windows amd64/386/arm64, Linux amd64/arm64; macOS em runner nativo.
- Literal DLS privado: 54 reports, 216 quantities/candidates, 1.080 literal
  summary checks, 5.616 distribution points/4.104 equality checks. Isso é
  extração observada, não validação de algoritmo instrumental.
- Scratch Linux herdado 112/245/3 skips e seis empty-startup Verify checks têm
  escopo anterior; não certificam GUI/USB/clean OS da revisão atual.
- Dependency audit: zero called/imported Go vulnerabilities no escopo registrado;
  advisory legacy openpgp não chamado conhecido. Não transferir resultado a
  upgrades ou novas dependências sem revalidar.
- Clones isolados da branch pública foram auditados com todos os objetos
  alcançáveis e fingerprints privados. Checkout com refs antigas pode falhar na
  auditoria `--all`: preservar os refs e auditar clone isolado, sem desativar gate.

Os logs privados e arquivos reais não estão no Git. Evidências independentes
públicas, scripts/oráculos, fixture manifests e testes estão nas áreas indicadas
por DEVELOPMENT_STATUS. Os checks novos desta ponte são registrados separadamente
em VALIDATION.json; não assumir nova execução das 699 verificações locais.

## Próxima ação de engenharia: PFX/P12 bounded

Ler [CMS_SIGNATURES.md](../../docs/CMS_SIGNATURES.md) e revisar
`internal/signature/cms.go`, `internal/app/signature.go`, export API/UI e testes.
O candidato `software.sslmate.com/src/go-pkcs12 v0.7.3`, BSD 3-clause, permanece
preparação: não está adicionado como dependência de produção.

Findings privados registrados, suficientes para retomar sem arquivos de corpus:

- Normal modern PFX Encode/DecodeChain com chave/certificado/senha efêmeros em RAM
  funcionou. Probe aumentou MAC Iterations para `2147483647`; filho nativo precisou
  ser terminado pelo timeout externo de dois segundos (2.001 s observados).
- DecodeChain bruto não expõe global work cap. Nunca chamá-lo diretamente em HTTP
  para uploads de credenciais. Timeout/cancel externo observado não é implementação
  de isolamento de produção ou hard memory/CPU bound.
- PBMAC1 usa iterations em Algorithm.Parameters e ignora outer MacData.Iterations.
  PBES2 tem iterations próprias; cobrir nested/shrouded key bags, não só MAC externo.
- PBMAC1 já limita KeyLength de 20 a 64 nessa versão; não alegar alocação arbitrária
  inexistente. Cipher key sizes selecionadas por algoritmo fixo.
- `GOMEMLIMIT` é soft; não denominá-lo limite rígido de SO.
- Fonte examinada pinada: pkcs12.go SHA256
  `fcebcb30eba1b9c89a4cd455a55bf4cf0a67056065502271f9e0f58988497f46`,
  mac.go `0a7f0693212da1aca068acfb69e22299b4ff185088d4caf1a998bccc6b3c3702`,
  crypto.go `692f768d11d541e54a6844ff291dc5dfd5cc0d6bad7364d4b6888fd1876cec7a`,
  pbkdf.go `2dd5393857f50f3470821e3e2a2aa64f1dbd9edc51c55dd1dfc5f057aa2e1e11`.

Retomada: revisar fonte pinada, reproduzir probe com credenciais sintéticas/RAM,
escolher preflight completo de custo ou worker nativo isolado com lifetime,
cancelamento, limites de credential/content/frame/I/O e CPU/memória aplicáveis.
Testar spoofed worker/cleanup, wrong password, rejeições, timeout e isolamento.
Nunca enviar secrets em args, logs, paths, Profile store, sync, backups, recovery
ou research package. Só após checks, integrar identidade legítima/local e
preferências não sensíveis por Profile, None/Visible/Digital/Complete. Overlay
antes de finalizar bytes/hash/sign/verify. PDF/PAdES/revogação/timestamp exigem
protocolos e interop próprios. Release Ed25519 é outra identidade.

## Trabalhos independentes e intervenções do dono

TASKS.json separa agent work de owner/evidence blockers. Engenharia ainda inclui
XLS bounded, ODS científico e distribution mapping com contratos/referências,
pesquisa DLS/versioning, bootstrap/browser/runtime/offline USB, clean-machine,
Corresponding Source/rebuild/licenças e providers oficiais.

O dono precisa fornecer docs/exports pareados de DLS/ZETA/NTA/DTS, configuração/
build instalado, testes GUI/USB reais, registrations Google/Microsoft/Apple,
identidade/licença/release e escopo de integração/remediação do histórico.
Fontes oficiais/referências e materiais ausentes estão em MATERIALS.md;
não pedir ao usuário tarefas técnicas que já sejam executáveis pelo agente.

Windows owner draft permanece `8a12ff6`, anterior a CMS/producer-origin. Upload/
download comparados, sem GUI/USB real confirmado. Não disparar novamente o
workflow fixado ou sobrescrever assets como se incluíssem esta ponte.

## Setup, checks e fronteira de produto

Versões testadas: Go 1.26.9, Node 22.23.3, Git 2.52.0 e Chromium. O checker de
distribuição usa Python 3.10+ e Git com suporte a `check-attr --source`.
Adapte paths/caches e
plataforma; upgrades deliberados exigem revalidação. No cloud original, ativação
em `/workspace/toolchains/mnelab-env.sh`; um clone em outra máquina não recebe
esse script ou configuração automaticamente. Instalação/registro do cloud salvo
não demonstra sua publicação ou validação em nova instância.

```sh
set -euo pipefail
(cd web && npm ci && npm run typecheck && npm run build && npm test)
test -z "$(gofmt -l cmd internal web)"
go vet ./...
go test -race ./...
scripts/build.sh
scripts/e2e.sh
```

Esses são comandos de retomada, não novos resultados desta etapa. Não reconstruir
web/dist durante Go/build/harness. Mac/tray exige cgo e Mac nativo; PowerShell não
interpreta Bash. Python/OpenSSL são ferramentas de desenvolvimento, não runtimes
obrigatórios do usuário.

Fronteira: raiz CLAUDE/AGENTS e esta pasta são export-ignored; frontend compila
entradas explícitas e Go embute somente dist. Executar checker e regressões após
alterar coordenação ou distribuição:

```sh
python3 scripts/check_development_boundary.py --repo . --artifacts out web/dist
git archive --format=zip HEAD | python3 scripts/check_development_boundary.py --archive-stdin
scripts/audit_test.sh
```

Antes de publicação, auditar tree/index/history/artifacts e todos os objetos do
**clone isolado** da branch, com histórico completo. O audit não deve suprimir
segredos/corpus ou metadados: permissão de contexto é estreita e somente no Git;
arquivos de produto/ZIP aninhado continuam rejeitados. Source ZIP/Download ZIP
omitem a ponte; alternância usa clone. Atualize provas e próximo passo sem copiar
conteúdo para UI, assets ou fixtures de produto.

## Histórico público e decisões

A cronologia completa de commits até a base está em CHECKPOINT.json. Destaques:
restart/count-rate/timezones/Cycle fixes; import review/recipes; ANOVA/DTS;
Dunnett/Mixed/effect intervals; DLS literal evidence; ODS/CSV/XLSX partial mapping;
keygen e export concorrentes; runtime build order; package/2 integrity; CMS/global
Verify; R source acquisition; producer-origin declarations; documentação atual.

R 4.6.0 foi adquirido de dois mirrors oficiais HTTPS com bytes idênticos e VERSION/
COPYING/path checks. Isso não prova checksum upstream independente, todas as
fontes/toolchain ligadas, rebuild ou complete Corresponding Source. Não publicar
Stable por ter compilado ou escolhido uma chave/licença.

Ao passar trabalho, registrar o revisado, divergências, mudanças, checks com
SHA/escopo, pendências, intervention list e próxima ação. Nenhum scheduler/objetivo
persistente nativo foi encontrado e testado. Um JSON não reinicia agentes após
quota. Sem API OpenAI paga; continue trabalho sustentado e preserve checkpoint.
