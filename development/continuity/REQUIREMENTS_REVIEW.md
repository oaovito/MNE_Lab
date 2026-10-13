<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Revisão dos requisitos e evidências

Esta matriz cobre as **44 seções principais** de
[MASTER_SPECIFICATION.md](MASTER_SPECIFICATION.md), incluindo suas subseções.
Ela é ponto de partida para uma revisão detalhada, não certificado de conclusão.
Leia também [USER_DIRECTIVES.md](USER_DIRECTIVES.md): instruções posteriores
restringem requisitos históricos e autorizam esta ponte de desenvolvimento no
repositório, sem colocá-la no produto.

O código funcional de referência é
`7985a57ff835699f2643687b2a80bbceb9b5e779`. A
[CI registrada](https://github.com/oaovito/MNE_Lab/actions/runs/38091538654)
passou nos quatro jobs dessa revisão. Commits documentais posteriores não
constituem nova execução de testes funcionais. Conferir o Git atual antes de
reutilizar essas provas em código modificado.

## Significado dos estados

- **PARTIAL:** há implementação ou documentação relevante, mas a seção inteira
  ainda não tem prova de conformidade.
- **REVIEW_REQUIRED:** o requisito precisa ser confrontado com implementação,
  testes, sistemas e comportamento atual antes de concluir.
- **BLOCKED_EVIDENCE:** falta documentação/arquivo/referência científica adequada;
  infraestrutura independente pode continuar.
- **HISTORICAL_OVERRIDE:** preservar o requisito histórico e aplicar a instrução
  posterior explícita; não repetir uma inicialização superada.

Nenhuma linha declara a seção inteira DONE. Statistical Analysis continua
IMPLEMENTED / TESTED / VALIDATED **dentro do escopo documentado**; sua seção
inclui outros requisitos, como assinatura e clean-machine, ainda pendentes.
DLS permanece PARTIAL / PROVISIONAL. ZETA e NTA mantêm validação científica
pendente. Arquivo aberto, gráfico, checksum e assinatura não validam ciência.

## Cobertura das seções

| SECTION | STATUS / EVIDENCE | GAP | NEXT ACTION |
|---|---|---|---|
| 1. IDENTIDADE E REPOSITÓRIO | HISTORICAL_OVERRIDE; identidade em README e repositório existente; [contexto](../../docs/TECHNICAL_CONTEXT.md) | Criação inicial foi superada; histórico de `main` é separado e contém material privado anterior | Continuar a branch de desenvolvimento; conferir situação Git sem modificar `main` ou publicar refs privadas |
| 2. USO DE REFERÊNCIAS E FERRAMENTAS | PARTIAL; source registry, lockfiles e notices de dependências | Toda nova dependência/fonte ainda exige revisão de licença, versão e adequação | Verificar proveniência e licença antes de incorporar código/dados; registrar a decisão |
| 3. AUTONOMIA | REVIEW_REQUIRED; roadmap e checkpoints disponíveis | Continuidade operacional não é demonstrada por uma etapa concluída; não há scheduler testado | Executar trabalho autorizado em etapas verificáveis, com próxima ação e blockers claros |
| 4. SEM MARCAS DE IA NO PRODUTO | PARTIAL; auditoria de artefatos e instrução posterior autoriza documentação de desenvolvimento | Exclusão de arquivos de coordenação do produto e três verificações independentes precisam ser mantidas | Auditar código distribuído, Git/artefatos e pacotes; não confundir coordenação autorizada com branding do produto |
| 5. DESIGN E UX | PARTIAL; design system, ícones, temas, layouts e browser E2E | Exigências completas de DPI, scaling, estados, tray e clean desktop não estão todas comprovadas | Revisar cada cláusula com screenshots/ações reais e registrar lacunas, sem chamar placeholder de pronto |
| 6. LOCALIZAÇÃO | PARTIAL; EN/PT-BR/ES e testes de catálogos/i18n | Cobertura completa de novas funções, tray, export e todos os estados precisa revisão por mudança | Conferir textos/IDs/placeholders, idioma do dispositivo e feedback em todos os fluxos |
| 7. ACCOUNT E PROFILES | PARTIAL; vault/store, APIs scoped, limite de cinco Profiles e isolamento | Revisão integral de namespaces, preferences, credenciais e transições ainda necessária | Confirmar todas as combinações Account/Profile e limpeza de estado ao trocar contexto |
| 8. TURBO | PARTIAL; preferences e restart com handoff/rollback testados | Requisitos UX e comportamento em desktop limpo precisam revisão | Preservar Save/Persist/Preserve/Restart/Restore e verificar Turbo sem alterar resultados |
| 9. MODOS DE EXECUÇÃO | PARTIAL; Portable/Temporary, lifecycle e recovery | Clean-system/USB, startup independente e limpeza em cenários reais permanecem pendentes | Executar gates de distribuição e confirmar preservação da única cópia válida |
| 10. RECOVERY, WRITES E BACKUP | PARTIAL; writes atômicos, snapshots verificados, recovery por conta/perfil | Matriz completa de crash/power loss/disk/USB/cloud e gerações exige revisão | Confrontar cada falha com testes e prova de recovery; não apagar trabalho não confirmado |
| 11. SYNC E ISOLAMENTO ENTRE INSTALAÇÕES | PARTIAL; queue/idempotência/conflicts/readback e testes locais | APIs reais e todas as combinações entre instalações ainda não demonstradas | Verificar namespace estável; folder copy não é confirmação remota; excluir private keys de sync |
| 12. PRIVACIDADE | PARTIAL; arquitetura client↔provider, cifragem e logs locais | Providers reais, features novas e distribuição precisam auditoria contínua | Conferir least privilege, secrets, dados/recovery e ausência de servidor central/telemetria |
| 13. CLOUD PROVIDERS | PARTIAL; adapters Google/iCloud/OneDrive e folder transport | Registrations e interoperabilidade real; provider migration completa precisa revisão | Validar serviços oficiais após configuração segura, preservando origem até destino confirmado |
| 14. MOBILE | PARTIAL; QR/pairing, viewer/controller e testes de telefone | Rede/sistemas reais, expiração/invalidation e estados completos requerem revisão | Confirmar escopo/replay/CSRF e encerramento por troca de perfil/exit |
| 15. UPDATES E LOCKEDBUILD | PARTIAL; assinatura de manifests, staging, schema, rollback e key safety | Identidade oficial, canal Stable, migrations e clean-system ainda pendentes | Preparar release somente depois dos gates; testar ativação/rollback/compatibilidade |
| 16. DISCOVERABILITY / USER AWARENESS | PARTIAL; feature registry, Help, tour, hints, palette e What's New | Divulgação de cada nova capacidade e razão de indisponibilidade precisa revisão integral | Conferir Help/palette/onboarding por idioma, sem esconder limites de suporte |
| 17. DOCUMENTAÇÃO CIENTÍFICA OFICIAL | BLOCKED_EVIDENCE / HISTORICAL_OVERRIDE; [regras](../../docs/SCIENTIFIC_RULES.md), [DLS](../../docs/DLS_VALIDATION.md), [fontes](../../docs/LIGHTSCATTERING_SOURCE_REGISTRY.md) | Documentação do projeto não substitui semântica instrumental; antiga autorização somente DLS foi ampliada com limites | Investigar conflitos e campos/units/versioning; aplicar status ZETA/NTA atuais |
| 18. CORPUS REAL DE DESENVOLVIMENTO | PARTIAL; 54 relatórios privados comparados literalmente; [materiais](MATERIALS.md) | Dados não são incluídos no repo; falta referência científica pareada para várias interpretações | Usar corpus privado autorizado e fixtures sintéticos, preservando hashes/originais fora da publicação |
| 19. PRIMEIRO MÓDULO: LIGHTSCATTERING | PARTIAL; parser `1.4.0`, spec provisional, Library/Graph/Cycle | Definições exatas de fields/units/acquisition/métodos e pares instrumentais ausentes | Concluir evidência de DLS sem converter missing em zero ou unit/weighting por suposição |
| 20. GRAPH LIBRARY E GRAPH ENGINE | PARTIAL; definição reproduzível e renderer compartilhado | Revisão integral de interações, comparability, limites e acessibilidade ainda necessária | Conferir cada tipo de graph contra dados/provenance e preservação de valores/métodos |
| 21. CYCLES | PARTIAL; plano/associação, dados individuais/statistics e testes de duplicate/timezone | Auditar intervalo 1–99, calendar semantics, gaps e todos os modos/sistemas | Confrontar clauses com testes e fluxos reais; não inferir unidade experimental |
| 22. PRESENTATION MODE | PARTIAL; fullscreen, navegação e mobile controller | Revisão de todos os inputs, múltiplos gráficos e dispositivos reais | Verificar keyboard/mouse/touch/remote sem alterar dados científicos |
| 23. DATA INGESTION MULTIFORMATO | PARTIAL; [Import Review](../../docs/IMPORT_REVIEW.md), CSV/TSV/XLSX e ODS literal | XLS, scientific ODS, distribution mapping e extensões condicionais; Google Sheets/API requer trabalho próprio | Implementar readers bounded com contracts/versioning e evidência suficiente; preservar original/recipe/review |
| 24. SMART EXPORT | PARTIAL; figure/data/package formats e publication safety testados | Revisar presets/settings/batch/history; formatos condicionais como EPS/Parquet não são suporte existente | Confrontar opções prometidas e compatibilidade; manter Recommended/All Compatible e final preview fiel |
| 25. SIGN EXPORT — SISTEMA UNIFICADO | PARTIAL; [CMS](../../docs/CMS_SIGNATURES.md), backend detached e global Verify | PFX/P12 bounded, Profile identity, Visible/Digital/Complete, PDF/PAdES, trust/revocation/timestamps | Priorizar controle de recursos e secrets; só depois integrar Smart Export e protocolos independentes |
| 26. DEPENDENCY INDEPENDENCE / SELF-CONTAINED RUNTIME | PARTIAL; runtime estatístico offline e ciência/export sem runtimes externos | Navegador instalado ainda necessário; bootstrap/clean desktop/offline USB e Corresponding Source incompletos | Executar plano de distribuição legal/reprodutível e gates em máquinas limpas |
| 27. DATA MODEL, PROVENANCE E AUDITABILIDADE | PARTIAL; model, parser/spec, source snapshots, producer origin e exports | Revisão de todo derived result e adapters futuros; authenticity não deriva de declaration | Preservar raw/units/time/source/recipe; testar snapshots independentes e dependency deletion |
| 28. PERFORMANCE E ACCESSIBILITY | REVIEW_REQUIRED; Turbo e layouts/screen sizes testados | Perfil de CPU/RAM/idle, grandes bibliotecas, scaling e accessibility completa não comprovados | Medir cenários representativos e verificar teclado/focus/reduced motion/cancelamento |
| 29. LOGGING E SECURITY | PARTIAL; redaction/retention, dependency/security/privacy audits | Features futuras e scripts de distribuição precisam mesma proteção | Auditar logs/artifacts sem expor matched secrets; manter versões e bounds atualizados |
| 30. TESTES E RELEASE GATE | PARTIAL; quatro jobs CI funcionais e suites nativas/frontend/browser | Assinatura completa, formatos adicionais, ciência pendente e clean-machine gates não concluídos | Executar checks pertinentes às mudanças e cada gate de release com revisão/escopo registrado |
| 31. CONTEXT DOCUMENT | PARTIAL; [contexto](../../docs/TECHNICAL_CONTEXT.md) e [estado](../../docs/DEVELOPMENT_STATUS.md) | Contexto deve acompanhar novos commits e distinguir provas antigas/atuais | Atualizar status/decisões/blockers sem inserir coordenação na UI ou runtime |
| 32. DEFINIÇÃO DE PRONTO | REVIEW_REQUIRED; [readiness](../../docs/READINESS.md) lista gates | LIGHTSCATTERING/ingestion/signing completos ainda não satisfeitos | Auditar todos os critérios; checkpoint pronto não equivale a produto pronto |
| 33. PRINCÍPIOS ABSOLUTOS | REVIEW_REQUIRED; princípios preservados integralmente no mestre | Necessário verificar cada caminho novo contra integridade, isolamento e ciência | Aplicar review de regressões sem enfraquecer princípios para passar testes |
| 34. ORDEM INICIAL DE IMPLEMENTAÇÃO | HISTORICAL_OVERRIDE; etapas originais preservadas | Recriação inicial foi superada; várias fases posteriores continuam abertas | Retomar estado comprovado e usar ordem atual, sem refazer trabalho ou descartar história |
| 35. STATISTICAL ANALYSIS ENGINE - ANOVA / MIXED EFFECTS | PARTIAL; [escopo numérico validado](../../docs/STATISTICAL_ANALYSIS.md), oráculos independentes e worker offline | Seção inteira inclui assinaturas/clean-machine, métodos/post-hoc/effect CIs adicionais e UX a revisar | Preservar 35.1–35.15; rastrear limites de cada método e completar tarefas independentes sem prometer designs não suportados |
| 36. MULTI-SCIENTIFIC-MODULE ARCHITECTURE | PARTIAL; registry/ingest/model compartilhados e guards de module | Shared Experiment completo, capabilities/Help/views e adapters científicos adicionais precisam revisão | Consolidar infraestrutura sem copiar engines ou herdar validação entre técnicas |
| 37. LIGHTSCATTERING MODULE | PARTIAL; módulo existente integrado | Ciência provisional e signing completo pendente | Preservar funcionalidades e regressões existentes ao adicionar módulos |
| 38. ZETA MODULE | BLOCKED_EVIDENCE; IMPLEMENTATION ALLOWED, estrutura privada revisada | SCIENTIFIC VALIDATION PENDING; docs/exports/reference de potential/mobility/quality e distributions ausentes | Implementar somente o sustentado; marcar PARTIAL/UNVALIDATED e fechar blockers específicos |
| 39. NTA MODULE | BLOCKED_EVIDENCE; ARCHITECTURE / PREPARATION ALLOWED | SCIENTIFIC VALIDATION PENDING; sem corpus real e contrato oficial suficiente | Preparar infraestrutura e obter schema/settings/resultados; não inventar tracking/calibration/units |
| 40. OFFICIAL MALVERN TECHNICAL NOTE INCORPORATED | PARTIAL; referência DLS/size consultada e limits registrados | Método Zetasizer e comparability guards completos dependem de exports e schema; não equivale a NanoBrook/Zeta | Preservar 40.1–40.12, GP/MNM/PA e weighting; implementar apenas quando presente/confirmado e comparar com referência |
| 41. DOCUMENT AUTHORITY - REFINED | PARTIAL; research matrix registra papel da fonte/conflicts | Conflitos G/C/métodos/versões permanecem unresolved | Investigar por função/version/model e registrar decisão; não harmonizar silenciosamente |
| 42. DOCUMENTATION CATALOG | PARTIAL; registry com links oficiais públicos e materiais externos | Corpus link privado e PDFs protegidos não são redistribuídos; algumas fontes não obtidas | Usar referências legais, distinguir catálogo/snippet de manual consultado e de fixture científico |
| 43. IMPLEMENTATION ORDER - UPDATED | PARTIAL / HISTORICAL_OVERRIDE; roadmap preserva DLS e independent work | Workbook não basta para ZETA VALIDATED; recursos dependentes de evidência não concluídos | Aplicar diretrizes científicas posteriores e registrar ordem/próxima ação sem bloquear módulos independentes |
| 44. DEFINITION OF DONE - MULTI-MODULE SCIENCE | REVIEW_REQUIRED; todos os critérios mantidos no mestre | Nenhum novo módulo cumpre integralmente todos os critérios só por abrir arquivo | Auditar scope/parser/provenance/graphs/cycles/statistics/sign/offline/isolation/clean-machine/tests/Help/UI e ciência independente |

## Como ampliar a rastreabilidade sem mascarar lacunas

Cada cláusula relevante deve ganhar um identificador estável, por exemplo
`MNE-35.13-SIGN`, ligado ao título/subseção original. Acrescentar arquivo/função,
teste, revisão executada, resultado e blocker. Não alterar o requisito porque
o produto implementa um subconjunto; registrar o subconjunto e a pendência.

Os pedidos complementares de Statistical Analysis, módulos, DTS e pesquisa de
formatos continuam aplicáveis. Sua síntese está em
[USER_DIRECTIVES.md](USER_DIRECTIVES.md), e os materiais/alcances em
[MATERIALS.md](MATERIALS.md). Identificar lacunas dessa síntese durante a revisão
não autoriza inventar detalhes de conversa ou dados privados ausentes.

Provas herdadas devem manter o SHA e seu escopo. Um novo teste de interoperabilidade
não promove validação científica; um teste científico de método não prova uma
instalação desktop limpa. Trabalho de desenvolvimento e intervenção do proprietário
devem continuar separados em [READINESS.md](../../docs/READINESS.md).
