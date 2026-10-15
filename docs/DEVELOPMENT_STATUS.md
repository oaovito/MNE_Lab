# Estado de desenvolvimento do MNE Lab

Atualizado em 12 de outubro de 2026. Versão de desenvolvimento: `0.1.0`;
schema de dados: `1`. **A aplicação ainda não é uma versão Stable completa.**

Este documento descreve o código funcional publicado em
`7985a57ff835699f2643687b2a80bbceb9b5e779`, na branch
`desenvolvimento`. A atualização seguinte é documental;
não altera o comportamento da aplicação. As últimas provas funcionais citadas
abaixo correspondem à revisão `7985a57`, e não a testes novos executados por
esta atualização documental.

## Leitura de referência

| Documento | Conteúdo que deve ser consultado antes de alterar a área |
|---|---|
| [TECHNICAL_CONTEXT.md](TECHNICAL_CONTEXT.md) | Arquitetura, chaves, armazenamento, lifecycle, módulos e matriz de capacidade |
| [READINESS.md](READINESS.md) | Trabalho restante de desenvolvimento e intervenções do proprietário |
| [STATISTICAL_ANALYSIS.md](STATISTICAL_ANALYSIS.md) | Desenhos suportados, convenções estatísticas, versões do engine, oráculos e tolerâncias |
| [SCIENTIFIC_RULES.md](SCIENTIFIC_RULES.md) | Units, métodos, count rates, tempo e limites científicos |
| [DLS_VALIDATION.md](DLS_VALIDATION.md) | Comparação literal de relatórios e evidências científicas ainda ausentes |
| [LIGHTSCATTERING_SOURCE_REGISTRY.md](LIGHTSCATTERING_SOURCE_REGISTRY.md) | Fontes documentais e identificação das versões consultadas |
| [LIGHTSCATTERING_WEB_RESEARCH.md](LIGHTSCATTERING_WEB_RESEARCH.md) | Pesquisa externa, claims, conflitos e decisões |
| [LIGHTSCATTERING_FORMAT_MATRIX.md](LIGHTSCATTERING_FORMAT_MATRIX.md) | Formato nativo versus genérico, conteúdo observado e suporte efetivo |
| [IMPORT_REVIEW.md](IMPORT_REVIEW.md) | Inspeção, receipts, recipes, worksheet/range e mapeamentos parciais |
| [FILE_PRODUCER_ORIGIN.md](FILE_PRODUCER_ORIGIN.md) | Origem declarada do arquivo, distinta de transporte e reconhecimento |
| [DTS_FORMAT.md](DTS_FORMAT.md) | Interoperabilidade estrutural e lacunas de semântica científica DTS |
| [PACKAGE_INTEGRITY.md](PACKAGE_INTEGRITY.md) | Contrato de checksum do pacote de pesquisa e compatibilidade de versões |
| [CMS_SIGNATURES.md](CMS_SIGNATURES.md) | Assinatura/verificação CMS detached e integração ainda pendente |
| [STATISTICAL_RUNTIME_LICENSES.md](STATISTICAL_RUNTIME_LICENSES.md), [STATISTICAL_PACKAGES_LICENSES.md](STATISTICAL_PACKAGES_LICENSES.md), [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) | Runtime estatístico, dependências, licenças e limites de distribuição |

Os documentos de fabricante contextualizam equipamentos e versões; não
autenticam a origem de um exemplar nem validam automaticamente um parser.
Dados experimentais e referências fornecidas pelo proprietário permanecem
fora do checkout público, dos fixtures públicos e dos artefatos distribuídos.

## Estado funcional

### Aplicação, armazenamento e execução

O aplicativo é um executável Go com interface Preact/TypeScript embutida.
A janela utiliza um navegador Chromium já instalado, com perfil próprio.
O runtime estatístico é embutido; o usuário não precisa instalar R, Python,
Node, Excel ou OpenSSL. O navegador e serviços normais do sistema operacional
continuam necessários.

Accounts e até cinco Profiles por Account possuem armazenamento criptografado,
passphrase, recovery key, isolamento de chaves e avatares. Os modos de storage
são USB + Cloud, USB Only e Cloud Only. Os modos de execução são Portable USB
e Temporary Machine. Há snapshots verificados, restauração, trabalho offline,
fila de sync, conflitos explícitos, recuperação e Save, Clean & Exit.

