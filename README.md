# MNE Lab

O MNE Lab é um aplicativo de laboratório para computador que organiza,
analisa e apresenta medições de LIGHTSCATTERING (exportações do NanoBrook
90Plus). Ele foi feito para rodar bem em computadores baratos: um único
executável nativo e pequeno, sem navegador embutido e com pouco uso de
memória.

## O que ele faz

- Até cinco perfis protegidos por conta, cada um com a sua forma de guardar:
  **USB + nuvem**, **Somente USB** ou **Somente nuvem**.
- Sincronização com a nuvem da própria pessoa: Google Drive / Google One,
  iCloud ou Microsoft OneDrive.
- **Modo USB portátil** (tudo fica no pendrive) e **Modo máquina temporária**
  (nada fica no computador depois de Salvar, limpar e sair).
- Biblioteca LIGHTSCATTERING: importação, gráficos, ciclos de estabilidade,
  estatísticas e apresentação.
- Exportação inteligente: figuras (PNG, SVG, PDF, TIFF, JPEG, WebP), dados
  (XLSX, CSV, TSV, TXT, JSON) e pacotes de pesquisa (ZIP), com proveniência.
- Acesso pelo celular com QR code: visualizador e controle da apresentação.
- Atualizações automáticas, LockedBuild e o seletor de builds Stable.
- Interface em português (Brasil), inglês e espanhol, seguindo o idioma do
  aparelho.

## Como compilar

Requisitos: Go 1.26 e Node.js 22.

```sh
cd web
npm ci
npm run build
cd ..
scripts/build.sh
```

O `scripts/build.sh` gera o aplicativo e o inicializador do Modo USB portátil
para Windows (x64, x86, ARM64) e Linux (x64, ARM64) em `out/`. As builds de
Windows têm o ícone e as informações de versão do MNE Lab e abrem sem janela
de console. As builds de macOS precisam de cgo (ícone na bandeja), então são
feitas em um Mac com `scripts/build.sh darwin/arm64 darwin/amd64`. `VERSION`
e `CHANNEL` definem a versão gravada nos executáveis.

As credenciais dos provedores de nuvem não ficam no repositório. As builds de
release as recebem por `-ldflags -X`; veja
[docs/TECHNICAL_CONTEXT.md](docs/TECHNICAL_CONTEXT.md).

## Verificações

```sh
cd web && npm run typecheck && npm test && cd ..
gofmt -l . && go vet ./... && go test -race ./...
```

Testes no navegador (precisam do Chromium: `cd web && npx playwright-core install chromium`):

```sh
scripts/e2e.sh
```

Eles abrem todas as telas em português, inglês e espanhol, nos dois temas e
em dois tamanhos de janela, e o fluxo do celular, e salvam as capturas de tela
em `out/e2e/`.

## Estrutura

| Caminho | Conteúdo |
|---|---|
| `cmd/mnelab` | O aplicativo. |
| `cmd/mnelab-launcher` | Inicializador das instalações no pendrive. |
| `cmd/mnelab-release` | Ferramenta de assinatura e manifesto das releases. |
| `internal/` | Pacotes do núcleo (armazenamento, sincronização, provedores, atualizações, ciência, exportação). |
| `web/` | Interface do computador e do celular. |
| `web/tests/e2e` | Testes no navegador. |
| `scripts/` | Compilação, testes no navegador e auditoria do repositório. |
| `assets/brand` | Ícones e marcas do MNE Lab. |
| `testdata/` | Arquivos de teste sintéticos. |

## Mais

- [Contexto técnico](docs/TECHNICAL_CONTEXT.md) (em inglês): arquitetura,
  decisões, situação atual e próximos passos.
- [Avisos de terceiros](THIRD_PARTY_NOTICES.md) (em inglês).

made by oaovito
