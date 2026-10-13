<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Especificação mestre para continuidade de desenvolvimento

Este arquivo contém requisitos de desenvolvimento do MNE Lab. A instrução
explícita mais recente do proprietário autoriza manter no repositório o contexto
necessário à passagem entre ferramentas de desenvolvimento. **Esse conteúdo
não pertence à interface, ao runtime, às instalações ou aos artefatos do produto.**
Corpus experimental, originais DTS, workbook Zeta, secrets, conversas privadas
e referências históricas privadas continuam externos ao repositório público.

Leia integralmente esta especificação e depois [USER_DIRECTIVES.md](USER_DIRECTIVES.md)
antes de agir. As instruções posteriores complementam ou restringem a fonte
histórica; não recrie a aplicação nem altere `main` diretamente. Revise o que já
existe e confronte cada requisito com implementação, testes e limites de evidência.

## Proveniência e fidelidade

A fonte é o mestre consolidado vigente fornecido pelo proprietário, versão do
pacote de implementação atualizado. Fonte original: **55041 bytes, 1505 linhas,
44 seções principais**. SHA-256 original:
`c43e35335bac42cf2201ac4db71c67aa05b7914ded7fdb3235e55d6f84502578`.
Este hash identifica a fonte antes da minimização, não os bytes deste arquivo.

O corpo entre os delimitadores abaixo preserva integralmente os títulos,
numeração, subseções, requisitos e qualificações da fonte. Foram substituídos
apenas três identificadores de grupos do corpus por `GRUPO_PRIVADO_1`,
`GRUPO_PRIVADO_2` e `GRUPO_PRIVADO_3`, duas ocorrências do mesmo exemplo de
unidade por `Unidade Sintética A` e o nome do workbook experimental pelo
localizador lógico `WORKBOOK_ZETA_PRIVADO`. Os aliases de grupos representam
corpus real externo; não significam fixtures sintéticos. O exemplo de unidade
é sintético e mantém a relação longitudinal exigida.

A fonte consolidada de 44 seções **não é a transcrição completa** da conversa
histórica nem recupera literalmente as versões antigas mencionadas até a
seção 422. Nenhum transcript ou dado experimental foi incorporado aqui.
Os materiais disponíveis, ausentes e seus limites estão em [MATERIALS.md](MATERIALS.md).

## Aplicação das instruções posteriores

Os trechos históricos continuam visíveis para permitir auditoria:

- **§1 e §34:** a criação inicial de repositório foi superada pela ordem de
  continuar o projeto existente e preservar o trabalho anterior. Não repetir
  essa inicialização, descartar histórico ou modificar `main` diretamente.
- **§17:** a antiga autorização exclusiva de LIGHTSCATTERING foi ampliada.
  ZETA permite implementação sustentada por evidências; NTA permite arquitetura
  e preparação. A validação científica de ambos permanece pendente.
- **§43:** planilha e abertura de arquivo não bastam para validar ZETA. Aplicam-se
  documentação adequada, resultado reproduzível e referência confiável, com
  escopo/tolerâncias. Não aproveitar o PDF de size analysis como documentação
  de potencial Zeta.
- **§4 e §31:** o produto e seu contexto técnico neutro continuam sem atribuições
  desnecessárias a ferramentas. A coordenação autorizada reside na área de
  desenvolvimento e nos pontos de entrada de desenvolvimento, sem entrar no
  conteúdo distribuído do aplicativo.

As demais instruções operacionais e científicas atuais estão em
[USER_DIRECTIVES.md](USER_DIRECTIVES.md). A matriz
[REQUIREMENTS_REVIEW.md](REQUIREMENTS_REVIEW.md) preserva as 44 seções, com lacunas
explicitamente abertas; cobertura documental não significa conformidade completa.

<!-- BEGIN_MASTER_REQUIREMENTS_BODY -->
# MNE LAB — PROMPT MESTRE CONSOLIDADO ATUAL

Você é responsável integralmente pela criação, implementação, validação, testes, arquitetura, segurança, privacidade, empacotamento, versionamento e evolução do projeto **MNE Lab**.

Tudo neste documento é requisito de produto, salvo quando marcado como referência futura. Não entregue apenas um plano: depois do planejamento mínimo necessário, comece efetivamente a construir.

---

## 1. IDENTIDADE E REPOSITÓRIO

O nome visual do produto é exatamente **MNE Lab**.

Crie um repositório novo, virgem e independente, com código, histórico Git, arquitetura, configuração, documentação e pipelines próprios. Não reutilize histórico Git, branches ou estrutura de outro projeto como base oficial.

O repositório oficial deve ser **PUBLIC** desde sua criação. Confirme a visibilidade pública antes de prosseguir significativamente.

Repositório público NÃO autoriza publicar senhas, tokens, refresh tokens, private keys, certificados privados, secrets, `.env` sensível, dados pessoais, dados científicos, corpus real de desenvolvimento, backups, recovery ou arquivos dos usuários.

Se a plataforma não permitir literalmente `MNE Lab` como nome do repositório, use a alternativa tecnicamente mais próxima sem alterar o nome visual do produto.

---

## 2. USO DE REFERÊNCIAS E FERRAMENTAS

"Começar do zero" significa repositório/histórico/arquitetura próprios, não isolamento técnico. Pesquise e utilize bibliotecas maduras, frameworks, documentação oficial, open source, artigos, benchmarks, padrões, ferramentas de testes, profiling, segurança e referências de arquitetura quando útil.

Respeite licenças, copyright, atribuições obrigatórias, segurança e compatibilidade. Não faça cópia cega.

---

## 3. AUTONOMIA

Assuma o volante técnico: pesquisar, decidir, implementar, testar, depurar, refatorar, documentar, empacotar, validar e corrigir. Não transfira ao usuário tarefas técnicas que você consegue executar.

Prioridade: integridade > confiabilidade > segurança > privacidade > correção científica > portabilidade > performance > fluidez > manutenção > UX > sofisticação visual.

---

## 4. SEM MARCAS DE IA NO PRODUTO

Não deixe branding, texto, comentários, metadata ou trailers desnecessários que atribuam o produto a Claude, Claude Code, Anthropic, IA, AI assistant/generator ou ferramenta semelhante.

Antes de release público, faça no mínimo 3 verificações independentes em código, comentários, documentação, Git metadata/trailers, configs, bundles, executável, assets, installer e artifacts.

A ausência dessas marcas não substitui atribuições legalmente obrigatórias de dependências.

---

## 5. DESIGN E UX

MNE Lab deve parecer um ambiente científico/laboratorial sofisticado, preciso, moderno, confiável e profissional. Evite aparência infantil, gamer, genérica, template, demo ou dependente do visual padrão do sistema operacional.

Crie design system coerente: tipografia, escala, spacing, grid, superfícies, bordas, radius, depth, iconografia, cores, contraste, states, feedback, motion, density, responsividade, error/loading/empty/offline.

Nada visível está pronto se estiver feio, desalinhado, inconsistente, quebrado, improvisado, com placeholder, asset borrado/deformado ou ícone ruim.

Crie ícone oficial MNE Lab e variantes apropriadas para executable, window, taskbar, Alt+Tab, Start, shortcuts, installer, uninstaller, notifications, tray, favicon e web/mobile.

System tray, quando suportado, deve oferecer abrir/restaurar, status, settings e saída segura. Exit não é kill bruto.

A interface principal deve evitar page scrolling em uso normal no desktop, notebook, mobile, viewer e remote. Use panels, drawers, tabs, modals, overlays e layouts adaptativos.