O restart usa handoff confirmado e limitado por pipes. A sessão e a janela
originais permanecem disponíveis até a substituta confirmar a restauração do
perfil e o startup. Falhas interrompem e coletam o filho antes do rollback.
Trabalho não confirmado remotamente fica preservado de forma criptografada.
Uma cópia no diretório do cliente de nuvem não comprova upload ao serviço e
não autoriza apagar a única cópia ou recovery.

Os adapters Google Drive, iCloud e OneDrive possuem infraestrutura e testes
locais; registrations oficiais e testes contra os serviços reais estão
pendentes. O updater verifica manifests Ed25519 e hashes, respeita compatibilidade
de schema, LockedBuild e rollback. A identidade oficial e o canal Stable
ainda não foram publicados.

A interface tem português do Brasil, inglês e espanhol, temas claro/escuro,
Turbo, descoberta de recursos, Help, command palette, onboarding e Presentation.
A paleta do proprietário (#F4F8FA base, #2C7FB8 destaque, #143A52 destaque
escuro) vale desde 2026-10-15: tema claro padrão com as três cores exatas e
tema escuro derivado; detalhes em TECHNICAL_CONTEXT, seção Design.
O acesso móvel usa pairing QR e comunicação cifrada/autenticada para viewer e
controle de apresentação na rede local.

### Dados, gráficos, Cycles e exportação

File Library preserva o original e seu SHA-256. O parser e a especificação,
raw tokens, units presentes, physical source lines, sheet/range e recipes
acompanham os resultados. Ausência, ambiguidade e advertências são explícitas.
Não há conversão automática de units nem inferência de réplicas independentes
a partir de nomes, arquivos, bins ou ordem de importação.

Relatórios DLS com alternativas Lognormal e Multimodal mantêm cada método e
representação separadamente. Uma seleção confirmada é fixada no gráfico salvo.
G(d) mantém valores originais sem assumir percentagem; C(d) permanece auxiliar
não mapeado. Current Count Rate e Average Count Rate são campos distintos.

Graph Engine produz distribuição, comparação, parâmetros no tempo e estudos
por Cycle. A mesma display list serve a preview, SVG, PDF e raster. Cycles
preservam plano, associação temporal, revisão e estatísticas descritivas;
datas/timezones não são completadas por suposição.

Smart Export já produz PNG, SVG, PDF, TIFF, JPEG, WebP, CSV, TSV, TXT, JSON,
XLSX, originais e pacotes ZIP de pesquisa. Resultados e provenance incluem
snapshots; tabelas secundárias de uma análise não são descartadas. Publicação
de arquivos é exclusiva em políticas que não autorizam replacement. O pacote
`mnelab-package/2` cobre também README; o verificador informa a lacuna de README
em `/1`. Esse contrato de checksum não autentica autor nem identidade.

`producerOrigin` pode ser desconhecido ou declaração explícita de exportação
de instrumento/tabela criada pelo usuário. Manufacturer/model/software/build
são texto declarado, sem verificação automática. A informação propaga-se para
Graph, Cycle, Statistics e provenance de exportação. A mudança invalida reviews
obsoletos sem reescrever resultados históricos. Parser, extensão ou labels
familiares não provam quem criou o arquivo.

### Statistical Analysis

Há análise revisada e isolada por Profile, definição dos fatores e unidades
experimentais, snapshots imutáveis, source-change detection, revisões de
resultado, gráficos estatísticos e exportação completa. O engine offline usa
webR `0.6.0` / R `4.6.0`, worker descartável e pacotes pinados, com cancelamento
e timeout. Nenhum download de pacote ocorre no produto.

O escopo validado inclui One-Way, Welch, Two-Way com células completas,
repeated measures clássico completo/balanceado, Tukey e simple effects
condicionais, Dunnett two-sided independente restrito e Mixed Gaussian ML
random intercept restrito. Inclui CI populacional eta squared em um fator
independente fixo e CI individual aproximado dos coeficientes Mixed. Desenhos,
pressupostos, df, tolerâncias, limites e métodos não suportados estão em
[STATISTICAL_ANALYSIS.md](STATISTICAL_ANALYSIS.md). Não há seleção automática
de método a partir de diagnósticos nem substituição silenciosa de um modelo
não suportado. Variáveis ZETA/NTA não ganham aceitação científica por compartilhar
essa infraestrutura.

## Matriz de implementação e evidência

