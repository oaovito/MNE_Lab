<!-- MNE_LAB_DEVELOPMENT_ONLY -->
# Diretrizes atuais do proprietário

Consolidação das instruções explícitas posteriores ao mestre. Não é uma
transcrição completa. Mestre e diretrizes devem ser lidos antes de alterar
arquivos; instruções posteriores prevalecem nos conflitos.

## Continuidade e autorização

- Continuar o MNE Lab existente e o estado atual do GitHub. Preservar o trabalho
  histórico comprovado; não criar outro projeto ou recomeçar do zero.
- A instrução de 2026-10-13 autoriza colocar o contexto necessário no repositório
  como ponte entre IAs, sem integrar o produto final. `CLAUDE.md`, `AGENTS.md` e
  `development/continuity` são desenvolvimento e devem ser excluídos da distribuição.
- A regra antiga de coordenação exclusivamente privada está substituída apenas
  para esses documentos. Dados científicos reais, arquivos pessoais, credenciais,
  transcrições e referências Git privadas continuam privados.
- §1/§34 do mestre descrevem a criação inicial; agora preservar este projeto.
  §4/§31 mantêm o produto e o contexto técnico neutro sem branding desnecessário.
- Revisar o já implementado contra requisitos, código, testes e evidências.
  Documentação não comprova atendimento integral de todos os requisitos.

## Autonomia, Git e passagem de trabalho

- Trabalhar em etapas verificáveis e continuar o implementável além de etapas
  intermediárias. Não transferir ao dono trabalho técnico que o agente pode fazer.
- Nunca modificar `main` diretamente, fazer force-push ou reescrever histórico.
  Integração/remediação do histórico privado necessita escopo separado.
- Não aplicar automaticamente stash, patches ou propostas históricas; não
  publicar refs privadas ou usar `push --all`/mirror. Preservar o trabalho anterior.
- Manter checkpoints, decisões, tarefas, testes/escopo, intervenções do proprietário
  e próxima ação exata nos registros compartilhados desta pasta.
- Sem API OpenAI paga e sem Kiro/AWS. Objetivos persistentes/agendamento nativo
  somente se disponíveis e testados dentro da assinatura. Não prometer reinício
  após quota sem mecanismo efetivo; preservar checkpoint antes do limite.

## Status científico obrigatório

| FEATURE | STATUS |
|---|---|
| STATISTICAL ANALYSIS | IMPLEMENTED / TESTED / VALIDATED no escopo documentado |
| LIGHTSCATTERING / DLS | PARTIAL / PROVISIONAL; continuar implementação/validação |
| ZETA | IMPLEMENTATION ALLOWED / SCIENTIFIC VALIDATION PENDING |
| NTA | ARCHITECTURE / PREPARATION ALLOWED / SCIENTIFIC VALIDATION PENDING |

Essas decisões atualizam §17/§38/§39/§43: a antiga restrição somente DLS não
proíbe infraestrutura segura ZETA/NTA; ciência só avança com evidências adequadas.

1. Não inventar campos, unidades, algoritmos ou regras.
2. Marcar funcionalidades parcialmente sustentadas PARTIAL / UNVALIDATED.
3. Workbook Zeta = corpus privado; DTS = investigação privada de interoperabilidade.
4. “Zetasizer Nano Analysis Methods Explained” é referência DLS/size, não potencial Zeta.
5. VALIDATED exige documentação adequada + resultado reproduzível + comparação
   confiável, com escopo/tolerâncias. Arquivo/gráfico/checksum/CMS não valida ciência.
6. Manter matriz FEATURE | IMPLEMENTED | TESTED | SCIENTIFICALLY VALIDATED | BLOCKER
   e blockers exatos ZETA/NTA. Workbook/export isolado não basta.
7. Compartilhar framework/storage/provenance/Library/Graph/Cycle/Statistics;
   manter schemas, métodos e semântica científicos por módulo.
8. Antes de novos parsers LIGHTSCATTERING, pesquisar fontes oficiais externas,
   revisar corpus/documentos, diferenciar versões/modelos/configurações e
   registrar fontes, confiança, conflitos e comparações.
9. `producerOrigin` é declaração não verificada, distinta de transporte/parser;
   não comprova autenticidade nativa.
10. Não bloquear módulos e trabalhos independentes pela falta de ciência ZETA/NTA.

## Design

- 2026-10-15: “A base e cores complementares da aplicação nessa ordem devem ser:
  #F4F8FA #2C7FB8 e #143A52”. Base #F4F8FA, destaque #2C7FB8 e destaque escuro
  #143A52. Complementa §5 do mestre; contraste/acessibilidade de §5/§28 continuam
  obrigatórios, então tons derivados do mesmo azul são usados onde a cor exata
  não atinge WCAG AA. Cores de figuras científicas não fazem parte da paleta da UI.

## Entrega

- O proprietário quer executável Windows acessível para teste. Draft `8a12ff6`
  é anterior a CMS/producer origin; GUI/USB real pendentes. Não alegar build atual/Stable.
- Distinguir o trabalho técnico executável pelo agente de documentos, máquinas
  reais, registros oficiais/licenças/decisões que exigem intervenção do dono.
- Ao interromper/entregar, listar pendências e próxima ação exata. Uma ponte
  pronta não significa aplicação terminada.
- Material privado ou histórico ausente deve ser declarado, nunca fabricado.
  Isso não bloqueia revisão inicial ou tarefas independentes sustentadas.
