<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Materiais, fontes e limites da continuidade

Este inventário pertence ao desenvolvimento. Ele não inclui os arquivos
experimentais, conversas, secrets, links privados de dados ou PDFs completos.
A autorização atual para contexto no repositório permite preservar requisitos
e decisões necessárias à continuidade; não transforma corpus privado em
conteúdo público ou conteúdo do aplicativo.

## Requisitos fornecidos pelo proprietário

| Fonte | Versão / identificação | Material público disponível aqui | Limite |
|---|---|---|---|
| Mestre consolidado do pacote de implementação atualizado | 44 seções principais; fonte de 55041 bytes / 1505 linhas; SHA-256 original `c43e35335bac42cf2201ac4db71c67aa05b7914ded7fdb3235e55d6f84502578` | [MASTER_SPECIFICATION.md](MASTER_SPECIFICATION.md), com seis substituições de identificadores privados e preâmbulo de autoridade | O hash identifica a fonte original; a cópia minimizada tem outros bytes. Não é a conversa completa até a seção 422 |
| Diretrizes posteriores explícitas | Instruções de continuidade, Git, ciência, limites, Windows e ponte no repo | [USER_DIRECTIVES.md](USER_DIRECTIVES.md) | Prevalecem sobre cláusulas históricas conflitantes; não autorizam publicar dados privados ou alterar `main` |
| Pedido de continuidade/recuperação | Revisão de histórico/estado, preservação e execução autônoma | Síntese operacional nas diretrizes e no contexto de continuidade | Conversa/transcripts originais não foram recuperados integralmente; não inventar decisões |
| Pedido Statistical Analysis | Methods/design/Experimental Unit/provenance/Graph/Cycle/export/validation e clean-machine | Mestre §35, diretrizes e [STATISTICAL_ANALYSIS.md](../../docs/STATISTICAL_ANALYSIS.md) | O comportamento documentado é validado em escopo restrito; o requisito completo inclui tarefas ainda abertas |
| Pedido multi-module | Shared core e separação LIGHTSCATTERING/ZETA/NTA | Mestre §§36–39/43–44, diretrizes e matriz técnica | Compartilhar infraestrutura não compartilha semântica ou validação científica |
| Pedido de DTS | Container/parser/framework/provenance/interoperabilidade e integração segura | Diretrizes e [DTS_FORMAT.md](../../docs/DTS_FORMAT.md) | O exemplar privado e a investigação literal não estão aqui; normalização científica permanece sem contrato suficiente |
| Pedido de pesquisa de formatos LIGHTSCATTERING | Pesquisa externa obrigatória, tiers/evidence/versioning/conflicts/native vs generic | Diretrizes e os três documentos LIGHTSCATTERING indicados abaixo | Pesquisa continua PARTIAL; formato documentado não significa parser nativo validado |

Os pedidos originais foram considerados para a consolidação, sem publicar
transcrições literais ou coordenadas privadas. Se a revisão revelar requisito
complementar não representado, registre a lacuna e consulte a fonte autorizada
quando disponível. Continue trabalho independente cuja definição seja suficiente.

## Documentação do produto e implementação