“Tested” descreve testes de software ou comparações específicas. Validação
científica exige documentação adequada, resultado reproduzível e comparação
com referência confiável, com limites e tolerâncias declarados. Abrir um arquivo,
desenhar um gráfico ou verificar uma assinatura não satisfaz esse critério.

| FEATURE | IMPLEMENTED | TESTED | SCIENTIFICALLY VALIDATED | BLOCKER |
|---|---|---|---|---|
| Accounts/Profiles, storage, backup/recovery e lifecycle | Sim | Testes nativos, race, isolamento e fluxos de navegador | Não se aplica | Testes de desktop limpo e USB real permanecem gates de distribuição |
| File Library, provenance, Graph, Cycle e exportação existentes | Sim | Snapshots, receipts, units, timestamps, renderização e exportação | Depende do adapter e do método; não transfere validação | Cada adapter novo precisa de contrato próprio |
| Producer origin | Sim, declaração não verificada | API scoped, reabertura cifrada, originals/resultados imutáveis e round trips JSON/TXT/XLSX | Não se aplica; não autentica exporter | Evidência independente de autenticidade continua necessária |
| Statistical Analysis clássico e Welch | Sim no escopo documentado | Engine R real versus referências independentes, fixtures sintéticos, persistência e worker offline | Sim, somente desenhos/tolerâncias documentados | Métodos/desenhos adicionais exigem implementação e referência próprias |
| Dunnett two-sided independente One-Way | Sim, control explícito e até oito tratamentos | Referência multivariate-t SciPy refinada, CIs e repetibilidade | Sim, somente escopo documentado | Welch/RM/Mixed/factorial/one-sided Dunnett não suportados |
| Mixed Gaussian ML random intercept | Sim, modelo restrito | Cinco desenhos independentes Statsmodels/SciPy, df e fronteiras | Sim, somente modelo e tolerâncias documentados | Random slopes, covariance structures adicionais, REML e Mixed post-hoc |
| CI eta squared populacional e coeficientes Mixed | Sim, respectivamente um fator fixo e intervalos individuais aproximados | Sete fixtures eta e 80 endpoints Mixed independentes | Sim no escopo e convenções documentados | Outros efeitos/desenhos, intervals simultâneos e variance components |
| NanoBrook DLS report parsing | Sim, parser `1.4.0`, spec `ls-spec/0.3-provisional` | Regressões sintéticas e comparação literal de 54 relatórios privados | PARTIAL / PROVISIONAL | Definições/build/units/settings e G/C/métodos exatos; resultados oficiais pareados |
| CSV/TSV e XLSX scalar mapping | PARTIAL, cinco escalares DLS com declarações explícitas | Full-source review, bounds, receipt/recipe, Profile, Graph/Cycle/Statistics/export | UNVALIDATED, inclusive units declaradas | Distribution/time mapping e evidência científica adequada; não habilitar ZETA/NTA por inferência |
| XLSX worksheet/range review e recipes | Sim para subconjunto documentado | XML independente, posições, seleção, fórmulas/macros rejeitadas, payload bounds | Não é validação científica do conteúdo | Rich/merged/linked workbooks não suportados |
| ODS 1.3 | PARTIAL, preview literal somente leitura | Fixture independente/schema, repetições, posições, malformed/unsafe inputs e fuzzing | Não é cálculo científico | Scientific mapping/import, outras versões e células mais ricas |
| XLS nativo e distribuições genericamente mapeadas | Pendente | Não há prova de reader científico suportado | PENDING | Reader bounded, contratos versionados e referências apropriadas |
| DTS | PARTIAL, CFB/envelopes/metadata e preservação | Inventário estrutural independente, malformed cases, fuzzing e import/export scoped | UNVALIDATED; sem quantities normalizadas | Serialização/semântica/versioning/record identities e exports oficiais pareados |
| ZETA | IMPLEMENTATION ALLOWED; parser científico pendente | Workbook privado revisado estruturalmente | SCIENTIFIC VALIDATION PENDING | Documentação oficial de potential/mobility/quality, units/settings/model, native summaries/distributions e referências pareadas |
| NTA | ARCHITECTURE / PREPARATION ALLOWED | Sem fixture real de resultado NTA | SCIENTIFIC VALIDATION PENDING | NanoSight version/schema/settings, exports e referências; timebase/calibration/trajectories/algoritmo se cálculo tracking for implementado |
| Pesquisa de formatos LIGHTSCATTERING | PARTIAL / VERSION-SPECIFIC | Cinco PDFs oficiais consultados e matriz de conflitos/evidência | Não valida algoritmo nem todos os formatos nativos | Build/configuração real, versões históricas, exemplares genuínos e contratos serializados |
| Integridade de research package | PARTIAL backend `/2`, leitura `/1` | Fixtures independentes, alterações de payload/index/CRC, paths e bounds | Não se aplica; `Authenticated=false` | UI global, signed manifest e integração autenticada dos pacotes |
| CMS detached Sign/Verify | PARTIAL backend e painel global Verify | OpenSSL independente RSA/ECDSA bidirecional; trust/time/content/structure e navegador | Não se aplica | PFX/P12 bounded, identidade/profile, Smart Export, Visible/Complete, PDF/PAdES, revogação e timestamps |
| Cloud providers | Infraestrutura/adapters e folder transport | Servidores locais, offline, sync/conflicts/readback | Não se aplica | Registrations oficiais e validação contra serviços reais |
| Updater, LockedBuild e launcher | Infraestrutura implementada | Manifests/hashes/schema, staging, rollback e chaves de teste | Não se aplica | Identidade oficial e artefatos Stable após gates |
| Distribuição Windows/Linux/macOS | Builds e draft Windows de teste | Cross-builds, CI nativo Windows/macOS; Windows draft upload/download comparados | Não se aplica | GUI/USB real, bootstrap, source reprodutível, licença e signing oficiais |

