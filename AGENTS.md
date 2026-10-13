<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Instruções comuns aos agentes de desenvolvimento

Antes de qualquer alteração, leia integralmente
[MASTER_SPECIFICATION.md](development/continuity/MASTER_SPECIFICATION.md) e
[USER_DIRECTIVES.md](development/continuity/USER_DIRECTIVES.md). As diretrizes
posteriores prevalecem em conflitos com regras históricas do mestre.

Siga a [ordem de retomada](development/continuity/README.md). Revise requisitos,
código, testes, evidências e Git atuais antes de implementar. Continue o projeto
existente; preserve arquitetura, trabalho anterior e correções comprovadas.

Confira `CONTEXT.md`, `CHECKPOINT.json`, `TASKS.json` e `VALIDATION.json` em
`development/continuity/`. Atualize esses registros a cada checkpoint e antes
de interromper por limite de uso. Registre decisões, escopo dos testes, blockers,
intervenções do proprietário e próxima ação exata.

Nunca modifique `main` diretamente, faça force-push ou publique refs privadas.
Não aplique stash/patches históricos automaticamente. Preserve originais,
provenance, receitas, snapshots e isolamento Account/Profile.

Finalize o build frontend antes de Go tests/build e browser harnesses; não
reconstrua `web/dist` enquanto consumidores executam. Faça checks proporcionais,
preserve exit status e não desative gates para obter sucesso.

STATISTICAL ANALYSIS validado somente no escopo documentado. DLS permanece
PARTIAL / PROVISIONAL; ZETA/NTA SCIENTIFIC VALIDATION PENDING. Não invente ciência.
Corpus real, credenciais, transcrições e refs históricas privadas ficam fora do Git.

Esta ponte está no Git por instrução do proprietário, exclusivamente para
desenvolvimento. Preserve marcadores, `export-ignore` e auditorias: ponte,
`CLAUDE.md` e `AGENTS.md` não podem entrar no produto, UI, executável ou source ZIP
de distribuição. Use clone da branch para obter o contexto.

Continue o implementável sem parar em etapas intermediárias. Não use API OpenAI
paga ou prometa retomada após quota sem mecanismo disponível e testado. Um
checkpoint concluído não significa aplicação pronta ou Stable.