Valide resoluções, aspect ratios, DPI, scaling, textos maiores, idiomas, mobile, hover/focus/loading/error/disabled/selected, Turbo e screenshots reais. Visual regression quando viável.

Footer da Home: **made by oaovito**, extremamente discreto, pequeno, elegante e legível.

---

## 6. LOCALIZAÇÃO

Toda a aplicação segue o idioma do dispositivo por padrão: desktop, USB, Temporary, mobile, onboarding, auth, Profiles, menus, notifications, tray, updater, LockedBuild, errors, recovery, Help, What's New, scientific modules, graphs e export.

Detecte idioma na primeira execução e permita alteração manual posterior. IDs internos devem ser neutros e nunca traduzidos. `LockedBuild` permanece exatamente assim.

---

## 7. ACCOUNT E PROFILES

Primeira interação significativa: **Create Account** ou **Sign In**, localizado.

Account = identidade/recuperação do ambiente. Profile = ambiente de trabalho. Máximo de 5 Profiles por Account.

Seleção de Profile ocorre antes da Home. Se houver menos de 5, ofereça Create Profile; nunca permita sexto.

Criação de Profile exige `Username` e `Storage Mode`: `USB Drive + Cloud`, `USB Drive Only`, `Cloud Only`.

Avatar opcional até 5 MB: PNG, JPG/JPEG, WebP, GIF; GIF animado quando apropriado.

Senha do Profile é opcional, pertence apenas a ele, nunca plaintext e deve usar KDF/hash moderno com salt. Não existe backdoor/master password MNE Lab.

Isolamento de Profile deve abranger files, history, settings, tokens, credentials, backups, recovery, scientific data, graphs, cycles, Import Profiles, Export History e preferências.

No header, avatar à direita; menu contém **My Stuff** e **Change Account**. My Stuff deve ser extensível.

Trocar Profile/Account deve finalizar estado, sincronizar quando aplicável, limpar caches/sessão sensível e carregar apenas o novo namespace.

---

## 8. TURBO

Home possui `Turbo`. Quando ligado, reduz/desabilita animações e efeitos cosméticos para priorizar performance sem alterar correção científica.

Toggle usa Save → Persist → Preserve → Restart → Restore. Persistente até o usuário desligar. Restart de Turbo não é final Exit.

---

## 9. MODOS DE EXECUÇÃO

### Portable USB Mode

Aplicação vive no USB e funciona em máquinas compatíveis, online/offline, sem depender de drive letter, username/path fixos ou software de desenvolvimento previamente instalado. Prefira paths relativos.

### Temporary Machine Mode

Versão completa sem USB. Fluxo: Download → Open → Sign In/Create Account → Restore → Work → Save → Sync → Verify → Clean → Exit.

No encerramento normal: bloqueie mudanças, finalize transactions/journal, salve, sync, verifique remoto, garanta que não existe dado válido somente local, encerre mobile/local servers, invalide tokens temporários, limpe RAM sensível e artifacts temporários controlados pelo MNE Lab, depois Exit.

Não apague logs do SO para esconder execução. Crash/power loss/Task Manager/BSOD são abnormal shutdowns e priorizam recovery.

Se não houver internet/remote confirmation, NUNCA destrua a única cópia válida. Preserve recovery mínimo, criptografado, transactional, integrity-checked e Account/Profile-bound.

---

## 10. RECOVERY, WRITES E BACKUP

Nunca dependa apenas de RAM para trabalho não persistido. Use journal/WAL/snapshots/manifests/checksums/atomic writes/encryption quando necessário.

`Cloud Only` significa cloud como destino persistente final, não RAM+Cloud. Enquanto cloud não confirmar, mantenha recovery local criptografado.

Safe write crítico: temp → write → flush/fsync → validate → checksum → atomic replace → previous recoverable version.

Nunca apague a única cópia válida.

Backup deve tolerar crash, power loss, USB removal, disk failure, cloud outage, upload failure, corruption e update interruption.

---

## 11. SYNC E ISOLAMENTO ENTRE INSTALAÇÕES

Persistent sync queue com operation ID, Account/Profile namespace, revision, transaction ID, checksum, origin, state, retries e last remote confirmation.

Estados conceituais: local, pending, syncing, confirmed, conflict, retryable. Sync idempotente. Não use Last Write Wins ingênuo em dados críticos.

REGRA ABSOLUTA ENTRE INSTALAÇÕES:

**Same Account + Same Profile = sync may be allowed.**

Qualquer outra combinação = **ISOLATE**.

Same Account + Different Profile: NÃO sincroniza.

Different Account + Profile com mesmo nome: NÃO sincroniza.

Mesma LAN/Wi-Fi, mesmo PC ou mesmo cloud provider não autorizam sync.

Use identificadores internos estáveis; nunca associe Profile pelo nome/avatar.

Nova instalação só recupera dados após Sign In → Same Account → Same Profile → provider authorization → validate → restore/sync.

Mesmo Account + Same Profile não autoriza sincronizar private keys.

Teste explicitamente todas as combinações positivas e negativas.

---

## 12. PRIVACIDADE

Princípio: **MNE Lab provides software; user keeps data.**

Não mantenha database central MNE Lab com identidade, profiles, files, history, scientific data, backups, recovery, cloud content, tokens ou preferences dos usuários.

Prefira arquitetura `MNE Lab client ↔ user's provider`.

Use OAuth2/OIDC/PKCE/Device Flow/APIs oficiais, least privilege e app-specific storage quando possível. Nunca peça password do provider quando existir fluxo oficial.

Tokens ficam no dispositivo/USB/secure store apropriado; nunca em analytics, repo ou servidor MNE.

Sem personal telemetry por padrão. Crash reports locais por padrão; qualquer upload futuro exige opt-in explícito e sanitização.

---

## 13. CLOUD PROVIDERS

Exatamente nesta ordem:
1. Google Drive / Google One
2. iCloud
3. Microsoft OneDrive

Google: OAuth/PKCE, APIs oficiais, scopes mínimos, Google One não é filesystem separado.

iCloud: somente mecanismos oficiais por plataforma; pesquise e documente limitações reais, sem fingir suporte.

OneDrive: Microsoft Identity/OAuth/PKCE/Graph/OneDrive APIs.

USB+Cloud e Cloud Only exigem provider. USB Only não.

Provider migration deve ocorrer localmente: Provider A → MNE client → validate → Provider B, mantendo A intacto até B confirmar.

LockedBuild não afeta sync.

Cloud UI deve ter qualidade visual equivalente, com assets oficiais/licenciados quando disponíveis.

---

## 14. MOBILE

Home possui botão que abre modal/overlay com QR, sem page navigation. Smartphone usa browser, sem app install.

Mobile funciona como viewer + remote controller + presentation controller, preferindo ligação local/direta.

Pairing usa token criptograficamente random, expiring/scoped, replay protection, invalidation e origin/CSRF controls quando aplicável. IP/porta não bastam.

Mobile não cria datastore científico independente nem envia dados através de infraestrutura MNE quando evitável.

---

## 15. UPDATES E LOCKEDBUILD

Automatic Update = ON por padrão. Única exceção: Portable USB explicitamente em LockedBuild.

Stable update: detect → authenticate/signature → compatibility → save/preserve → download → validate → install → migrate → validate → restart → restore.

Packages/manifests devem ser autenticados, com checksums/signatures apropriados.

LockedBuild existe somente em USB, fixa a build e seu conjunto compatível de dependencies. Não significa offline/degraded/no sync.

Ativação explicitamente pelo usuário. Pode notificar discretamente nova Stable, mas não atualizar automaticamente.