| Documento | Função / alcance |
|---|---|
| [TECHNICAL_CONTEXT.md](../../docs/TECHNICAL_CONTEXT.md) | Arquitetura, storage/chaves/lifecycle, módulos e matriz científica explícita |
| [DEVELOPMENT_STATUS.md](../../docs/DEVELOPMENT_STATUS.md) | Código funcional de referência, funcionalidades, regressões, mapa de código e commands |
| [READINESS.md](../../docs/READINESS.md) | Pendências de desenvolvimento separadas das intervenções do proprietário |
| [STATISTICAL_ANALYSIS.md](../../docs/STATISTICAL_ANALYSIS.md) | R/webR e methods suportados, numerical oracles, tolerâncias e limites |
| [SCIENTIFIC_RULES.md](../../docs/SCIENTIFIC_RULES.md) | Quantities/units/métodos, count rates, tempo e restrições |
| [DLS_VALIDATION.md](../../docs/DLS_VALIDATION.md) | Extração literal independente do corpus disponível; ciência não promovida |
| [IMPORT_REVIEW.md](../../docs/IMPORT_REVIEW.md) | Recipes/receipts/review e adapters limitados CSV/TSV/XLSX/ODS |
| [FILE_PRODUCER_ORIGIN.md](../../docs/FILE_PRODUCER_ORIGIN.md) | Declaração não verificada distinta de transporte/parser |
| [DTS_FORMAT.md](../../docs/DTS_FORMAT.md) | CFB/envelopes/metadata, preservação e blockers científicos DTS |
| [CMS_SIGNATURES.md](../../docs/CMS_SIGNATURES.md) | Backend detached CMS e Verify global parcial; signing integração pendente |
| [PACKAGE_INTEGRITY.md](../../docs/PACKAGE_INTEGRITY.md) | Checksum package `/2`, leitura `/1`; `Authenticated=false` |
| [LIGHTSCATTERING_SOURCE_REGISTRY.md](../../docs/LIGHTSCATTERING_SOURCE_REGISTRY.md) | IDs/URLs oficiais, versions e recibos de documentos públicos |
| [LIGHTSCATTERING_WEB_RESEARCH.md](../../docs/LIGHTSCATTERING_WEB_RESEARCH.md) | Queries, claims, tiers, confidence, conflicts e decisões |
| [LIGHTSCATTERING_FORMAT_MATRIX.md](../../docs/LIGHTSCATTERING_FORMAT_MATRIX.md) | Formatos documentados versus conteúdo observado versus suporte do aplicativo |
| [STATISTICAL_RUNTIME_LICENSES.md](../../docs/STATISTICAL_RUNTIME_LICENSES.md), [STATISTICAL_PACKAGES_LICENSES.md](../../docs/STATISTICAL_PACKAGES_LICENSES.md) e [THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md) | Fontes/pins/licenças e limites de distribuição; inventário não prova Corresponding Source completo |