O workbook Zeta é corpus privado de desenvolvimento. O DTS é material privado
de interoperabilidade. A referência Malvern sobre size analysis é contexto
DLS, não documentação de potencial Zeta. Nenhum deles elimina os blockers
específicos da matriz.

Os guias oficiais consultados incluem Particle Solutions `2.3`, `4.0` e
manual da família NanoBrook com build de software não identificado. O manual
da família documenta `.bak`; a serialização, magic, cobertura de dados e
compatibilidade de versão permanecem desconhecidas. “Excel” nos menus não
estabelece `.xls` versus `.xlsx`. Não extrapolar recursos Omni para toda
configuração 90Plus nem escolhas de exportação PS2.3 para PS4 sem prova.

## Regressões críticas já corrigidas

| Área | Comportamento corrigido e restrição que deve continuar preservada |
|---|---|
| Restart/lifecycle | Replacement só assume após handoff confirmado; filhos falhos são interrompidos/coletados antes de recuperar store/lock e preferências antigas |
| Recovery e sync | Contas fechadas/trocadas e gerações antigas de recovery não são descartadas; folder transport não comprova remote API confirmation |
| Count rate | Current/Average separados, raw/units mantidos; não há photon-to-particle conversion |
| Tempo | Raw date, zone, ambiguity/confirmation, offsets e frações preservados; wall clock sem zone fica sem offset inventado |
| Cycles | IDs repetidos não multiplicam observações no preview/save; lista do chamador permanece intacta |
| Account scope | Leitura de Account.ID usa a mesma proteção da troca de passphrase; nenhum retorno a acesso concorrente inseguro |
| Import review | Recipe/bytes/Account/Profile/parser/spec vinculados ao receipt; confirmar faz parse completo, não do preview truncado |
| Recipe selection | Ordem dos campos JSON não desfaz a seleção revisada |
| XLSX bounds | Worksheets excluídos, shared/repeated strings e bytes expandidos também passam por validação; não há bypass por subset |
| Build/test order | Runtime e package image precisam existir antes dos testes Go/frontend que os utilizam; clean build não depende de arquivos gerados antigos |
| Release key | Criação exclusiva não segue symlink nem substitui chave existente |
| Concurrent export | Keep-both/ask usam publicação exclusiva; não há fallback para rename que substitui saída concorrente |
| Package integrity | README é coberto em `/2`; `/1` continua legível com coverage gap explícito |
| CMS | Public key unavailable/typed-nil rejeitada; assinatura dos atributos autenticada antes de concluir content mismatch; trust/time/digest separados |
| Navegador | Seleção de menu por texto acessível independente do badge opcional evita clique na ação errada |
| Producer origin | Alteração apenas metadata declarada; originals/scientific status e análises históricas não são reescritos |

## Mapa de código para continuidade