Build selector mostra current version/date/channel/latest stable/compatibility/release notes/migrations. Downgrade só se seguro.

Build != User Data. Migrations precisam ser transactional e recoverable.

---

## 16. DISCOVERABILITY / USER AWARENESS

Usuário deve saber quais funções existem sem adivinhar.

Onboarding curto, refinado, skippable e acessível depois. Use contextual first-use hints sem nagging.

Help/Guide/Features organizado e pesquisável, offline para documentação local importante.

Tooltips em ícones não óbvios. What's New discreto para funções novas. Se função existe mas está indisponível, às vezes mostre disabled com reason em vez de esconder.

Search/command palette deve localizar features/pages/settings/actions/help com sinônimos localizados. Ex.: DLS → LIGHTSCATTERING; signature → Sign Export/Verify Signature.

Estados do sistema devem ser compreensíveis: saved/pending/offline/provider/sync error/recovery/update/LockedBuild/mobile/import/graph/export etc. Cor não pode ser o único sinal.

---

## 17. DOCUMENTAÇÃO CIENTÍFICA OFICIAL

Use `02_SCIENTIFIC_REFERENCE/Documentacao_de_Dados_Lightscattering_e_Zeta_LATEST.pdf` como referência científica/funcional inicial oficial.

Ela define contexto para NanoBrook 90Plus / Brookhaven DLS e também contém Zeta. Nesta fase, **somente LIGHTSCATTERING está autorizado para implementação**. Zeta é referência futura, salvo nova instrução explícita.

O PDF orienta interpretação, fields, units, parser expectations, graphs, temporal/stability studies, tests e Help.

LIGHTSCATTERING relevante: Effective Diameter, Polydispersity/PDI, Current Count Rate, BaseLine Index, Diameter/Particle Size, Intensity.

PDF = scientific specification. Experimental TXT/XLSX/etc = experimental data. Nunca confundir.

Autoridade deve ser tratada por função, não de forma ingênua: o arquivo real é autoridade para os valores exportados e sua sintaxe concreta; a documentação do projeto/fabricante é autoridade para significado e semântica; literatura confiável amplia contexto científico. Conflitos devem ser investigados e documentados.

Registre filename/hash/version/revision/incorporation date da especificação. PDF não é runtime dependency.

Fluxo: documentation → rules/expected meaning → real files → validation → parser → normalized data → graphs/cycles/statistics → presentation/export.

---

## 18. CORPUS REAL DE DESENVOLVIMENTO

Este pacote inclui corpus real privado em `03_PRIVATE_DEVELOPMENT_CORPUS/`, originado do Drive fornecido pelo proprietário.

Contém grupos GRUPO_PRIVADO_1, GRUPO_PRIVADO_2, GRUPO_PRIVADO_3 e `WORKBOOK_ZETA_PRIVADO`. Use para compreender formatos reais, headers, delimitadores, units, naming, replicates, temporal organization, edge cases e para validar parser/normalization/graphs/cycles/statistics.

O corpus é **Development Reference Corpus**, nunca Product Content.

"Treinar a aplicação" significa estudar dados reais, generalizar regras, criar parsers/validators e fixtures sintéticos. Não significa embutir os experimentos no runtime ou memorizar sample names/values.

NUNCA inclua dados reais em repo público, executable, installer, Portable build, Temporary package, assets, demo, screenshots públicas, default DB, CI público ou release. Mantenha em `.private-fixtures/` / `.dev-data/` gitignored quando trabalhar localmente.

Crie fixtures sintéticos públicos que repliquem structure/headers/delimiters/units/edge cases sem copiar desnecessariamente valores reais.

Private validation + Public synthetic regression suite.

---

## 19. PRIMEIRO MÓDULO: LIGHTSCATTERING

Crie módulo `LIGHTSCATTERING` para dados inicialmente do NanoBrook 90Plus.

Importação: file picker, drag & drop, multi-file, add to experiment.

Preserve byte-for-byte o original. Mantenha separadamente: Original File + Parsed Data + Normalized Data + Metadata + Derived Graphs.

Metadata: filename, format, size, hash, import date, measurement date/time se presente, Sample ID, units, parser version, origin, notes.

File Library pesquisável por name, Sample ID, date, experiment, group, tag, period e notes. Permita reuso sem reimportar.

Provenance graph → source files → timestamps/samples → transforms/filters/statistics → parser/spec version.

Campos esperados quando presentes: Effective Diameter, PDI/Polydispersity, Current Count Rate, BaseLine Index, Diameter/Particle Size, Intensity. Missing != zero.

DLS principal: X Hydrodynamic Diameter (nm) quando confirmado; Y Intensity (%) quando confirmado; log X quando cientificamente apropriado.

Se arquivo fornece Intensity/Volume/Number, permita selecionar. Não converta intensity → volume/number sem método válido e transparente.

Graph pode usar 1 ou múltiplos arquivos, com overlay/comparison, legends e acessibilidade.

---

## 20. GRAPH LIBRARY E GRAPH ENGINE

Graph Library salva definição reproduzível, não somente imagem: sources, series, axes, scale, filters, labels, calculations, statistics e visual config.

Interações quando úteis: hover, inspect, zoom, reset, series toggle, metadata, source navigation, compare.

Design científico deve ser refinado sem distorcer dados. Nunca esconda peaks, altere values, suavize sem informar, corte outliers silenciosamente ou manipule escala por estética.

A documentação indica X logarítmico para distribuições DLS, labels Hydrodynamic Diameter (nm) / Intensity (%), tipografia limpa, linhas legíveis e comparação de múltiplas amostras. Use isso como preset científico, adaptando ao design MNE Lab sem perder correção.

---

## 21. CYCLES

Graph/selection compatível oferece `Create Cycle`.

Intervalo inteiro 1–99 com Hours/Days/Weeks/Months/Years.

Fields: name, files, experiment, sample, start date, interval, unit, duration, replicates, parameters. Preview temporal antes de criar.

Suporte replicates/triplicates, raw individual values, mean e standard deviation quando houver observações suficientes.

Possíveis trends quando os dados suportarem: Effective Diameter, PDI, Count Rate, BaseLine, DLS curves ao longo do tempo, comparisons por período.

Nunca invente missing values. Missing cycle point = gap. Preserve timestamps reais; não invente timezone. Months/Years usam calendar semantics, não 30d/365d fixos.

Cycle Library persistente, sincronizável conforme regras de Profile e recuperável offline.

---

## 22. PRESENTATION MODE

Graphs possuem Presentation Mode fullscreen, removendo UI desnecessária e mantendo somente controles essenciais. Multi-graph next/previous, keyboard/mouse/touch/mobile remote.

Presentation não altera os dados científicos.

---

## 23. DATA INGESTION MULTIFORMATO

Crie arquitetura separada: File Decoder → Tabular Reader → Structure Detector → Scientific Parser → Normalizer → Validator → Storage.

Suporte prioritário real: TXT, CSV, TSV, XLSX, XLS e ODS quando biblioteca madura/confiável permitir.

Prepare extensibilidade para JSON estruturado e futuros instrument exports.

Formato é container; conteúdo define o dataset científico. TXT não significa automaticamente LIGHTSCATTERING e XLSX pode conter LIGHTSCATTERING.

Excel não é dependência. Leia `.xlsx/.xls` sem Microsoft Excel instalado. Enumere worksheets, permita selecionar uma/algumas/todas compatíveis, preserve workbook original, sheet/range/mapping na provenance.

ODS não depende de LibreOffice.

