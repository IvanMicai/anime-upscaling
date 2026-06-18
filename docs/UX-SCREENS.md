# AnimeUp — Mapa de Telas (para redesign de UX)

Documento descritivo de **todas as telas** da dashboard web do AnimeUp, pensado
para um(a) profissional de UX/UI redesenhar a interface. Cada seção descreve:
**objetivo da tela**, **quando o usuário chega nela**, **quais informações ela
mostra** e **quais ações estão disponíveis**.

> Contexto do produto: AnimeUp é uma dashboard self-hosted (roda em servidor
> local/home server) para processar vídeos de anime em lote — **upscaling**
> (aumento de resolução por IA), **interpolação de frames** (aumento de FPS),
> **otimização/re-encode** e **verificação de integridade**. O usuário coloca
> arquivos numa pasta, dispara "jobs" pelo navegador e o servidor executa o
> processamento (video2x + FFmpeg) com aceleração por GPU.
>
> Perfil do usuário: técnico/entusiasta que administra a própria coleção de
> mídia. Uso tipicamente em desktop, mas o layout atual já é responsivo (há
> breakpoints para mobile). Tema **dark** fixo.
>
> Idioma: a interface hoje é **mista** (mistura português e inglês). Padronizar
> o idioma é uma oportunidade de melhoria.

---

## Índice de telas

| # | Tela | Rota | Função principal |
|---|------|------|------------------|
| 0 | Shell / Navegação global | (em todas) | Cabeçalho + menu + logout |
| 1 | Login | `/login` | Autenticação por senha |
| 2 | Jobs (Dashboard / Home) | `/` | Lista e monitora todos os jobs |
| 3 | Criar Job | `/jobs/new` | Formulário para disparar um processamento |
| 4 | Detalhe do Job | `/jobs/[id]` | Progresso e logs ao vivo de um job |
| 5 | Pipelines (lista) | `/pipelines` | Gerencia/executa pipelines salvos |
| 6 | Novo Pipeline | `/pipelines/new` | Monta um pipeline (sequência de etapas) |
| 7 | Editar Pipeline | `/pipelines/[id]/edit` | Edita um pipeline existente |
| 8 | Files (Explorador) | `/files` | Navega/inspeciona/baixa/exclui arquivos |
| 9 | Settings | `/settings` | Concorrência de GPU/FFmpeg |

---

## 0. Shell / Navegação global

**Onde aparece:** envolve **todas** as telas autenticadas (não aparece no
Login).

**Objetivo:** dar identidade, orientação e navegação consistentes.

**Informações e elementos:**
- **Logo + título "AnimeUp"** (clicável → leva para a Home/Jobs) e subtítulo
  "Video Processing Dashboard".
- **Menu de navegação** com 4 links:
  - **Jobs** (Home, `/`) — fica ativo também nas rotas `/jobs/*`.
  - **Pipelines** (`/pipelines`).
  - **Files** (`/files`).
  - **Settings** (`/settings`).
- **Botão Logout** (encerra a sessão).
- Container central com largura máxima (~1024px), conteúdo centralizado.

**Observações para UX:** não há indicação de qual servidor/host está conectado,
nem status global do sistema (ex.: GPU saudável, fila ativa). O link ativo é
destacado, mas a hierarquia entre logo, navegação e ações é simples.

---

## 1. Login (`/login`)

**Objetivo:** proteger a dashboard com uma senha única (gate de acesso).

**Quando o usuário chega:** ao acessar sem sessão válida.

**Informações e elementos:**
- Card centralizado na tela.
- Título **"AnimeUp"**.
- **Campo de senha** (único input, com autofocus).
- **Botão "Log in"** (mostra "Logging in..." enquanto processa).
- **Mensagem de erro** em vermelho ("Wrong password" / "Something went wrong").

**Fluxo:** ao acertar a senha, redireciona para a Home (`/`).

**Observações para UX:** tela minimalista, sem usuário/login (só senha), sem
"lembrar-me", sem branding/ilustração. Não há recuperação de senha (é uma senha
única do servidor).

---

## 2. Jobs — Dashboard / Home (`/`)

**Objetivo:** ser a tela inicial e o **centro de monitoramento** — lista todos
os jobs de processamento (em fila, rodando, concluídos, falhos, cancelados) e
permite criar um novo.

**Atualização:** a lista faz **polling automático a cada 3 segundos** (status e
progresso mudam ao vivo, sem recarregar).