| Área | Caminhos principais |
|---|---|
| Executável/janela/modos | `cmd/mnelab`, `cmd/mnelab-launcher`, `internal/app/run.go`, `internal/app/lifecycle.go`, `internal/shell`, `internal/paths`, `internal/instance` |
| Accounts e dados | `internal/vault`, `internal/secure`, `internal/store`, `internal/backup`, `internal/atomicfile`, `internal/logging` |
| Sync/provider/update | `internal/syncer`, `internal/provider`, `internal/update`, `cmd/mnelab-release` |
| APIs/library/import | `internal/app/api.go`, `ls.go`, `library.go`, `import_inspection.go`, `import_profiles.go` |
| Adapters/provenance | `internal/science/ingest/parse.go`, `internal/science/module/registry.go`, `internal/science/model`, `internal/science/reference` |
| DLS e tabelas | `internal/science/lightscattering/nanobrook.go`, `parser.go`, `spreadsheet.go`, `column_mapping.go`, `workbook_mapping.go`, `ods_preview.go` |
| DTS | `internal/science/malvern/dts/container.go`, `preflight.go`, `records.go`; testes `internal/app/dts_test.go` |
| Producer origin | `internal/science/model/producer.go`, `internal/app/producer.go`, `internal/science/graph/producer_test.go`, `internal/export/producer_test.go` |
| Graph/Cycle/render | `internal/science/graph`, `internal/science/cycle`, `internal/science/stats`, `internal/plot` |
| Statistical Analysis | `internal/science/analysis/model.go`, `internal/app/statistics*.go`, `web/src/lib/statistics.ts`, `statistics-engine.R`, `web/src/screens/ls/statistics.tsx` |
| Oráculos estatísticos | `scripts/statistics*_oracle.py`, `web/tests/fixtures/statistics*-golden.json`, `web/tests/statistics.test.mjs` |
| Export/package | `internal/app/exports.go`, `internal/export/data.go`, `writers.go`, `statistics.go`, `package.go`, `verify_package.go`, `publish*.go` |
| CMS e UI de verificação | `internal/signature/cms.go`, `cms_test.go`, `internal/app/signature.go`, `signature_test.go`, `web/src/screens/signature.tsx` |
| Interface e idiomas | `web/src/main.tsx`, `web/src/screens/ls/files.tsx`, `screens/export.tsx`, `lib/types.ts`, `lib/features.ts`, `web/src/i18n`, `internal/i18n` |
| Testes/build/CI | `scripts/build.sh`, `e2e.sh`, `audit.sh`, `audit_test.sh`, `web/tests/e2e`, `.github/workflows/ci.yml` |

## Roadmap de desenvolvimento

1. **Assinatura local bounded:** definir preflight de custo ou worker nativo
   isolado antes de expor PFX/P12. Controlar tamanho, custo, lifetime,
   cancelamento e input/output; secrets só em memória/pipes, sem passwords ou
   chaves em args, logs, paths, store, sync, backup, recovery ou pacotes. Um
   decoder de PKCS12 sem work cap não pode executar diretamente no handler.
2. **Smart Export signing:** ligar identidade/preferences ao Profile sem
   persistir private keys no storage do Profile. Finalizar overlays antes de
   hash/sign; implementar None/Visible/Digital/Complete com comportamento
   explícito. Comparar CMS independente; PDF/PAdES, revogação e timestamps
   requerem contratos e interoperabilidade próprios. Chave Ed25519 de release
   permanece distinta da credencial de assinatura de pesquisa.
3. **Importação adicional:** XLS bounded, scientific ODS e distribution
   mapping, com contratos versionados e referências adequadas. Preservar
   preview literal e recipes existentes; implementar infraestrutura independente
   quando seguro, sem inventar schema nativo, units ou algoritmo.
4. **Pesquisa/validação DLS:** fechar diferenças de versões/configurações,
   documentação G/C/métodos/units e arquivos genuínos pareados. ZETA/NTA
   avançam apenas nas partes sustentadas por suas próprias evidências.
5. **Distribuição:** bootstrap de navegador/runtime, instalação/startup em
   sistemas limpos e USB offline; manter aquisição reproduzível com integridade.
   FAT32/exFAT e execução GUI real não são cobertos por cross-compilation.
6. **Corresponding Source:** completar inputs/fontes ligadas/toolchain e
   reconstrução reproduzível do runtime estatístico, validar licenças e método
   de fornecimento. R `4.6.0` foi obtido de dois mirrors HTTPS oficiais com bytes
   idênticos; isso não prova todo o source necessário ou rebuild independente.