TXT/CSV/TSV devem tratar delimiter, decimal comma/point, encoding, headers, comments, missing values, units, repeated sections e ambiguidades.

Não confie apenas na extensão. Inspecione conteúdo/MIME/structure.

Smart Import: Drop → Inspect → Detect Format → Detect Structure → Detect Scientific Module → Preview → Confirm → Import. Se ambíguo, pergunte; não chute.

Preview mostra filename, format, sheet, rows, columns, headers, units, scientific type detected, warnings, unknown fields.

Quando necessário, permita manual column mapping visual/reproducible e Import Profiles Profile-scoped.

Usuário deve saber que o app suporta NanoBrook TXT, TXT, CSV, TSV, Excel e ODS: Import Hub, drag/drop, Help, onboarding e command palette. Não esconda essas capacidades.

Google Sheets/cloud spreadsheets poderão ser importados por APIs oficiais e exportação segura quando provider estiver conectado, sem pedir password Google ou enviar planilha a servidor MNE.

---

## 24. SMART EXPORT

Sistema global `Smart Export`.

Princípio: **RECOMMEND WITHOUT RESTRICTING**.

Mostre `Recommended` primeiro e `All Compatible Formats` depois.

Figures: PNG, SVG, PDF, TIFF recomendados; JPEG/JPG/WebP/EPS e outros quando realmente suportados. Settings: width/height/aspect, px/mm/cm/in, DPI/PPI, background, title, legend, metadata, line thickness, font, margins.

Presets: Quick, Screen, Presentation, Print, Scientific Publication, Raw Data, Complete Research Package, Custom.

Data export: CSV/XLSX/TSV/TXT recomendados; JSON/Parquet quando apropriado. Preserve units, Sample ID, timestamps, replicates, cycle points, source references e metadata.

Cycle: Export Results / Export Data / Export Full Cycle.

Complete Research Package deve conter originals, normalized data, graphs, statistics, provenance e manifest com checksums, parser/schema/spec versions e relationships. Planeje reimport futuro, mesmo que não seja primeira entrega.

Batch export múltiplos objetos, com naming inteligente e collision handling explícito. Preview deve corresponder ao output final.

Export Engine separado do Graph Engine, usando modelo intermediário reproduzível.

Temporary Mode: deliberate user export sobrevive ao cleanup.

---

## 25. SIGN EXPORT — SISTEMA UNIFICADO

Integre ao Smart Export:
- None
- Visible Signature
- Digital Signature
- Complete Signature = Visible + Digital

Visible Signature pode conter, opcionalmente, nome, instituição, laboratório/departamento, role, ORCID, date/time, texto adicional e imagem de assinatura do usuário.

Digital Signature = integridade/autenticidade criptográfica. Nunca apresente assinatura visual como equivalente à digital.

Complete Signature pipeline obrigatório: final scientific artifact → apply visible signature → finalize bytes → hash → digital signature → validate → signed artifact. Nunca aplique marca visual depois da assinatura digital.

Private key pertence ao usuário. Nunca sincronize via Google Drive/iCloud/OneDrive/Profile sync/backup/recovery/Complete Research Package; nunca logue, envie ou armazene em infraestrutura MNE.

Suporte arquitetural, quando disponível, para OS certificate store, PFX/P12, smart card, hardware token e padrões consolidados. Não crie crypto proprietária.

PFX/P12: não persista password por padrão. Certificate store UI mostra apenas subject/issuer/validity/purpose/fingerprint parcial quando útil.

PDF: assinatura interoperável, PAdES quando apropriado. CSV/TXT e formatos sem mecanismo embutido confiável: detached signature, por exemplo `.p7s`. Imagens podem ter assinatura visual; digital preferencialmente detached se não houver padrão adequado. XLSX/ODS: não altere scientific cells; use metadata/header/footer/dedicated sheet para visible signature e assinatura nativa somente se madura/interoperável, caso contrário detached.

Complete Research Package pode ser assinado via package/manifest e checksums.

Stored original nunca é modificado para assinar: create export copy → sign copy.

Implemente `Verify Signature` com estados distintos: Signature Valid; Valid—Trust Unknown; Invalid; File Modified After Signing; Certificate Expired; Revoked quando verificável; Timestamp Valid; Unable to Verify.

Diferencie cryptographic validity de trust. Self-signed pode ser matematicamente válido sem identidade independentemente confiável.

Assinaturas devem ser verificáveis fora do MNE Lab quando o padrão permitir.

Signing Identity é Profile-scoped. Mesmo Account+Same Profile pode sincronizar preferências não sensíveis, mas nunca private key. Cada instalação precisa acesso legítimo local à chave para assinar.

Temporary Mode deve limpar PFX/password/PIN/certificates temporários quando seguro. USB não armazena private key por padrão.

---

## 26. DEPENDENCY INDEPENDENCE / SELF-CONTAINED RUNTIME

REGRA ABSOLUTA: MNE Lab não pode depender de o usuário já possuir Python, Node/npm/pnpm/yarn, Java/JRE/JDK, .NET Runtime, Visual C++ Redistributable, database server, LibreOffice, Microsoft Excel, Git, OpenSSL CLI, FFmpeg, package managers, SDKs, scientific libraries ou ferramentas de desenvolvimento instaladas.

Experiência esperada: Download/USB → Open → MNE Lab funciona.

Development Environment != User Runtime Environment.

Sempre que possível, distribua application + bundled runtime + bundled libraries + bundled internal tools. Prefira application-local > user-scoped > system-wide.

Nunca mande o usuário pesquisar/download/configurar dependências manualmente, editar PATH, instalar pip/npm packages ou localizar DLLs.

Se uma dependência externa for realmente inevitável e não puder ser redistribuída por motivo técnico/legal/licenciamento: detecte automaticamente; use fonte oficial; faça download quando permitido; valide HTTPS/origin/hash/signature/license/version/architecture; instale/configure; valide; continue. O usuário não coleta a ferramenta.

Quando necessário, use bootstrapper MNE Lab: Launch → Environment/Platform Check → Required Components Check → Repair/Install Missing → Validate → Start.

Evite admin/root. Se inevitável, use mecanismo oficial do SO somente no momento necessário; nunca peça terminal manual.

### Portable USB

Requisito ainda mais rígido: no host limpo, sem runtime dev, Excel, LibreOffice, OpenSSL CLI ou scientific tools, Insert USB → Open → Run. Inclua runtime portátil quando apropriado e legal. Use relative paths; não dependa de PATH global, fixed drive letter, user profile ou registry dev-specific. Núcleo deve funcionar offline sem baixar runtime no primeiro launch.

### Temporary Mode

Também self-contained. Registre componentes temporários criados e remova apenas aquilo que pertence exclusivamente ao MNE Lab quando seguro. Nunca remova componentes do SO/terceiros já existentes.

### Formatos e ferramentas

XLSX/XLS não dependem de Excel. ODS não depende de LibreOffice. PDF generation/signing não depende de Adobe Acrobat. Sign Export não depende de OpenSSL CLI. Graph Engine não depende de Origin/Orange/MATLAB/R/Python externo. Scientific calculations necessárias devem estar no produto. Local state não exige PostgreSQL/MySQL/MongoDB/Redis externo.

Internet só é requisito para funções genuinamente online: cloud, OAuth, update, remote timestamp etc. Funções locais devem funcionar offline.

Quando viável, forneça distribuição/installer capaz de instalação completa offline para laboratórios/rede restrita.

Controle supply chain: versions pinned, lockfiles/manifests/checksums, source/license/signature, reproducible builds e controlled upgrades.