**Informações e elementos:**
- Título de seção **"Jobs"** + botão **"New Job"** (→ `/jobs/new`).
- Mensagem de erro se a API falhar ("Failed to load jobs: …").
- **Tabela de jobs** (ordenada do mais recente para o mais antigo) com colunas:
  - **Type** — tipo do job (`upscale`, `interpolate`, `optimize`, `check`) ou,
    para pipelines, o **nome do pipeline**. Exibido como "badge".
  - **Status** — badge colorido: Queued (roxo), Running (azul), Completed
    (verde), Failed (vermelho), Cancelled (amarelo).
  - **Files** — quantidade de arquivos no job (ex.: "12 files"). *(oculta em
    telas pequenas)*
  - **Progress** — percentual + contagem, ex.: "75% (9/12)". *(oculta em telas
    pequenas)*
  - **ETA** — estimativa de tempo restante (só para jobs "running"), ex.:
    "~12m 30s". *(oculta em telas médias)*
  - **Created** — data/hora de criação. *(oculta em telas pequenas)*
  - **Ações** — link **"View"** (→ detalhe do job) e ícone de **lixeira**
    (remover/cancelar+remover, com confirmação).
- **Estado vazio:** quando não há jobs, mostra uma área tracejada com a mensagem
  "No jobs yet. Create one to get started."