7. **Providers/release:** validar serviços reais após registration segura,
   preparar identidade oficial, artifacts/manifests e somente então avaliar
   os critérios de release Stable.

Esses itens são desenvolvimento; obter certificado ou preencher configuração
não implementa automaticamente a funcionalidade faltante. Falta de evidência
de um módulo não deve impedir progresso independente dos demais.

## Intervenções necessárias do proprietário

| Intervenção | Entrega necessária |
|---|---|
| DLS/NanoBrook | Configuração e build instalados; definições precisas de campos/units/settings/métodos; genuine CSV/Excel, distribution XLS/CSV, archive `.bak`, SOP/layout e relatório pareado |
| ZETA | Documentação específica potential/mobility/quality/model/units e exports nativos com resultados oficiais pareados; para DTS, record identity e distribuições correspondentes |
| NTA | Versão/schema/settings NanoSight, arquivos reais summary/distribution e referência; trajectories/timebase/calibration/algoritmo se cálculo tracking for requerido |
| Teste desktop | Verificação Windows GUI/USB, incluindo cenários offline/clean system; depois macOS/Linux e filesystems representativos |
| Apps cloud | Registration oficial Google/Microsoft/Apple e configuração por canal seguro, seguida de testes reais |
| Release/distribuição | Identidade pública de release e gestão segura da chave privada; assinatura/notarização OS quando exigida; licença e forma autorizada de Corresponding Source |
| Pesquisa externa | Revisar/publicar a configuração de rede pendente quando necessário; respostas 403/challenges não são evidência documental e não devem ser contornadas |
| Integração Git | Decidir escopo de integração e tratamento do histórico antigo que contém material privado; nenhuma alteração direta de `main` está autorizada |

O draft Windows de teste é construído de `8a12ff6`, anterior ao backend CMS e
producer origin. Não é um binário da revisão funcional atual nem Stable.
Upload e download foram comparados, mas GUI/USB no computador do proprietário
ainda não foi confirmado. Não sobrescrever nem republicar esse draft como se
contivesse funcionalidades posteriores.

## Histórico e integração Git

A branch de desenvolvimento tem ancestralidade limpa e independente auditada.
`main` permanece em `9c5a06f3122a89a4e47d707a77043b1a8da3d53d` no checkpoint
descrito. A comparação remota informou ausência de ancestral comum. Portanto,
o fluxo normal de PR direto entre essas revisões não está pronto.

O histórico antigo e as referências preservadas contêm material privado,
embora a árvore pública atual não contenha esse corpus. Um candidato baseado
nesse histórico herda esses objetos. Não publicar referências antigas,
reescrever histórico, alterar `main` diretamente ou enfraquecer a auditoria
para contornar o problema. A integração exige revisão própria e autorização
do escopo de privacidade; continua separada do desenvolvimento funcional.

## Provas existentes e limites

