<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Entrada do Claude Code

Leia [AGENTS.md](AGENTS.md), a instrução comum aos agentes. Antes de editar,
leia integralmente [o mestre](development/continuity/MASTER_SPECIFICATION.md)
e [as diretrizes atuais](development/continuity/USER_DIRECTIVES.md).
Revise a implementação antes de continuar; não reinicie o projeto.

Siga [development/continuity/README.md](development/continuity/README.md).
O contexto, checkpoint, tarefas, provas e materiais disponíveis estão nessa
pasta. [PROMPT.md](development/continuity/PROMPT.md) contém o prompt reutilizável.
Os caminhos são relativos ao clone; não exige outro serviço ou workspace para
a revisão inicial. Material privado ausente é um blocker das tarefas dependentes.

Atualize os mesmos registros ao passar trabalho para outra sessão/ferramenta.
A ponte está no repositório por instrução explícita do proprietário, mas não
faz parte do produto. Preserve `export-ignore`, marcadores e gates.
Corpus, credenciais e refs históricas privadas continuam fora dessa ponte.

Nunca altere `main` diretamente. Não confunda teste de software com validação
científica ou conclusão de checkpoint com produto pronto/Stable.
