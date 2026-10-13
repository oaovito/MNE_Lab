<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Prompt de continuidade reutilizável

Todos os caminhos abaixo são relativos ao clone da branch de desenvolvimento.

```text
Continue o MNE Lab existente, oaovito/MNE_Lab; não recomece do zero.

ANTES DE ALTERAR ARQUIVOS, leia integralmente
development/continuity/MASTER_SPECIFICATION.md e considere sempre suas diretrizes.
Depois leia USER_DIRECTIVES.md da mesma pasta: instruções explícitas posteriores
prevalecem nos conflitos com regras históricas do mestre.

Leia AGENTS.md, CLAUDE.md e development/continuity/{README.md,CONTEXT.md,
CHECKPOINT.json,TASKS.json,VALIDATION.json,REQUIREMENTS_REVIEW.md,MATERIALS.md}.
Depois revise docs/DEVELOPMENT_STATUS.md, docs/TECHNICAL_CONTEXT.md, docs/READINESS.md,
os guias especializados indicados e o código/testes/CI reais. Revise o que já
foi feito antes de decidir mudanças; documentação não comprova execução.

Confira branch/HEAD/tree/dirty state/GitHub. Continue na branch
codex/mne-lab-checkpoint-20261008 ou outra branch de desenvolvimento autorizada.
Nunca alterar main diretamente, fazer force-push, aplicar stash/patches antigos
ou publicar refs privadas. Preserve arquitetura, originais/hash/provenance,
receitas, snapshots e isolamento Account/Profile.

A ponte é desenvolvimento no Git, excluída do produto. Preserve export-ignore,
marcadores e gates; não incluir contexto em UI/executável/installer/source ZIP.
Não copiar corpus real, PDFs protegidos, credenciais, chaves ou transcrições
privadas para Git. Registre material ausente sem fabricar valores/histórico.

STATISTICAL ANALYSIS permanece IMPLEMENTED/TESTED/VALIDATED no escopo documentado;
DLS PARTIAL/PROVISIONAL; ZETA IMPLEMENTATION ALLOWED e NTA ARCHITECTURE/PREPARATION
ALLOWED, ambos SCIENTIFIC VALIDATION PENDING. Não inventar ciência ou promover
validação porque arquivo/gráfico/checksum/CMS funciona. Atualizar matriz/blockers.

Após revisar, siga TASKS.json: recursos/isolamento PFX/P12 antes de Smart Export.
CMS/global Verify já existem parcialmente. PKCS12 candidato não é dependência
de produção. Não expor DecodeChain bruto ao HTTP; cobrir KDFs aninhados, custo,
cancelamento, secrets/isolamento e OpenSSL interop antes de API/UI. Assinar bytes
finais após overlay. CONTEXT contém findings reproduzíveis e limites da prova.

Continue tarefas independentes com contratos/evidências suficientes: adapters,
pesquisa DLS, bootstrap/distribuição, source/licenças e providers oficiais.
Não bloquear todo o desenvolvimento por ZETA/NTA. Finalize build frontend antes
de Go tests/build/harness. Verificar proporcionalmente, preservar exit status
e não desativar assertions/gates. Identificar revisão/escopo e limitações;
build não comprova clean-machine/USB, CI antigo não prova SHA novo.

Registre revisão inicial/divergências e plano curto; depois implemente trabalho
autorizado sem parar no planejamento. Atualize contexto/checkpoint/tarefas/provas
e docs a cada etapa, commit/push normal e readback, main intacta. Continue o
implementável sem parar em etapas intermediárias. Ao entregar, liste exatamente
o que exige intervenção do proprietário e a próxima ação exata.

Sem API OpenAI paga; não prometer reinício após quota sem mecanismo disponível
e testado. Antes do limite, preservar checkpoint. Checkpoint pronto não equivale
a produto pronto/Stable. Comece pela revisão e prossiga.
```