Não dependa de PATH global, PYTHONPATH, NODE_PATH, JAVA_HOME, absolute dev paths, locally installed fonts, Excel COM, LibreOffice executable, OpenSSL CLI, machine-specific registry ou DLL existente só no dev machine.

### Clean-machine release gate

A referência de distribuição é uma máquina compatível limpa, não o PC de desenvolvimento.

Teste em fresh VM/clean Windows/fresh user/isolated CI quando possível.

Teste mínimo:
Fresh supported machine + No Python + No Node + No Java dev env + No Excel + No LibreOffice + No Git + No OpenSSL CLI + No scientific apps + No developer tools = MNE Lab Works.

Teste separadamente normal installation, Portable USB, Temporary Mode e offline local functions. Na clean machine, teste startup, Account/Profile, storage, LIGHTSCATTERING import, TXT/CSV/TSV/XLSX/XLS/ODS, Graph Engine, Cycle Engine, Smart Export, PDF generation, spreadsheet export, Visible/Digital/Detached signature, Verify Signature e offline.

Se o software funciona apenas porque a máquina dev possui algo global instalado, isso é **distribution bug**.

Uma Stable Build NÃO está pronta se funciona no PC de desenvolvimento mas falha em máquina compatível limpa por falta de software/runtime/library/ferramenta pré-instalada.

Regra final: **MNE LAB MUST BRING WHAT MNE LAB NEEDS.**

---

## 27. DATA MODEL, PROVENANCE E AUDITABILIDADE

Scientific Data Model semântico para measurements, samples, replicates, distributions, units, timestamps, parameters, source files, cycles, derived stats e provenance. Não use apenas arrays genéricos.

Units fazem parte do dado. UI rounding não altera stored precision. Locale formatting separado do valor interno.

Measurement timestamp != import timestamp; não invente measurement date/timezone.

Derived result deve registrar source IDs, parser version, scientific spec version, transformations/parameters/statistics/graph definition/schema.

End-to-end: Scientific Reference → Scientific Rules; Original Experimental File → Parsed → Normalized → Graph → Cycle → Statistics → Presentation → Smart Export → Optional Signature.

Deletion de source com dependências deve avisar e preservar provenance; considere Trash/Soft Delete.

---

## 28. PERFORMANCE E ACCESSIBILITY

Evite aggressive polling, busy loops, high idle CPU, excessive RAM/rerenders e heavy effects. Use indexing, virtualization, lazy loading, caching e async para grandes bibliotecas.

Heavy operations não bloqueiam UI; mostre progress e permita cancel quando seguro, deixando estado consistente.

Turbo reduz efeitos, não correção.

Accessibility: contrast, readability, keyboard, focus, scaling, reduced motion e feedback. Cor nunca é único indicador.

---

## 29. LOGGING E SECURITY

Logs locais com levels/rotation/retention. Nunca registre passwords, full tokens, refresh tokens, private keys, certificate passwords, secrets ou private scientific content.

No repo: no secrets; `.gitignore`, secret scanning, dependency/security audit.

---

## 30. TESTES E RELEASE GATE

CI/release, quando aplicável: format, lint, typecheck, unit, integration, E2E, build, dependency audit, security audit, secret scan, portability, privacy, visual regression, parser, graph, cycle, export, signature, scientific validation, artifact-attribution audit.

Data ingestion tests: TXT/CSV/TSV/XLSX/XLS/ODS, multiple sheets, empty sheet, decimal comma/point, malformed/corrupt file, false extension, encoding, units, multiple datasets, large file, mixed types, mapping.

LIGHTSCATTERING tests: real private corpus quando disponível + synthetic fixtures; Effective Diameter/PDI/Count Rate/BaseLine/Diameter/Intensity; log X; single/multi; replicates/triplicates; mean/SD; cycles; missing points; irregular timestamps; provenance; offline; sync; USB; Temporary; Presentation; mobile; Smart Export.

Isolation tests: Same Account+Same Profile sync; same account/different profile no leak; different account/same visible profile name no leak; same PC/different profiles no leak; same network no authorization; Portable+Temporary same account/profile continuity.

Signature tests: unsigned, visible, digital, complete, PDF, PNG/CSV detached, XLSX, package, batch, PFX/P12, certificate store, self-signed, expired, revoked when possible, corrupted, wrong password, modified-after-signing, offline, Temporary cleanup, Profile/install isolation, verify outside MNE when possible.

Stable release gate também exige: clean machine test, scientific validation, privacy/security audit, no private corpus in artifacts, no secrets, package integrity, migrations, portability, visual quality e no mínimo 3 independent scans for unwanted AI/tool artifacts.

---

## 31. CONTEXT DOCUMENT

Mantenha arquivo técnico neutro no repo com architecture, decisions, implemented/partial/pending, auth, storage, privacy, providers, sync, backup, recovery, updater, LockedBuild, Temporary, USB, mobile, localization, design, onboarding, scientific reference/hash, parser, ingestion, provenance, Graph Engine, Cycles, Export, Signatures, dependency packaging, tests, known issues e next steps.

Não mencione Claude/IA nesse documento.

---

## 32. DEFINIÇÃO DE PRONTO

LIGHTSCATTERING só está pronto quando importa arquivos reais/compatíveis, preserva original, normaliza corretamente, mantém provenance, File/Graph/Cycle Libraries funcionam, cycles 1–99 funcionam, replicates/statistics corretos, Presentation, Smart Export, Sign Export, Verify Signature, offline/cloud/USB/Temporary, backup/recovery/localization/design/scientific validation e visual validation passam.

Data Ingestion só está pronto quando formatos realmente prometidos funcionam, sheets/preview/mapping/original/provenance/awareness funcionam e real private + synthetic validation passam.

Sign Export só está pronto quando None/Visible/Digital/Complete funcionam, private key permanece privada, PDF é interoperável, formatos incompatíveis usam detached signature, Verify funciona, modified file é detectado, trust é diferenciado, Temporary limpa secrets e clean-machine/offline cases passam.

---

## 33. PRINCÍPIOS ABSOLUTOS

- NEVER DELETE THE ONLY VALID COPY.
- APPLICATION BUILD != USER DATA.
- DOCUMENTATION != EXPERIMENTAL DATA.
- FORMAT != SCIENTIFIC MEANING.
- RAW DATA != DERIVED DATA.
- VISIBLE SIGNATURE != DIGITAL SIGNATURE.
- SAME ACCOUNT + SAME PROFILE = SYNC MAY BE ALLOWED.
- ANY OTHER ACCOUNT/PROFILE COMBINATION = ISOLATE.
- PRIVATE KEYS NEVER SYNC AUTOMATICALLY.
- REAL DEVELOPMENT DATA NEVER SHIPS TO USERS.
- RECOMMEND WITHOUT RESTRICTING.
- SCIENTIFIC DATA MUST REMAIN TRACEABLE AND REPRODUCIBLE.
- VISUAL QUALITY AND FUNCTIONAL CORRECTNESS ARE BOTH REQUIRED.
- MNE LAB MUST BRING WHAT MNE LAB NEEDS.

---

## 34. ORDEM INICIAL DE IMPLEMENTAÇÃO