Provas funcionais de referência pertencem ao commit
`7985a57ff835699f2643687b2a80bbceb9b5e779` e à
[CI 38091538654](https://github.com/oaovito/MNE_Lab/actions/runs/38091538654).
Updates documentais não executam automaticamente esses testes. Os documentos
devem explicitar revisão/escopo antes de reutilizar números em código novo.

## Referências científicas efetivamente consultadas

Os PDFs fornecidos pelo proprietário representam **sete instâncias de três
documentos únicos** por igualdade de bytes: projeto CURRENT/LATEST, projeto
PREVIOUS e nota técnica Malvern DLS/size. São mantidos externamente e não
redistribuídos aqui. O rótulo CURRENT/LATEST não fornece sozinho data de
publicação, build instrumental ou confirmação independente.

O papel documental está registrado por IDs no source registry:

- `LOCAL-LS-SPEC-CURRENT` e `LOCAL-LS-SPEC-PREVIOUS`: intenção/workflow do
  projeto; não substituem fatos demonstrados do instrumento.
- `LOCAL-MALVERN-DLS-TN101124`: Zetasizer Nano Analysis Methods Explained,
  quatro páginas na cópia fornecida; referência de DLS/size, **não potencial Zeta**.
- `REAL-LS-TXT-54`, `REAL-COMPILED-ZETA-XLSX` e `REAL-MALVERN-DTS`: material
  privado observado; não arquivos públicos ou exemplos incorporados.

Cinco documentos oficiais Brookhaven foram obtidos. Seus URLs públicos e
locators estão no [source registry](../../docs/LIGHTSCATTERING_SOURCE_REGISTRY.md).
Os hashes abaixo identificam os bytes consultados, sem licença automática para
redistribuir PDFs ou alegação de validação científica:

| ID | Versão / escopo | Bytes | SHA-256 |
|---|---|---|---|
| BIC-PS-23-QSG | Particle Solutions 2.3; publication date não estabelecida | 3032843 | `a0ad2d3778f52a631e69e2eaf7efa698ebf8dd24223daaf200803171a684e226` |
| BIC-PS-40-QSG | Particle Solutions 4.0; publication date não estabelecida | 4068180 | `6f862e8388f66f32792b50cc1ea2e820f99fe8cdd7106a265b5b9e0e72d5b3a2` |
| BIC-NB-SERIES-13DC | Família NanoBrook; Rev1.3DC no filename; software build desconhecido | 1972659 | `7312bfb350d5812eadfc243416d6765b28712ac81ef6938b69a1d49a236a9ab3` |
| BIC-DLS-QSG | DLS/SOP workflow; software build desconhecido | 1244186 | `1d25e1a6445fda720b5dacd5c093c88736cbca0a3c458f80081d6023d009297d` |
| BIC-NB-INSTALL | Instalação física; sem schema de dados | 1868364 | `54bc97002bd69d4434b09673c681c8f51d9926ab8eea42aa0fcbf7fb48acd12a` |

PS2.3 e PS4.0 documentam selected-measurement Excel/CSV e archive operations;
“Excel” não define extensão ou schema. O manual da família documenta `.bak`,
mas inclui recursos Omni que podem faltar em outra configuração. Não há exemplar
genuíno `.bak`/SOP/layout aqui. NNLS/CONTIN dessa documentação não resolve
automaticamente Lognormal/Multimodal/G/C do corpus privado.

URLs/snippets/catálogos MAN0485/MAN0486/NanoSight/standards são descoberta ou
referência documental, não prova de que o manual foi obtido e validado. Observar
restrições legais, login e standards pagos. Nenhum link privado do Drive é
publicado nesta área.

## Corpus privado: disponível externamente, não incluído

| Material | Uso autorizado | Evidência disponível / limite |
|---|---|---|
| 54 relatórios TXT DLS reais | Descoberta e comparação literal privada; regressões públicas sintéticas | 216 reported quantities e 216 distribution candidates cross-checked; units/métodos/modelo ainda não cientificamente validados |
| Workbook Zeta compilado, localizador lógico `WORKBOOK_ZETA_PRIVADO` | Desenvolvimento/interoperabilidade tabular privada | Não é export nativo de instrumento comprovado, schema NanoBrook Excel ou validação ZETA |
| Exemplar DTS original privado | Investigação CFB/envelopes/record metadata e interoperabilidade | Não confirma measurement type, scientific arrays/units/time/methods; não usar paths internos para executar/buscar arquivos |
| Originais/manifests/hashes por arquivo e registros de amostras | Integridade e rastreabilidade no ambiente autorizado | Não são publicados no repo, CI, screenshots, fixtures ou artefatos |
| Histórico Git privado preservado e proposta de integração | Recuperação/revisão local sob escopo autorizado | Não publicar refs/stash/proposta ou histórico com corpus; autorização de continuidade não autoriza remediar `main` |

Há 42 relatórios TXT no pacote mestre e doze adicionais preservados externamente;
isso soma os 54 analisados. Não presumir duplicação ou fabricar os arquivos
restantes. Os dois CSV do pacote atualizado são **manifesto e índice de documentos**,
não exports científicos nativos nem fixtures de schema CSV. Seus links de dados
privados também permanecem externos.

## Materiais e evidências ainda ausentes

- Conversa original integral e transcripts/checkpoint original completos do
  desenvolvimento anterior. A história comprovada foi preservada; ausência de
  transcripts não bloqueia revisão e desenvolvimento independente.
- Build/configuração instrumental exata, native summary/distribution exports
  CSV/Excel/XLS e archive/SOP/layout pareados com referência confiável para DLS.
- Semântica/units/acquisition de G/C e Lognormal/Multimodal, e dados de aquisição
  com settings/algoritmo confiáveis para qualquer recálculo.
- Documentação/exports reais de potential/mobility/quality/settings/model e
  arrays/reference DTS correspondentes para elevar capacidades ZETA.
- Schema/version/settings/exports/results NanoSight reais; trajectories,
  timebase/calibration/algorithm se tracking calculations forem implementadas.
- Referência externa independente/histórica de versões relevantes onde pesquisas
  tiveram 403/challenges; não tratar bloqueio ou search snippet como documentação.
- Source/rebuild completos do runtime estatístico, inputs de toolchain/image
  exatos e decisão de distribuição/licença; fontes pinadas parciais não bastam.
- Teste real clean Windows/USB/offline e demais target systems, providers reais
  após registration segura, identidade de release e signing completo.

Consulte os blockers por módulo em
[TECHNICAL_CONTEXT.md](../../docs/TECHNICAL_CONTEXT.md) e a divisão entre trabalho
de desenvolvimento e intervenção do proprietário em
[READINESS.md](../../docs/READINESS.md). Registre ausências sem inventar fatos e
continue o que puder implementar/testar com definição suficiente.
