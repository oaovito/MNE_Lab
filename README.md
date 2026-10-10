# MNE Lab

Análise de espalhamento dinâmico de luz (DLS) do arquivo do NanoBrook 90Plus
à figura pronta para publicação. Privado por arquitetura, roda direto de um
pendrive e foi feito para computadores modestos.

## Por que o MNE Lab

- **Execução local.** Um executável nativo inclui o runtime estatístico offline.
  A janela usa o navegador Chromium instalado no computador (Microsoft Edge
  no Windows). O modo Turbo corta efeitos para máquinas mais antigas.
- **Privado por arquitetura.** Não existe servidor central nem telemetria. As
  contas e os dados ficam no pendrive e na nuvem da própria pessoa. Cada perfil
  é cifrado com a sua própria chave, e a frase secreta nunca sai do computador.
- **Rigor científico.** O arquivo original é guardado sem alterações, cada
  valor lido mostra a unidade e a origem, um campo ausente continua ausente e
  datas ambíguas nunca são adivinhadas. O que aparece na tela é exatamente o
  que é exportado.
- **Vai com você.** No Modo USB portátil, aplicativo e trabalho vivem no
  pendrive. No Modo máquina temporária, nada fica no computador depois de
  Salvar, limpar e sair.

## O que ele faz

- **LIGHTSCATTERING:** importação das exportações do NanoBrook 90Plus (texto,
  CSV, TSV e planilhas XLSX com valores medidos, com vírgula ou ponto decimal, em UTF-8, UTF-16 ou nas
  codificações antigas do Windows), revisão de cada medição, gráficos de distribuição e de parâmetros no tempo, comparação entre
  medições e ciclos de estabilidade com estatísticas.
- **Exportação inteligente:** figuras em PNG, SVG, PDF, TIFF, JPEG e WebP,
  dados em XLSX, CSV, TSV, TXT e JSON, e um pacote de pesquisa em ZIP com
  arquivos originais, somas de verificação e proveniência. Nada é sobrescrito
  sem aviso.
- **Apresentação:** modo de tela cheia, com o celular como controle remoto e
  visualizador, pareado por QR code e criptografado na rede local.
- **Perfis e armazenamento:** até cinco perfis protegidos por conta, cada um
  em USB + nuvem, Somente USB ou Somente nuvem, com Google Drive / Google One,
  iCloud ou Microsoft OneDrive. Funciona sem internet e sincroniza depois.
- **Segurança do trabalho:** backups conferidos depois de gravados,
  recuperação de trabalho interrompido e conflitos de sincronização que nunca
  descartam uma versão em silêncio.
- **Atualizações:** versões Stable assinadas, aplicadas em um momento seguro,
  com retorno automático à build anterior se algo falhar. No pendrive, o
  LockedBuild mantém a build escolhida.
- **Idiomas:** português (Brasil), inglês e espanhol, seguindo o idioma do
  sistema.

## Requisitos

- Windows 10 ou 11 (x64, x86 ou ARM64) com Microsoft Edge (já vem no
  Windows), Google Chrome, Brave ou Chromium.
- Linux (x64 ou ARM64) com Microsoft Edge, Google Chrome, Chromium ou Brave.
- macOS (Apple Silicon ou Intel) com Google Chrome, Microsoft Edge, Brave ou
  Chromium.

## Estado do projeto

Versão 0.1.0, em desenvolvimento. O parser segue a lista de campos da
especificação do produto (`ls-spec/0.3-provisional`); a validação com o
documento científico oficial e com exportações reais do NanoBrook 90Plus vem
antes da primeira versão Stable. Os arquivos de teste do repositório são
sintéticos e estão identificados como tal.

Planilhas XLSX são lidas diretamente pelo aplicativo, sem Excel ou ferramentas
de conversão. A origem de cada medição inclui a planilha e as linhas do arquivo.
Fórmulas e macros não são executadas nem interpretadas como medições; use uma
cópia com os valores medidos. ODS 1.3 tem prévia literal somente leitura, preservando
texto e valor declarado; mapeamento/importação científica ODS e leitura XLS seguem
pendentes. Tabelas incompletas
ou com várias distribuições sem separação de medições são sinalizadas para revisão,
sem gerar curvas de um trecho arbitrário. Relatórios nativos com alternativas
Lognormal e Multimodal permitem revisar os valores e escolher a distribuição.
O método fica registrado nos gráficos salvos.

## Compilação

Requisitos: Go 1.26.9 ou uma versão posterior com as correções de segurança atuais, e Node.js 22.

```sh
cd web && npm ci && npm run build && cd ..
scripts/build.sh
```

O `scripts/build.sh` gera em `out/` o aplicativo e o inicializador do Modo USB
portátil para Windows (x64, x86 e ARM64) e Linux (x64 e ARM64), com
`-trimpath`. As builds de Windows levam ícone, informações de versão e
manifesto, e abrem sem janela de console. As de macOS precisam de cgo (ícone
na bandeja) e são feitas em um Mac com `scripts/build.sh darwin/arm64
darwin/amd64`. `VERSION` e `CHANNEL` definem a versão gravada nos
executáveis.

As credenciais dos provedores de nuvem e a chave de assinatura das versões
nunca ficam no repositório: as builds oficiais as recebem na publicação (veja
o [contexto técnico](docs/TECHNICAL_CONTEXT.md)).

## Verificações

```sh
cd web && npm run typecheck && npm test && cd ..
gofmt -l . && go vet ./... && go test -race ./...
scripts/e2e.sh
```

O `scripts/e2e.sh` abre todas as telas em português, inglês e espanhol, nos
temas claro e escuro, em 1366×768 e 1024×600, nos modos USB portátil e
máquina temporária, e percorre o fluxo do celular. Ele falha com erros de
script, textos cortados, chaves sem tradução ou uma página que rola, e guarda
as capturas de tela em `out/e2e/`. Precisa do Chromium
(`cd web && npx playwright-core install chromium`). O CI roda tudo isso a cada
push, mais auditoria de dependências e de segredos.

## Estrutura

| Caminho | Conteúdo |
|---|---|
| `cmd/mnelab` | O aplicativo. |
| `cmd/mnelab-launcher` | Inicializador das instalações no pendrive. |
| `cmd/mnelab-release` | Assinatura e manifesto das versões. |
| `internal/` | Núcleo: armazenamento cifrado, sincronização, provedores, atualizações, ciência e exportação. |
| `web/` | Interface do computador e do celular (Preact e TypeScript). |
| `web/tests/e2e` | Testes no navegador. |
| `scripts/` | Compilação, testes no navegador e auditoria do repositório. |
| `assets/brand` | Ícones e marcas do MNE Lab. |
| `testdata/` | Arquivos de teste sintéticos. |

## Documentação

- [Contexto técnico](docs/TECHNICAL_CONTEXT.md) (em inglês): arquitetura,
  decisões, chaves e criptografia, sincronização, atualizações, estado de cada
  parte e próximos passos.
- [Avisos de terceiros](THIRD_PARTY_NOTICES.md) (em inglês).

made by oaovito