1. validar ambiente/ferramentas;
2. criar repo PUBLIC e confirmar visibilidade;
3. configurar Git, `.gitignore`, secret/data scanning;
4. impedir corpus privado de entrar no repo/build;
5. escolher stack com foco em self-contained clean-machine delivery;
6. arquitetura modular;
7. design system/localization;
8. Account/Profile/namespace isolation;
9. storage/encryption/recovery/backup/sync;
10. Google/iCloud/OneDrive abstractions;
11. Portable/Temporary/update/LockedBuild/tray/mobile;
12. onboarding/Help/discovery;
13. analisar PDF e registrar hash/spec version;
14. analisar corpus privado e gerar synthetic fixtures;
15. Scientific Data Model + Data Ingestion Layer;
16. TXT/CSV/TSV/XLSX/XLS/ODS readers;
17. Smart Import/Preview/Mapping/Import Profiles/File Library;
18. NanoBrook parser + validation real/synthetic;
19. provenance;
20. Graph Engine + DLS single/multi + Graph Library;
21. Cycle Engine/Library/statistics;
22. Presentation/mobile integration;
23. Smart Export/Figure/Data/Package/Export History;
24. Visible Signature + crypto layer + PDF/detached + Complete Signature + Verify;
25. dependency bundling/bootstrapper/repair/offline installer strategy;
26. clean-machine testing;
27. functional/visual/scientific/security/privacy/release audits;
28. atualizar context document.

Depois do planejamento mínimo necessário: **COMECE EFETIVAMENTE A CONSTRUIR O MNE LAB.**

Não marque mock como pronto. Não finja suporte. Não esconda limitações. Não publique parser cientificamente não validado. Não publique corpus privado. Não publique private keys. Não permita sync incompatível. Não sacrifique integridade por cleanup, ciência por estética ou estética por falta de refinamento.


---

# 35. STATISTICAL ANALYSIS ENGINE - ANOVA / MIXED EFFECTS

Implemente agora um módulo estatístico nativo, reproduzível e integrado ao MNE Lab.

Não crie apenas um botão genérico "ANOVA".

O sistema deve compreender o desenho experimental antes de recomendar/executar o método.

## 35.1 Métodos obrigatórios

Suportar, com implementação real e validada:

- One-Way ANOVA;
- Two-Way ANOVA;
- Repeated-Measures ANOVA;
- Welch's ANOVA;
- arquitetura e implementação madura para Linear Mixed-Effects Models quando a stack permitir;
- Tukey HSD;
- Dunnett para comparação de múltiplos tratamentos contra controle;
- correções de multiplicidade apropriadas;
- effect sizes;
- confidence intervals.

## 35.2 Integração com Cycles

Adicionar:

Cycle -> Analyze -> Statistical Analysis

O engine deve inspecionar:

- dependent variable;
- formulation/treatment/control;
- fatores;
- time points;
- replicates;
- Experimental Unit ID;
- repeated vs independent observations;
- missing data;
- balanced/unbalanced design;
- irregular temporal structure.

Exemplo:

3 formulações x 5 semanas x 3 replicatas

pode gerar recomendação:

Two-Way ANOVA - Formulation x Time

ou, se as mesmas unidades forem medidas ao longo do tempo:

Repeated-Measures ANOVA

ou Mixed-Effects quando mais adequado.

Nunca adivinhar silenciosamente se observações são independentes ou repetidas.

## 35.3 Experimental Unit ID

O Scientific Data Model deve representar explicitamente a unidade experimental.

Não use apenas o nome visual da amostra como identidade.

A análise longitudinal precisa saber se:

Unidade Sintética A / Week 1

e

Unidade Sintética A / Week 2

representam a mesma unidade experimental.

## 35.4 Assumption checks

Quando aplicável, oferecer:

- residual Q-Q plot;
- residual histogram;
- Shapiro-Wilk como evidência auxiliar;
- Levene ou alternativa robusta para homogeneidade;
- sphericity em repeated measures;
- Greenhouse-Geisser;
- Huynh-Feldt.

Não transformar um único teste de pressuposto em decisão automática absoluta.

## 35.5 Resultados

Mostrar, quando aplicável:

- analysis type;
- dependent variable;
- factors/levels;
- N;
- replicates;
- experimental units;
- SS;
- df;
- MS;
- F;
- p;
- effect size;
- confidence intervals;
- assumption diagnostics;
- corrections;
- post-hoc;
- adjusted p-values.

Nunca reportar `p = 0.000`.

Use `p < 0.001` quando apropriado.

Alpha padrão:

0.05

mas configurável e registrado na provenance.

## 35.6 Effect size

Implementar conforme método:

- eta squared;
- partial eta squared;
- omega squared;

e identificar explicitamente qual foi usado.

## 35.7 Interação

Em Two-Way ANOVA, interação é resultado de primeira classe.

Se Formulation x Time for relevante, não resumir conclusão apenas aos main effects.

Permitir Simple Effects, por exemplo:

- comparar formulações dentro da Week 4;
- comparar semanas dentro de uma formulação.

## 35.8 DLS

Não aplicar automaticamente ANOVA separada a cada bin da curva Diameter x Intensity.

Por padrão, análise inferencial deve priorizar parâmetros cientificamente interpretáveis, como:

- Effective Diameter;
- PDI;
- outros summary metrics válidos.

A curva completa permanece para visualização/comparação.

## 35.9 Missing data e outliers

Missing != 0.

Não imputar silenciosamente.

Não excluir outlier automaticamente.

Exclusão precisa ser explícita, registrada e manter o original intacto.

## 35.10 StatisticalAnalysis object

Persistir um objeto reproduzível contendo:

- analysis ID;
- source dataset/file IDs;
- worksheet/range quando aplicável;
- dependent variable;
- factors;
- levels;
- Experimental Unit IDs;
- repeated/independent structure;
- method;
- assumption diagnostics;
- corrections;
- alpha;
- post-hoc;
- results;
- effect sizes;
- confidence intervals;
- calculation/statistical engine version;
- timestamp;
- provenance.

Se source data mudar, marcar:

Source Changed

e permitir Recalculate sem sobrescrever silenciosamente a análise histórica.

## 35.11 Statistical library

Criar área pesquisável:

Statistical Analyses

integrada a My Stuff e aos objetos científicos.

Permitir:

- open;
- inspect;
- reproduce;
- duplicate;
- export;
- trace sources;
- inspect assumptions;
- inspect post-hoc;
- open related Graph/Cycle.

## 35.12 Graph integration

Graphs compatíveis podem mostrar:

- raw individual points;
- mean;
- SD;
- SEM;
- CI;
- significance annotations;
- exact p-values.

Sempre identificar o significado das error bars.

Evitar gráficos ilegíveis com dezenas de brackets.

## 35.13 Smart Export / Sign Export

Statistical Analysis deve suportar:

- Export Results;
- Export Tables;
- Export Graph;
- Export Full Analysis;
- XLSX;
- CSV;
- PDF;
- JSON quando apropriado;
- inclusion in Complete Research Package;
- Visible Signature;
- Digital Signature;
- Complete Signature.

## 35.14 Self-contained

O usuário não instala R, Python, SciPy, NumPy, SPSS, SAS, MATLAB, Origin, Prism, JASP ou Jamovi.

Se uma biblioteca/runtime estatístico for usado internamente, empacote-o segundo as regras self-contained do projeto.

Testar em clean machine.

## 35.15 Statistical validation

Comparar resultados críticos contra implementação estatística independente e confiável.

Criar synthetic golden datasets para:

- One-Way;
- Welch;
- Two-Way;
- interaction;
- repeated measures;
- unbalanced data;
- missing points;
- Tukey;
- Dunnett;
- effect size;
- confidence intervals;
- Mixed Effects quando implementado.

Statistical Analysis não está pronto sem validação numérica independente.

---

# 36. MULTI-SCIENTIFIC-MODULE ARCHITECTURE

O MNE Lab deve evoluir como:

ONE SCIENTIFIC PLATFORM
+
MULTIPLE INDEPENDENT SCIENTIFIC MODULES.

Estrutura conceitual:

MNE Lab
|
+-- LIGHTSCATTERING
|   +-- DLS / NanoBrook 90Plus
|
+-- ZETA
|   +-- Zetasizer Nano / potencial zeta
|
+-- NTA
|   +-- NanoSight / Nanoparticle Tracking Analysis
|
+-- Shared Scientific Core
    +-- Smart Import
    +-- File Library
    +-- Scientific Data Model
    +-- Provenance
    +-- Graph Engine
    +-- Cycle Engine
    +-- Statistical Analysis
    +-- Presentation Mode
    +-- Smart Export
    +-- Sign Export

Cada módulo deve possuir:

- parser próprio;
- scientific schema próprio;
- validators próprios;
- field definitions próprias;
- graph semantics próprios;
- scientific Help próprio;
- tests próprios.

Não copiar toda a infraestrutura para cada módulo.

Compartilhe infraestrutura; não misture ciência.

## 36.1 Organização interna recomendada

Estrutura plugin-like, por exemplo:

scientific/
    core/
    lightscattering/
    zeta/
    nta/

Cada módulo registra:

- module ID;
- supported instruments;
- format detectors;
- parsers;
- validators;
- scientific fields;
- graph types;
- analysis capabilities;
- Help content;
- export semantics.

O formato do arquivo não determina sozinho o módulo.

TXT pode carregar mais de uma ciência.

XLSX também.

Use:

format + content + structure + metadata.

Se o módulo for ambíguo, perguntar ao usuário em vez de adivinhar.

## 36.2 Shared Experiment

Permitir que um mesmo experimento possua dados de várias técnicas.

Exemplo:

Experiment: Formulation Stability

- DLS;
- Zeta;
- NTA.

Cada técnica mantém dataset e análise semanticamente separados.

Não executar automaticamente análise multivariada cross-module sem método explicitamente implementado.

---

# 37. LIGHTSCATTERING MODULE

O módulo inicial continua:

LIGHTSCATTERING
Dynamic Light Scattering - DLS
NanoBrook 90Plus / Brookhaven Instruments.

Continuam válidos:

- Effective Diameter;
- PDI / Polydispersity;
- Current Count Rate;
- BaseLine Index;
- Diameter / Particle Size;
- Intensity;
- distributions;
- replicates;
- temporal stability;
- File Library;
- Graph Library;
- Cycles;
- Statistical Analysis;
- Smart Export;
- Sign Export.

Não interromper nem degradar a implementação existente ao adicionar módulos novos.

---

# 38. ZETA MODULE

Criar módulo científico independente:

ZETA

com foco inicial no:

Zetasizer Nano
Malvern Panalytical.

Fundamentar implementação em:

1. documentação científica fornecida pelo proprietário;
2. documentação oficial Malvern;
3. exports reais do equipamento;
4. literatura científica confiável quando necessária.

Nunca derivar toda a ciência apenas olhando screenshots ou uma planilha.

## 38.1 Campos iniciais

Preparar o modelo para campos confirmados por documentação/export real, como:

- Zeta Potential (mV);
- Zeta Deviation;
- Conductivity;
- Electrophoretic Mobility;
- temperature;
- dispersant information;
- peak information;
- measurement quality;
- sample metadata;
- relevant instrument metadata.

Não inventar campo não documentado.

## 38.2 Provenance levels

Distinguir:

RAW / INSTRUMENT DATA
INSTRUMENT-DERIVED RESULT
MNE LAB DERIVED RESULT

Exemplo:

instrument export
-> electrophoretic mobility / instrument result
-> Zeta Potential
-> MNE Lab replicate statistics
-> Cycle
-> ANOVA/Mixed Effects
-> Export.

Nunca apresentar resultado derivado pelo MNE Lab como se tivesse sido calculado pelo instrumento.

## 38.3 Graphs

Suportar, conforme dados/documentação:

- Zeta Potential Distribution;
- Zeta Potential vs Time;
- Conductivity vs Time;
- formulation comparison;
- replicate comparison;
- mean +/- SD;
- confidence intervals;
- statistical annotations.

Escalas devem corresponder ao significado real dos dados.

## 38.4 Cycles / Statistics

Reutilizar o Cycle Engine e Statistical Analysis.

Exemplo:

Dependent Variable:
Zeta Potential

Factors:
Formulation x Time.

Suportar 1-99 Hours/Days/Weeks/Months/Years conforme regra global.

---

# 39. NTA MODULE

Criar módulo separado:

NTA
Nanoparticle Tracking Analysis

com foco inicial:

NanoSight / NanoSight Pro.

NTA != DLS != Zeta Potential.

Não reutilizar incorretamente algoritmos, parâmetros, units, assumptions ou gráficos.

## 39.1 Data model

Derivar os campos reais de:

- documentação oficial;
- exports reais;
- literatura quando necessária.

Possíveis categorias, somente se confirmadas:

- particle size;
- concentration;
- particle tracks;
- size distribution;
- sample metadata;
- acquisition conditions;
- frame rate;
- exposure;
- measurement conditions;
- replicate information.

## 39.2 NTA workflow

Arquitetura preparada para:

video/acquisition-derived results
-> particle tracking
-> size/concentration results
-> normalized scientific model
-> graphs
-> cycles
-> statistics
-> export/signature.

Não obrigar o MNE Lab a processar vídeo bruto se o escopo inicial for somente importar resultados exportados; decidir com base nos formatos reais e documentação.

---

# 40. OFFICIAL MALVERN TECHNICAL NOTE INCORPORATED

O arquivo incluído neste pacote:

02_SCIENTIFIC_REFERENCE/MALVERN/
tn101124_Zetasizer_Nano_Analysis_Methods_Explained_EN.pdf

é uma referência oficial de fabricante e deve ser estudado antes de implementar suporte a dados de tamanho provenientes do Zetasizer Nano.

Título:

ZETASIZER NANO ANALYSIS METHODS EXPLAINED
General Purpose Analysis, Multiple Narrow Mode Analysis and Protein Analysis Explained.

IMPORTANTE:

Este documento trata principalmente de ANÁLISE DE TAMANHO POR DLS no Zetasizer Nano.

Ele NÃO deve ser tratado como documentação suficiente para o algoritmo de potencial Zeta.

Use-o no domínio DLS/Zetasizer e no Shared DLS scientific logic quando apropriado.

## 40.1 Raw data -> size result

O documento explica que o software transforma:

measured raw data
=
auto correlation function

em:

size result.

O MNE Lab deve preservar a distinção entre:

- raw/instrument measurement;
- manufacturer-derived result;
- MNE-derived result.

## 40.2 Dois tipos de fit

O documento informa que, após obter a correlation function, dois tipos gerais de análise são tentados:

1. cumulants;
2. distribution analysis.

Representar isso na provenance quando esses campos existirem no export.

## 40.3 Cumulants

No cumulants analysis:

- a inclinação conduz ao cumulant / z-average size;
- a curvatura conduz ao second cumulant / polydispersity index;
- o método fornece average size + average sample homogeneity parameter.

Não reinterpretar PDI como mera largura de pico de uma distribution inversion.

Preservar o significado técnico do campo exportado.

## 40.4 Distribution analysis

A distribution analysis utiliza regularização/inversão e diferentes escolhas de smoothness/resolution.

Os três analysis methods descritos são:

- General Purpose (GP);
- Multiple Narrow Modes (MNM);
- Protein Analysis (PA).

O MNE Lab deve representar explicitamente o analysis method quando estiver presente ou confirmado.

