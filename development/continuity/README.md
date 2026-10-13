<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Ponte persistente entre agentes

Esta pasta atende à instrução de 2026-10-13: contexto de continuidade no Git,
para Claude Code, Codex ou outra ferramenta revezarem o desenvolvimento.
É documentação de trabalho, excluída do produto. Corpus/credenciais permanecem privados.

## Ordem de leitura antes de editar

1. [MASTER_SPECIFICATION.md](MASTER_SPECIFICATION.md), inteiro: 44 seções do mestre.
2. [USER_DIRECTIVES.md](USER_DIRECTIVES.md): decisões posteriores/precedência.
3. [CONTEXT.md](CONTEXT.md), [CHECKPOINT.json](CHECKPOINT.json),
   [TASKS.json](TASKS.json) e [VALIDATION.json](VALIDATION.json).
4. [REQUIREMENTS_REVIEW.md](REQUIREMENTS_REVIEW.md) e [MATERIALS.md](MATERIALS.md).
5. [Estado técnico](../../docs/DEVELOPMENT_STATUS.md),
   [contexto técnico](../../docs/TECHNICAL_CONTEXT.md),
   [readiness](../../docs/READINESS.md) e os guias específicos do módulo.
6. Código, fixtures, testes, histórico e CI atuais. Revise divergências;
   não considere texto como prova de execução ou conformidade integral.

[AGENTS.md](../../AGENTS.md) é a entrada comum;
[CLAUDE.md](../../CLAUDE.md) usa os mesmos registros.
[PROMPT.md](PROMPT.md) é reutilizável numa nova sessão/ferramenta.

O clone basta para entender requisitos, estado e próxima ação. Arquivos privados
continuam necessários às tarefas dependentes, identificadas em MATERIALS.md;
sua ausência não bloqueia revisão ou trabalho independente.

## Primeira ação

Branch: `codex/mne-lab-checkpoint-20261008`. Confira HEAD real, estado e remoto:

```sh
git status --short
git branch --show-current
git log -1 --format='%H %s'
git ls-remote origin refs/heads/main refs/heads/codex/mne-lab-checkpoint-20261008
```

Se o fetchspec trouxer só `main`, busque explicitamente a branch nominal.
Não force checkout/reset sobre mudanças. Nunca altere `main` diretamente,
aplique stash histórico ou publique refs privadas automaticamente.

Revise os requisitos e a implementação, registre divergências e prossiga pela
próxima ação em TASKS.json: recursos/isolamento PFX/P12 antes de Smart Export.
CMS/backend e global Verify já existem; não reimplementar por ler log antigo.

## Checkpoint para a próxima sessão

- CONTEXT: mudança, decisão, escopo e regressões.
- TASKS: completed/in-progress/blocked, aceitação e próxima ação exata.
- CHECKPOINT: data, bases, arquivos e estado de trabalho; obter HEAD real pelo Git.
- VALIDATION: checks realmente executados, revisão/escopo, passes/fails/skips e limites.
- REQUIREMENTS_REVIEW e docs técnicos: requisitos/evidências/gaps atualizados.
- Links, segredo/corpus e fronteira da distribuição revisados; commit/push normais
  e readback conferidos, `main` intacta.

Não transferir testes de SHA antigo a novo, nem declarar ciência validada por
abertura/plot. Registre a próxima ação antes de atingir um limite de uso.

## Distribuição

`.gitattributes` aplica `export-ignore` à ponte e às duas entradas. Go embute
somente `web/dist`; frontend usa entradas explícitas. Gates de distribuição
rejeitam contexto também em ZIPs aninhados e arquivos renomeados.

**Use `git clone` ou GitHub para ler a ponte. Download ZIP/source ZIP pode omiti-la
por causa de `export-ignore`.** Preserve marcadores e verificações; não copiar
contexto para UI/assets/default DB. Não colocar segredos, PFX, PDFs protegidos,
corpus, transcrições ou bundle histórico nesta pasta.

As provas da fronteira estão em VALIDATION.json. Stable segue os gates em
READINESS. Um arquivo de tarefas não é executor de retomada após quota.

A permissão de coordenação não isenta termos confidenciais: secrets, corpus,
regexes personalizadas e histórico continuam examinados. A exceção de branding
no Git é limitada a padrões literais conhecidos de ferramentas; os mesmos
padrões permanecem ativos em metadados e artefatos do produto.
