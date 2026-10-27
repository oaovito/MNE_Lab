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

`synthetic-nanobrook-run-table.txt` traz o diâmetro efetivo sem unidade na
linha de resultados e a unidade só no cabeçalho da tabela de corridas, com
valores inventados, para verificar que a unidade vem do próprio arquivo e
somente quando a linha Combined repete o mesmo valor.

`synthetic-nanobrook-twelve-runs.txt` traz uma única medição com uma tabela
de doze corridas, com valores inventados, para verificar que as corridas são
guardadas como leituras do equipamento, exatamente como escritas, e que
continuam sendo uma medição de uma unidade experimental (nunca doze N).

`synthetic-twenty-series.txt` traz vinte medições inventadas (amostras
"Synthetic S01" a "Synthetic S20") num único arquivo, cada uma com uma
distribuição de intensidade, para testar o Series Explorer com muitas
séries: busca, filtros, seleção de três, gráfico, edição da seleção e que
as outras dezessete continuam intactas.
