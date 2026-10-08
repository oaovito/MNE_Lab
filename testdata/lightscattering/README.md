# Arquivos de teste de LIGHTSCATTERING

Estes arquivos são **sintéticos**. Eles foram escritos para testar como o
parser trata os campos citados na especificação científica (Effective
Diameter, Polydispersity, Current Count Rate, Average Count Rate, BaseLine Index, Diameter,
Intensity, Volume, Number), delimitadores, separadores decimais,
codificações, campos ausentes e entradas malformadas.

Eles não são exportações de um NanoBrook 90Plus. A fixture
`synthetic-nanobrook-methods.txt` reproduz a estrutura de alternativas
Lognormal/Multimodal e os grupos de colunas, com valores inventados.
As exportações reais e documentos de referência ficam fora do checkout
público e são usados somente em validações privadas. Veja
`docs/TECHNICAL_CONTEXT.md`, seções 3 e 17.

`synthetic-count-rate-types.txt` contém leituras inventadas de contagem
atual e média, com unidades e precisões diferentes, para verificar que
as duas grandezas permanecem independentes.