Adicionar ao Scientific Data Model, quando aplicável:

analysisMethod:
    GENERAL_PURPOSE
    MULTIPLE_NARROW_MODES
    PROTEIN_ANALYSIS
    UNKNOWN

Nunca inventar o valor se o export não o informar e não houver evidência confiável.

## 40.5 Analysis method afeta distributions

O documento demonstra com dados simulados que o mesmo tipo de dado pode produzir distributions com diferentes:

- peak mean;
- peak width;
- relative width;

dependendo do analysis method.

Portanto:

ANALYSIS METHOD IS PROVENANCE-CRITICAL.

Nunca comparar silenciosamente duas distributions obtidas por diferentes analysis methods como se fossem diretamente equivalentes.

## 40.6 Comparability guard

Antes de comparação de distributions, verificar:

- instrument/source;
- analysis method;
- transformation;
- units;
- relevant analysis settings.

Se analysis methods diferirem:

mostrar warning claro.

Exemplo:

"These distributions were produced using different Zetasizer analysis methods and may not be directly comparable."

Permitir inspeção, mas não mascarar a incompatibilidade.

## 40.7 Cumulant vs distribution result

No exemplo simulado do documento, o resultado cumulant permaneceu igual entre GP/MNM/PA, enquanto as distributions mudaram.

Não assumir que qualquer diferença de distribution implica necessariamente diferença de z-average/PDI.

Guardar esses resultados como conceitos distintos.

## 40.8 Intensity / Volume / Number

O documento apresenta transformations:

- Intensity;
- Volume;
- Number.

As transformations podem produzir diferentes peak means/widths.

Portanto:

Intensity != Volume != Number.

Não misturar séries com transforms diferentes.

Nunca converter automaticamente entre elas sem método documentado e provenance explícita.

## 40.9 Recommendation rules from manufacturer note

Quando o contexto realmente corresponder ao documento:

- General Purpose: escolha padrão quando a forma da distribuição não é conhecida;
- Multiple Narrow Modes: apropriado quando se sabe que a amostra consiste em mistura de narrow peaks;
- Protein Analysis: destinado a amostras de proteína e resolução de small protein peak na presença de aggregates.

Estas são regras derivadas da nota técnica do fabricante.

Apresentar como recommendation/context, não como inferência universal para todo instrumento/toda amostra.

## 40.10 Relative-width guidance

O documento apresenta rough rules of thumb para intensidade em amostras homogêneas/monodispersas:

- GP: relative width < 25%;
- MNM: relative width < 15%;
- PA: relative width < 8%.

Esses números devem ser:

- tratados como manufacturer guidance;
- associados ao analysis method;
- associados à transformation de intensidade;
- não transformados em definição universal de estabilidade/monodispersidade;
- não aplicados a NanoBrook automaticamente;
- não usados para classificar qualquer dataset sem contexto.

Se exibidos em Help, citar a origem conceitualmente como manufacturer guidance.

## 40.11 Source version

Registre na scientific specification/provenance:

document:
tn101124zetasizernanoanalysismethodsexplained-EN.pdf

manufacturer:
Malvern Panalytical

document role:
manufacturer technical reference for Zetasizer Nano DLS analysis methods.

Registrar hash do arquivo presente no pacote e usar como source-version reference.

## 40.12 Tests derived from this document

Criar synthetic tests que comprovem:

- analysisMethod é preservado;
- GP/MNM/PA não são tratados como equivalentes;
- intensity/volume/number permanecem semanticamente distintos;
- comparison warns quando methods divergem;
- cumulant fields não são confundidos com distribution peak values;
- unknown method permanece UNKNOWN;
- no hardcoded conversion inventada;
- manufacturer guidance não é aplicada ao NanoBrook sem justificativa.

---

# 41. DOCUMENT AUTHORITY - REFINED

Não use uma hierarquia única e simplista para todos os tipos de informação.

Separe autoridade por função:

REAL INSTRUMENT EXPORT
-> autoridade sobre os valores e a estrutura efetivamente exportados naquele arquivo.

OFFICIAL MANUFACTURER DOCUMENTATION
-> autoridade primária sobre significado técnico de campos, opções do software e comportamento documentado do instrumento/software.

PROJECT SCIENTIFIC DOCUMENTATION
-> autoridade sobre comportamento desejado do MNE Lab e workflow científico do projeto, desde que não contradiga fatos instrumentais demonstrados.

SCIENTIFIC LITERATURE / STANDARDS
-> autoridade para metodologia científica geral, validação, interpretação e práticas aceitas.

MNE LAB IMPLEMENTATION
-> deve registrar decisões quando as fontes não forem suficientes.

Se fontes entrarem em conflito:

- não escolher silenciosamente;
- registrar conflito;
- investigar versão/modelo/software;
- preservar rastreabilidade da decisão.

---

# 42. DOCUMENTATION CATALOG

Este pacote contém:

07_DOCUMENTATION_CATALOG/

com índice de:

- NanoBrook manuals;
- Particle Solutions references;
- Zetasizer Nano manuals/technical notes;
- NanoSight/NTA materials;
- ISO references;
- real-data Drive reference.

Alguns documentos são:

- public direct PDF;
- public web content;
- official login required;
- paid standards.

Não copiar conteúdo protegido para o repositório público sem licença.

Use links/references legalmente permitidos.

---

# 43. IMPLEMENTATION ORDER - UPDATED

A ordem recomendada passa a ser:

1. preservar o estado funcional atual;
2. ler o prompt mestre inteiro;
3. analisar as referências científicas fornecidas;
4. registrar hash/version de cada documento incorporado;
5. manter LIGHTSCATTERING funcional;
6. consolidar Shared Scientific Core;
7. implementar Statistical Analysis Engine;
8. integrar Analyze aos Cycles;
9. validar ANOVA/Mixed Effects com synthetic golden datasets e implementação independente;
10. incorporar as regras da nota oficial `tn101124...pdf` ao modelo DLS/Zetasizer;
11. criar analysisMethod provenance/compatibility guards;
12. implementar ZETA como módulo científico separado;
13. validar ZETA com documentação oficial + planilha/export real;
14. integrar ZETA a Graphs/Cycles/Statistics/Export/Sign;
15. pesquisar/validar documentação e exports reais NanoSight;
16. implementar NTA como módulo separado;
17. integrar NTA ao Shared Scientific Core;
18. atualizar Help e Feature Discovery;
19. executar clean-machine/offline/Portable/Temporary tests;
20. executar scientific/privacy/security/visual/release audits;
21. atualizar context document.

Não sacrificar o módulo existente para construir os novos.

---

# 44. DEFINITION OF DONE - MULTI-MODULE SCIENCE

Um novo módulo científico não está pronto porque "abre um arquivo".

Só está pronto quando possui:

- scientific scope documentado;
- supported instrument/source;
- validated format detection;
- validated parser;
- original preservation;
- normalized scientific model;
- provenance;
- graph semantics;
- cycle support quando aplicável;
- statistical support quando aplicável;
- Smart Export;
- Sign Export;
- File/Graph/Statistical Libraries integration;
- offline;
- profile/install isolation;
- clean-machine operation;
- synthetic tests;
- private real-data validation quando disponível;
- independent scientific validation;
- Help;
- polished UI.

FINAL PRINCIPLE:

SHARE INFRASTRUCTURE.
DO NOT MIX SCIENCE.
PRESERVE METHOD CONTEXT.
MAKE EVERY RESULT TRACEABLE.

<!-- END_MASTER_REQUIREMENTS_BODY -->