CI da revisão funcional exata:
[run 38091538654](https://github.com/oaovito/MNE_Lab/actions/runs/38091538654).
Os jobs `check`, `macos`, `windows-export` e `e2e` concluíram com sucesso.

- Go: 249 testes principais, 554 casos/subcasos, 27 pacotes, quatro skips
  esperados; race e vet.
- Frontend: 52 testes, typecheck e build.
- Navegador local: 26 relatórios, 699 verificações aprovadas, zero falhas;
  suite producer-origin específica: 42/0. A fonte local é funcionalmente a
  mesma do commit `d08f11b`; `7985a57` alterou apenas alinhamento gofmt, e o CI
  executou o commit final exato.
- Builds: Windows x64/x86/ARM64 e Linux x64/ARM64; macOS Intel/Apple Silicon
  em runner macOS. CI nativo Windows cobre exports/CMS/release-key safety.
- Auditoria da branch isolada e de todos os seus objetos alcançáveis: nenhuma
  correspondência com os arquivos privados originais verificada no checkpoint.

Provas anteriores de runtime sem ferramentas de desenvolvimento, fuzzing,
comparações de parser e OpenSSL têm escopos específicos; não significam uma
certificação geral nem validação de desktop limpo. Não atribuir os números de
um checkpoint anterior a código novo sem execução ou equivalência explícita.
Auditoria de todas as referências do checkout histórico pode encontrar os
originais privados preservados; isso não deve ser mascarado como sucesso.

## Compilação e verificações reproduzíveis

Ambiente testado: **Go 1.26.9**, **Node.js 22.23.3**; `go.mod` exige Go
`1.26.9` e `web/package-lock.json` fixa as dependências. Atualizações de runtime
estatístico/toolchain exigem nova comparação numérica e de integridade.
Os comandos abaixo usam Bash, na raiz do repositório, sem secrets no terminal.

Primeiro, completar o build da interface e do runtime gerado:

```bash
set -euo pipefail
(
  cd web
  npm ci
  npm run typecheck
  npm run build
  npm test
)
```

Depois executar o núcleo. A verificação de formato deve realmente falhar se
houver saída; apenas imprimir `gofmt -l` não testa conformidade:

```bash
format_files=$(gofmt -l cmd internal web)
if [ -n "$format_files" ]; then
  printf '%s\n' "$format_files"
  exit 1
fi
go vet ./...
go test -race ./...
```

Não reconstruir `web/dist` enquanto executam Go tests, builds ou browser
harnesses que embutem esses arquivos. `scripts/build.sh` gera aplicativo e
launcher com `-trimpath`; o default cobre cinco targets Windows/Linux:

```bash
scripts/build.sh
```

macOS exige Mac com cgo/compilador disponível para tray. Após o mesmo build
frontend, no Mac:

```bash
go test ./...
scripts/build.sh darwin/arm64 darwin/amd64
```

A verificação nativa Windows usada pelo CI, em PowerShell ou shell do runner:

```text
go test -count=1 -v ./internal/export ./internal/signature ./cmd/mnelab-release
```

Ela não substitui teste da aplicação gráfica ou do launcher em USB. Build
Windows a partir de Bash usa `scripts/build.sh windows/amd64` após frontend;
resources/ícone/manifest são gerados pelo script.

Para as suites completas de navegador, instalar Chromium de teste ou definir
`MNELAB_CHROMIUM` para uma instalação disponível; executar depois do frontend:

```bash
(
  cd web
  npx playwright-core install chromium
)
scripts/e2e.sh
```

Em runner Linux novo pode ser necessário
`npx playwright-core install --with-deps chromium`. Capturas e relatórios vão
para `out/e2e`. Essa suite cobre idiomas, temas, tamanhos, modos, telefone,
imports, mappings, métodos, units/timezones, estatística, DTS, CMS e producer
origin. Os fixtures públicos são inventados e identificados.

Auditorias de build/dependências, com acesso de build às fontes oficiais:

```bash
scripts/audit_test.sh
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
(
  cd web
  npm audit --omit=dev
)
```

Execute `scripts/audit.sh` em um **clone isolado da branch pública limpa**,
com histórico completo, sem importar outras referências. O script examina
`git rev-list --all`; em um checkout que preserva referências históricas
privadas, a auditoria pode falhar por esses objetos. Preserve essas referências
e o gate. No clone isolado, após construir seus artefatos ou apontar para os
artefatos da revisão correspondente:

```bash
scripts/audit.sh out web/dist
```

O histórico da branch auditada deve estar completo e não shallow; as provas
de privacidade precisam cobrir os objetos alcançáveis dessa branch sem trazer
referências históricas privadas para publicação. Credenciais e dados reais
não devem entrar nos reports ou fixtures públicos.

## Critérios de conclusão

Uma etapa só está concluída quando o contrato e o limite estão documentados,
a implementação passou pelos checks pertinentes, regressões relevantes
permanecem cobertas, provenance e isolamento sobrevivem à persistência/export
e o checkpoint pode ser recuperado com a próxima ação exata registrada.

Um recurso científico só pode elevar seu status depois de documentação
adequada, resultado reproduzível e comparação independente com referência
confiável. Suporte parcial deve continuar visível como PARTIAL/UNVALIDATED;
dados ausentes ou desconhecidos não viram zeros, units ou algoritmos inventados.

Stable exige, além dos checks de código, distribuição/licenciamento e source
reprodutível completos, identidade oficial, artifacts/manifests seguros e
testes dos sistemas/USB prometidos. Recursos científicos liberados precisam
de escopo validado; recursos pendentes não podem ser promovidos por uma release.
Nenhum mecanismo nativo de retomada automática após quota foi verificado;
checkpoint é registro de continuidade, não executor. Não há uso de API paga
para manter desenvolvimento.