**Ações principais:**
- Criar novo job.
- Abrir detalhe de um job.
- Remover um job (se estiver rodando/na fila, pede confirmação "Cancel this
  running job and remove it?"; usa o `confirm()` nativo do navegador).

**Observações para UX:** a tela depende muito de uma tabela densa; em mobile
várias colunas somem, restando praticamente Type/Status/ações. Não há filtros,
busca, paginação nem agrupamento por status — pode crescer indefinidamente. A
confirmação de exclusão usa diálogo nativo do browser (inconsistente com os
diálogos customizados usados em outras telas).

---

## 3. Criar Job (`/jobs/new`)

**Objetivo:** configurar e disparar um processamento sobre um conjunto de
arquivos. É um formulário **dinâmico**: os campos mudam conforme o tipo
escolhido. Também permite **executar um pipeline salvo** a partir daqui.

**Quando o usuário chega:** pelo botão "New Job" na Home.

**Estrutura da tela (de cima para baixo):**
1. Link **"← Back to Jobs"**.
2. Título **"Create Job"**.
3. **Type (seletor)** — escolhe o que fazer:
   - `Upscale`, `Interpolate`, `Optimize`, `Check`
   - e, em um grupo separado **"Saved Pipelines"**, qualquer pipeline já criado.
   - Ao escolher um pipeline, os campos de configuração somem (a config vem do
     pipeline) e só restam origem + seleção de arquivos.
4. **Pasta de origem** — de onde vêm os arquivos: Input / Output (upscaled) /
   Interpolated / Optimized. *(permite encadear: ex. otimizar o que já foi
   upscalado.)*
5. **Campos específicos do tipo** (ver abaixo).
6. **Files** — o seletor de arquivos (componente FilePicker, descrito na seção
   8b).
7. **Ações fixas no rodapé:**
   - **"Run All"** — processa todos os arquivos da pasta/origem.
   - **"Run Selected (N)"** — processa só os marcados (desabilitado se nenhum).

**Campos por tipo:**

- **Upscale (aumento de resolução):**
  - *Processador* — RealESRGAN (IA, melhor p/ anime) · libplacebo (shaders
    Anime4K) · RealCUGAN (IA p/ ilustrações). Cada opção tem descrição inline.
  - *Modelo* — depende do processador (ex.: "Anime Video v3", shaders Anime4K
    A/B/C, etc.), com descrição.
  - *Scale* — 2x / 3x / 4x (opções válidas variam por modelo).
  - *Redução de Ruído* — Desativado / Baixo / Médio / Alto.

- **Interpolate (aumento de FPS):**
  - *Multiplicador* — 2x / 3x / 4x (dobra/triplica/quadruplica o framerate).
  - *Modelo RIFE* — várias versões (v4.6 recomendado … legados).
  - *Detecção de Cena* — Alta(5) / Média(10) / Baixa(20) / Desativada(100).

- **Optimize (re-encode/compressão):** formulário mais complexo, com campos que
  aparecem/somem conforme escolhas:
  - *Codec de Vídeo* — H.265/HEVC · H.264/AVC · VP9 · Copiar stream (sem
    re-encode).
  - *Acelerar com GPU* — checkbox (só aparece para codecs elegíveis; só habilita
    se houver um "GPU vendor" configurado em Settings). Avisa que compete por
    slots com o upscale.
  - *Qualidade* — Ultra(CRF16) / Alta(19) / Média(22) / Baixa(26).
  - *Preset* — ultrafast … veryslow (some quando GPU ligada ou VP9).
  - *Tune* — Animação / Filme / Grão / Zero Latência / Nenhum (mesmas regras de
    visibilidade do preset).
  - *Formato de Pixel* — 10-bit / 8-bit / 4:4:4.
  - *Codec de Áudio* — Copiar / AAC / Opus / MP3.
  - *Resolução* — Original / 1/2 / 1/4 (downscale).
  - *Frame Rate* — modo Relativo (Original / 1/2 / 1/4) ou Absoluto (digita fps).
  - *Threads* — Auto / 1 / 2 / 4 / 8 / 16 / 32 (some quando GPU ligada).
  - Com "Copiar stream" quase todos os campos somem (não há re-encode).

- **Check (verificação de integridade):** não tem campos específicos — só
  origem + seleção de arquivos.

**Observações para UX:** é a tela mais densa e "config-heavy" do produto. Os
campos do Optimize aparecem/desaparecem condicionalmente (lógica complexa de
visibilidade). O seletor de arquivos divide o espaço vertical com o formulário
numa coluna alta (a tela ocupa toda a altura da viewport). Não há **preview do
resultado** (diferente do builder de pipeline, que mostra resolução/FPS/tamanho
estimado). Não há validação visível além do estado dos botões.

---

## 4. Detalhe do Job (`/jobs/[id]`)

**Objetivo:** acompanhar **um job específico** em tempo real — status, progresso
detalhado e logs ao vivo.

**Atualização:** polling do job a cada **2s**; logs via **stream ao vivo**
(SSE/streaming) com indicador "streaming"/"disconnected".

**Estrutura da tela:**
1. Link **"← Back to Jobs"**.
2. **Cabeçalho do Job (card):**
   - **ID do job** (em fonte mono).
   - **Badge de tipo** (ou nome do pipeline) + **badge de status**.
   - **Botões:** "Cancel" (só se running/queued) e "Remove".
   - **Datas:** Created e Finished.
3. **Barra de progresso:**
   - Barra segmentada por cor: verde (concluídos), vermelho (falhas), amarelo
     (pulados).
   - Resumo textual: "9/12 — 8 ok / 1 err / 0 skip".
   - **Progresso por stream/container** (quando há vários workers ao vivo): para
     cada GPU/FFMPEG mostra nome do arquivo, fase, frame atual/total + %, FPS,
     tempo decorrido e **ETA**.
4. **Visualizador de Logs:**
   - **Abas de filtro** por fonte: "All" + cada fonte (ex.: "GPU 0", "FFMPEG",
     "PIPELINE").
   - Indicador "streaming/disconnected" + contagem de linhas.
   - Lista monoespaçada com colunas: horário, badge de fonte (colorida),
     **nível** (INFO/OK/ERRO/SKIP/WARN/STEP, coloridos), passo [N/M] quando
     houver, e a mensagem.
   - **Auto-scroll** para a última linha.

**Ações principais:** cancelar job, remover job (com confirmação nativa),
filtrar logs por fonte.

**Observações para UX:** é uma tela de "observabilidade". O bloco de logs é
longo (300–500px) e técnico. A barra de progresso por container é rica mas
textual/monoespaçada — há espaço para visualização mais clara. Erro de carga
mostra só "Error: …" com link de voltar.

---

## 5. Pipelines — Lista (`/pipelines`)

**Objetivo:** gerenciar **pipelines salvos** — receitas reutilizáveis que
encadeiam várias etapas (ex.: Upscale 2x → Interpolate 2x → Optimize) para rodar
de uma vez só.

**Atualização:** polling a cada 5s.

**Informações e elementos:**
- Título **"Pipelines"** + botão **"Novo Pipeline"** (→ `/pipelines/new`).
- **Estado vazio:** área tracejada "Nenhum pipeline criado" + botão "Criar
  primeiro pipeline".
- **Lista de cards**, um por pipeline, cada um com:
  - **Nome** do pipeline.
  - **Resumo das etapas** (ex.: "Upscale 2x (RealESRGAN) → Optimize (Alta,
    H.265)").
  - **Transformação esperada**: estado inicial → final (ex.: "1080p 24fps →
    2160p 24fps (H.265)").
  - **Estimativa de tamanho** do resultado (ex.: "~1.2 GB (24min) · ~50 MB/min").
  - **Ações:** botão **"Executar"** (abre diálogo de execução), ícone **lápis**
    (editar) e ícone **lixeira** (excluir, sem confirmação extra).
- **Diálogo "Executar: <nome>"** (modal):
  - Seletor de **Pasta de origem**.
  - **Seletor de arquivos** (FilePicker).
  - Botões **"Run All"** e **"Run Selected (N)"** → cria o job e redireciona para
    a Home.

**Observações para UX:** os cards comunicam bem o "antes → depois" e o tamanho
estimado (ótimo diferencial). A exclusão é imediata (sem confirmação) — risco de
clique acidental. O diálogo de execução reaproveita o mesmo seletor de arquivos
da criação de job.

---

## 6. Novo Pipeline (`/pipelines/new`) e 7. Editar Pipeline (`/pipelines/[id]/edit`)

> As duas telas usam o **mesmo construtor** (PipelineBuilder). A diferença é só o
> título ("Novo Pipeline" vs "Editar Pipeline") e se vem pré-preenchido.

**Objetivo:** montar visualmente uma sequência de etapas de processamento e
salvar como pipeline reutilizável, vendo o impacto de cada etapa.

**Estrutura da tela (fluxo vertical, estilo "esteira"):**
1. Link **"← Voltar para Pipelines"** + título.
2. **Nome** do pipeline (input de texto, obrigatório).
3. **Card "Input"** (ponto de partida fixo): "1080p · 24fps".
4. **Cards de etapa** encadeados, ligados por **setas "↓"**. Cada etapa
   (StepCard) traz:
   - **Número da ordem** + seletor do tipo de operação (Upscale / Interpolate /
     Optimize).
   - **Controles para mover** (▲/▼) e **remover** (✕).
   - Os **mesmos campos** do formulário de Criar Job, conforme a operação
     (upscale: processador/modelo/scale/ruído; interpolate:
     multiplicador/RIFE/cena; optimize: codec/qualidade/preset/etc.).
   - **Rodapé da etapa:** estado resultante **após** aquela etapa (ex.: "→ 2160p
     · 24fps (H.265)") + estimativa de tamanho.
5. **Botões para adicionar etapa:** "+ Upscale", "+ Interpolate", "+ Optimize".
6. **Preview "Resultado"** (card de destaque): estado inicial → final + tamanho
   estimado total. (Se não há etapas: "Adicione steps para ver o preview".)
7. Mensagem de erro (ex.: "Nome é obrigatório", "Adicione pelo menos um step").
8. Botão **"Salvar Pipeline"** / **"Atualizar Pipeline"**.

**Ações principais:** nomear, adicionar/remover/reordenar etapas, configurar cada
etapa, salvar.

**Observações para UX:** é a tela mais "visual" do produto — a metáfora de
esteira com preview incremental por etapa é forte e deve ser preservada/melhorada
no redesign. A reordenação é por botões ▲/▼ (não há drag-and-drop). Cada card de
etapa é alto por causa da quantidade de campos.

---

## 8. Files — Explorador (`/files`)

**Objetivo:** navegar, inspecionar, **baixar** e **excluir** os arquivos de vídeo
gerenciados pelo sistema, organizados por estágio de processamento.

**Conceito central — 4 "pastas"/estágios** (cada uma com cor própria):
- **Input** (amarelo) — fontes originais.
- **Upscaling/Output** (azul) — resultado do upscale.
- **Interpolated** (roxo) — resultado da interpolação.
- **Optimized** (verde) — resultado do re-encode.

**Estrutura da tela:**
1. Link **"← Back to Jobs"**.
2. **Abas** das 4 pastas (troca o diretório-base).
3. **Breadcrumbs** (navegação de subpastas — coleções com temporadas, etc.).
4. **Barra de filtros/legenda** — botões-pílula coloridos (Input/Upscaling/
   Interpolated/Optimized) que filtram quais arquivos aparecem; à direita,
   indicador **"Cached … ago" + botão "Refresh"** (a listagem é cacheada).
5. **Botão "Delete Mode"** (ativa modo de exclusão).
6. **Tabela de arquivos:**
   - Linhas de **subpastas** (clicáveis, com ícone de pasta) — em Files mostram
     também os tamanhos agregados por estágio.
   - Linhas de **arquivos**: nome (mono, truncado) e, por estágio, **tamanho +
     resolução** (ex.: "1.4G | 1080p"). Estágios inexistentes mostram "—".
   - **Tooltip ao passar o mouse** em cada célula: tamanho exato, resolução,
     framerate, faixas de **áudio** e **legendas**.
   - **Botão de download** por célula (ícone, baixa aquele estágio do arquivo).
   - **Rodapé "Total"**: soma por estágio + contagem de arquivos.
7. **Modo de Exclusão (Delete Mode):**
   - Barra vermelha de resumo ("X input, Y upscaled, …" ou "Select files to
     delete").
   - Cada célula vira **selecionável** (destaque vermelho). É possível excluir
     estágios específicos de um arquivo (ex.: apagar só o "input", manter o
     otimizado).
   - Botões **"Clear"** e **"Delete selected"**.
   - **Diálogo de confirmação**: lista os arquivos por estágio com tamanhos e
     total geral, antes de excluir permanentemente.

**Ações principais:** trocar de estágio, navegar subpastas, filtrar, atualizar
cache, baixar arquivos, excluir (com confirmação detalhada).

**Observações para UX:** tela rica e poderosa, mas densa — muita informação por
linha (4 estágios × tamanho/resolução) e dois "modos" (normal vs delete) com
comportamentos de clique diferentes. As colunas de estágio somem em telas
pequenas (mobile fica limitado a nome). A relação "mesmo arquivo em 4 estágios" é
o modelo mental central e precisa ficar muito claro no redesign.

### 8b. FilePicker (componente reutilizado em Criar Job e ao Executar Pipeline)

Variante do explorador focada em **selecionar** arquivos (não em gerenciar):
- Mesma estrutura (breadcrumbs, legenda/filtros, tabela por estágios, cache/
  refresh, e até um Delete Mode embutido).
- Acrescenta **checkboxes** por linha + **"Select All (N files, tamanho)"**, com
  suporte a **seleção por shift-click** (intervalo).
- A seleção sobrevive à navegação entre subpastas (usa caminho relativo).

---

## 9. Settings (`/settings`)

**Objetivo:** ajustar a **concorrência** do processamento — quanto trabalho roda
em paralelo no servidor.

**Quando o usuário chega:** pelo menu "Settings".

**Informações e elementos (um card "Concorrência"):**
- Texto explicativo + **nº de GPUs detectadas** ("GPUs detectadas: N").
- Aviso: mudanças só aplicam quando **não há jobs em execução**.
- **Streams por GPU** (1–8) — quantos processos video2x por GPU.
- **GPU vendor (encode ffmpeg)** — Nenhum (CPU) / NVIDIA (NVENC) / AMD (AMF) /
  Intel (QSV). Habilita o toggle "Usar GPU" nos jobs de Optimize.
- **Streams de FFmpeg** (1–8) — quantos encodes FFmpeg em paralelo.
- Cada campo tem **texto de ajuda** explicando o trade-off.
- Mensagens de **erro** (vermelho) / **sucesso** ("Settings aplicadas com
  sucesso", verde).
- Botão **"Salvar"** (só habilita quando há mudança não salva — estado "dirty").
- Estado de carregamento: "Carregando...".

**Observações para UX:** é uma tela simples de configuração de servidor, voltada
a usuário técnico (termos como NVENC/QSV/streams). Tudo num único card; não há
seções/abas para crescer (ex.: caminhos de pastas, notificações, conta). Texto de
ajuda é bom, mas denso.

---

## Padrões transversais (úteis para o redesign)

- **Cores semânticas consistentes:** amarelo=Input, azul=Upscaling/Output,
  roxo=Interpolated, verde=Optimized; e status: roxo=Queued, azul=Running,
  verde=Completed, vermelho=Failed, amarelo=Cancelled. Manter essa linguagem
  cromática ajuda a leitura.
- **Tempo real:** Home (3s), Detalhe do Job (2s + stream), Pipelines (5s).
- **Estados:** a maioria das telas trata loading / erro / vazio (estados vazios
  têm CTA). Bom ponto de partida para um design system.
- **Inconsistências a resolver:**
  - **Idioma misto** (PT + EN) — ex.: "New Job"/"Create Job" vs "Novo Pipeline".
  - **Confirmações** mistas: diálogo nativo do browser (`confirm`/`alert` em
    jobs) vs diálogos customizados (exclusão de arquivos). A exclusão de
    pipeline não tem confirmação.
  - **Ausência de preview de resultado** na criação de Job avulso, embora exista
    no builder de pipeline.
  - **Sem busca/filtros/paginação** na lista de Jobs.
- **Densidade:** Criar Job, Files e os Step Cards são as telas mais densas e as
  que mais ganham com hierarquia visual, agrupamento e responsividade.
- **Navegação "voltar":** várias telas usam um link textual "← Back/Voltar" em
  vez de um padrão consistente (breadcrumb/topo).
</content>
</invoke>
