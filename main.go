package main

import (
	"NostrFilePublisher/blossom"
	"NostrFilePublisher/icons"
	"NostrFilePublisher/model"
	"NostrFilePublisher/util"
	"context"
	"fmt"
	"github.com/bbrks/go-blurhash"
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/minio/sha256-simd"
	urlX "net/url"
)

var App *model.AppState
var myApp fyne.App

// main é o ponto de entrada da aplicação.
func main() {
	// Inicializa a aplicação Fyne
	myApp = app.NewWithID("com.github.gabrielmoura.nostrFilePublisher")
	myApp.UniqueID()
	myApp.SetIcon(icons.AppIcon())
	myWindow := myApp.NewWindow("Nostr Client")

	// Inicializa o estado da aplicação
	App = &model.AppState{
		HttpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		// Inicializa com alguns relays e servidores de exemplo
		BlossomServers: make(map[string]string),
		Relays:         make(map[string]*model.RelayStatus),
		Mutex:          &sync.Mutex{},
		UniqueID:       myApp.UniqueID(),
	}
	// Adiciona dados de exemplo
	App.Relays["wss://relay.damus.io"] = &model.RelayStatus{URL: "wss://relay.damus.io", Status: "Desconectado"}
	App.Relays["wss://relay.snort.social"] = &model.RelayStatus{URL: "wss://relay.snort.social", Status: "Desconectado"}

	App.BlossomServers["https://nostr.media"] = "https://nostr.media"

	// Configura o ícone e o menu da bandeja do sistema
	setupSystemTray(myApp, myWindow)

	// Cria as abas da aplicação
	tabs := container.NewAppTabs(
		container.NewTabItem("Principal", mainScreen()),
		container.NewTabItem("Vídeo", videoScreen(myWindow)),
		container.NewTabItem("Arquivos", fileScreen(myWindow)),
		container.NewTabItem("Configurações", settingsScreen(myWindow)),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	myWindow.SetContent(tabs)
	myWindow.Resize(fyne.NewSize(800, 600))
	myWindow.ShowAndRun()
}

// setupSystemTray configura o ícone e o menu da bandeja do sistema.
func setupSystemTray(a fyne.App, w fyne.Window) {
	if desk, ok := a.(desktop.App); ok {
		// Ícone de exemplo. Para funcionar, o arquivo 'icon.png' deve estar presente.
		// Como o ícone original estava vazio, usamos um padrão Fyne por enquanto.
		// icon := fyne.NewStaticResource("LoveIcon", iconData)
		m := fyne.NewMenu("Nostr",
			fyne.NewMenuItem("Mostrar", func() {
				w.Show()
			}),
			fyne.NewMenuItem("Ocultar", func() {
				w.Hide()
			}),
			fyne.NewMenuItem("Sair", func() {
				a.Quit()
			}),
		)
		desk.SetSystemTrayMenu(m)
		desk.SetSystemTrayIcon(icons.AppIcon()) // Descomente quando tiver dados de ícone válidos
	}
}

// mainScreen exibe a tela principal com o status dos relays e servidores.
func mainScreen() fyne.CanvasObject {
	// --- Lista de Relays ---
	// Usamos um container com scroll para garantir que a lista seja rolável.
	// O status da conexão agora é representado por um widget.Label.
	relayList := widget.NewList(
		func() int {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			return len(App.Relays)
		},
		func() fyne.CanvasObject {
			return container.New(layout.NewGridLayout(2), widget.NewLabel("URL"), widget.NewLabel("Status"))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			keys := make([]string, 0, len(App.Relays))
			for k := range App.Relays {
				keys = append(keys, k)
			}
			relay := App.Relays[keys[i]]
			grid := o.(*fyne.Container)
			grid.Objects[0].(*widget.Label).SetText(relay.URL)
			grid.Objects[1].(*widget.Label).SetText(relay.Status)
		},
	)

	// --- Lista de Servidores Blossom ---
	// Também em um container com scroll para consistência.
	blossomList := widget.NewList(
		func() int {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			return len(App.BlossomServers)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("Template Server")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			keys := make([]string, 0, len(App.BlossomServers))
			for k := range App.BlossomServers {
				keys = append(keys, k)
			}
			o.(*widget.Label).SetText(keys[i])
		},
	)

	// FUNCIONALIDADE IMPLEMENTADA: O container agora usa um Border layout para
	// preencher o espaço disponível. As listas estão dentro de um ScrollContainer
	// para serem roláveis e usam um grid para exibir URL e status.
	relayBox := container.NewBorder(
		widget.NewLabelWithStyle("Relays Conectados", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		container.NewScroll(relayList),
	)

	blossomBox := container.NewBorder(
		widget.NewLabelWithStyle("Servidores Blossom", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		container.NewScroll(blossomList),
	)

	// Divide a tela para melhor organização
	split := container.NewHSplit(relayBox, blossomBox)
	split.Offset = 0.5
	return split
}

// videoScreen constrói a UI para upload e publicação de vídeos.
func videoScreen(win fyne.Window) fyne.CanvasObject {
	var fileBlossom []model.BlossomResponse
	// Estados locais para a tela de vídeo

	preEvent := &model.PreEvent{
		Kind:    nostr.KindShortVideoEvent,
		PrivKey: App.Nsec,
	}

	// --- Widgets da UI ---
	titleEntry := widget.NewEntry()
	titleEntry.SetPlaceHolder("Título do vídeo...")

	summaryEntry := widget.NewEntry()
	summaryEntry.SetPlaceHolder("Resumo do vídeo...")

	descriptionEntry := widget.NewMultiLineEntry()
	descriptionEntry.SetPlaceHolder("Descrição detalhada...")
	descriptionEntry.Wrapping = fyne.TextWrapWord

	dateEntry := widget.NewDateEntry()
	dateEntry.SetPlaceHolder("Data de Publicação")

	imageEntry := widget.NewEntry()
	imageEntry.SetPlaceHolder("URL da Imagem de Capa (image)")

	thumbEntry := widget.NewEntry()
	thumbEntry.SetPlaceHolder("URL da Miniatura (thumb)")

	tagsLabel := widget.NewLabel("Tags (opcional):")
	tagsOpenDialogButton := widget.NewButton("Adicionar", func() {
		newTag := widget.NewEntry()
		tagSaveButton := widget.NewButton("Salvar", func() {
			tagsRaw := strings.TrimSpace(newTag.Text)
			if tagsRaw == "" {
				dialog.ShowInformation("Atenção", "Por favor, insira uma Tag válida.",
					win)
				return
			}

			preEvent.Tags = append(preEvent.Tags, tagsRaw)
			tagsLabel.SetText("Tags: " + strings.Join(preEvent.Tags, ", "))
			newTag.SetText("") // Limpa o campo após salvar
			dialog.ShowInformation("Sucesso", "Indexador adicionado com sucesso!", win)
		})
		dialog.NewCustom("Adicionar Tag", "Fechar", container.NewVBox(
			widget.NewLabel("Digite a Tag:"),
			newTag,
			tagSaveButton,
		), win).Show()
		log.Println("Botão de adicionar indexador clicado")
	})

	indexersLabel := ttwidget.NewLabel("Indexadores (opcional):")
	indexersLabel.SetToolTip("Indexadores são usados para categorizar e facilitar a busca de eventos.\nExemplos comuns incluem nomes de plataformas ou serviços relacionados ao conteúdo do vídeo.")

	indexerButton := widget.NewButton("Adicionar", func() {
		newIndexer := widget.NewEntry()
		indexerSaveButton := widget.NewButton("Salvar", func() {
			indexer := strings.TrimSpace(newIndexer.Text)
			if indexer == "" {
				dialog.ShowInformation("Atenção", "Por favor, insira uma Tag Index válida.",
					win)
				return
			}

			preEvent.Indexers = append(preEvent.Indexers, indexer)
			indexersLabel.SetText("Indexadores: " + strings.Join(preEvent.Indexers, ", "))
			newIndexer.SetText("") // Limpa o campo após salvar
			dialog.ShowInformation("Sucesso", "Indexador adicionado com sucesso!", win)
		})
		dialog.NewCustom("Adicionar Indexador", "Fechar", container.NewVBox(
			widget.NewLabel("Digite o indexador:"),
			newIndexer,
			indexerSaveButton,
		), win).Show()
		log.Println("Botão de adicionar indexador clicado")
	})

	nsfwCheck := widget.NewCheck("Conteudo Sensível", func(b bool) {
		if b {
			preEvent.Nsfw = b
		}
	})

	fileSizeLabel := widget.NewLabel("Tamanho do Arquivo: (selecione um arquivo)")
	bUrlsLabel := widget.NewLabel("Blossom URLs: (após upload)")
	eventOutput := widget.NewMultiLineEntry()
	eventOutput.SetPlaceHolder("O evento Nostr gerado aparecerá aqui...")
	eventOutput.Disable()

	videoTypeEntry := widget.NewSelect([]string{"Vídeo Curto (Kind 34235)", "Vídeo Longo (Kind 34236)"}, func(selected string) {
		if strings.Contains(selected, "Curto") {
			preEvent.Kind = nostr.KindShortVideoEvent
		} else {
			preEvent.Kind = nostr.KindVideoEvent
		}
		log.Println("Tipo de vídeo selecionado (Kind):", preEvent.Kind)
	})
	videoTypeEntry.SetSelectedIndex(0)

	// --- Botões e Ações ---
	var evt nostr.Event
	defineManualUrlButton := widget.NewButton("Definir URL Manualmente", func() {
		sha256Entry := widget.NewEntry()
		sizeEntry := widget.NewEntry()
		sha256Entry.SetPlaceHolder("b1674191a88ec5cdd733e4240a81803105dc412d6c6708d53ab94fc248f4f553")
		sizeEntry.SetPlaceHolder("184292")
		sha256Entry.Validator = func(s string) error {
			if len(s) != 64 {
				return fmt.Errorf("SHA-256 inválido, deve ter 64 caracteres")
			}
			_, err := fmt.Sscanf(s, "%x", new([32]byte))
			if err != nil {
				return fmt.Errorf("SHA-256 inválido: %v", err)
			}
			return nil
		}
		sizeEntry.Validator = func(s string) error {
			var size int64
			_, err := fmt.Sscanf(s, "%d", &size)
			if err != nil || size < 0 {
				return fmt.Errorf("tamanho inválido, deve ser um número positivo")
			}
			preEvent.Size = size
			return nil
		}
		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("https://example.com/meuvideo.mp4")
		urlEntry.SetText(preEvent.Path) // Preenche com o caminho atual se existir
		urlEntry.Validator = func(s string) error {
			if s == "" {
				return fmt.Errorf("a URL não pode estar vazia")
			}
			parsed, err := urlX.ParseRequestURI(s)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return fmt.Errorf("URL inválida")
			}
			return nil
		}
		urlSaveButton := widget.NewButton("Salvar", func() {
			err := urlEntry.Validate()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			fileBlossom = append(fileBlossom, model.BlossomResponse{
				URL: urlEntry.Text,
			})

			urlEntry.SetText("") // Limpa o campo após salvar
			bUrlsLabel.SetText("Blossom Link definido manualmente:\n" + fileBlossom[0].URL)

			mime, err := util.GetMimeFromUrl(App.HttpClient, fileBlossom[0].URL)
			if err != nil {
				log.Println("Erro ao detectar MIME da URL:", err)
				mime = "application/octet-stream"
			}
			preEvent.MimeType = mime
			dialog.ShowInformation("Sucesso", "URL definida com sucesso!", win)
		})
		dialog.NewCustom("Definir URL Manualmente", "Fechar", container.NewVBox(
			container.NewVBox(widget.NewLabel("Digite a URL do vídeo:"), urlEntry),
			widget.NewSeparator(),
			container.NewVBox(widget.NewLabel("Digite o SHA-256 do arquivo:"), sha256Entry),
			container.NewVBox(widget.NewLabel("Digite o tamanho do arquivo (em bytes):"), sizeEntry),

			urlSaveButton,
		), win).Show()
		log.Println("Botão de definir URL manual clicado")
	})
	selectFileButton := widget.NewButton("Selecionar Arquivo de Vídeo", func() {
		dialog.ShowFileOpen(func(file fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if file == nil {
				return
			}

			preEvent.Path = file.URI().Path()
			preEvent.MimeType = file.URI().MimeType()

			// Abre o arquivo para ler metadados
			f, err := os.Open(file.URI().Path())
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			defer f.Close()

			// Calcula o hash SHA-256 do conteúdo do arquivo
			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				dialog.ShowError(err, win)
				return
			}

			preEvent.Sha256 = fmt.Sprintf("%x", h.Sum(nil))
			log.Println("Hash SHA-256 do arquivo:", preEvent.Sha256)

			// Detecta o tipo MIME
			f.Seek(0, 0) // Volta ao início do arquivo
			buffer := make([]byte, 512)
			_, err = f.Read(buffer)
			if err != nil && err != io.EOF {
				dialog.ShowError(err, win)
				return
			}

			preEvent.MimeType = http.DetectContentType(buffer)
			log.Println("Tipo MIME do arquivo:", preEvent.MimeType)

			// Obtém o tamanho do arquivo
			stat, err := f.Stat()
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			preEvent.Size = stat.Size()

			if len(App.BlossomServers) >= 1 {
				fBlossom, errs := blossom.SendFile(App.HttpClient, *preEvent, *App)
				if len(errs) > 0 {
					var errMsgs []string
					for _, e := range errs {
						errMsgs = append(errMsgs, e.Error())
					}
					dialog.ShowError(fmt.Errorf("Erros ao enviar para Blossom:\n%s", strings.Join(errMsgs, "\n")), win)
					return
				}
				fileBlossom = fBlossom
			} else {
				dialog.ShowInformation("Atenção", "Nenhum servidor Blossom configurado. Por favor, adicione um servidor na aba Configurações.", win)
				return
			}
			//fBlossom
			if len(fileBlossom) > 0 {
				var fileURLs string
				for _, f := range fileBlossom {
					fileURLs += fmt.Sprintf("URL: %s\n", f.URL)
				}

				log.Println("URLs geradas pelo Blossom:", fileURLs)
				bUrlsLabel.SetText("Blossom Link gerado:\n" + fileURLs)
			}
			fileSizeLabel.SetText(fmt.Sprintf("Tamanho: %d bytes | MIME: %s", preEvent.Size, preEvent.MimeType))
		}, win)
	})

	generateEventButton := widget.NewButton("Gerar Evento", func() {
		// Verifica se o arquivo foi selecionado ou se a URL foi definida manualmente
		if len(fileBlossom) == 0 && preEvent.Path == "" {
			dialog.ShowInformation("Atenção", "Por favor, selecione um arquivo primeiro.", win)
			return
		}
		if App.Nsec == "" {
			dialog.ShowInformation("Atenção", "Por favor, configure sua chave NSEC na aba de Configurações.", win)
			return
		}

		// Monta as tags do evento Nostr
		t := nostr.Tags{
			nostr.Tag{"d", fmt.Sprintf("%s.%d", App.UniqueID, time.Now().Unix())},
			nostr.Tag{"m", preEvent.MimeType},
			nostr.Tag{"url", fileBlossom[0].URL},
		}
		for _, r := range App.Relays {
			t = append(t, nostr.Tag{"r", r.URL})
		}
		if preEvent.Sha256 != "" {
			t = append(t, nostr.Tag{"x", preEvent.Sha256})
		}
		if preEvent.Size > 0 {
			t = append(t, nostr.Tag{"size", fmt.Sprintf("%d", preEvent.Size)})
		}

		if len(fileBlossom) > 1 {
			for _, f := range fileBlossom[1:] {
				t = append(t, nostr.Tag{"fallback", f.URL})
			}
		}
		if titleEntry.Text != "" {
			t = append(t, nostr.Tag{"title", titleEntry.Text})
		}
		if summaryEntry.Text != "" {
			t = append(t, nostr.Tag{"summary", summaryEntry.Text})
		}
		if imageEntry.Text != "" {
			t = append(t, nostr.Tag{"image", imageEntry.Text})
		}
		if thumbEntry.Text != "" {
			t = append(t, nostr.Tag{"thumb", thumbEntry.Text})
			// FUNCIONALIDADE IMPLEMENTADA: Geração de BlurHash a partir da URL da thumbnail
			go func() {
				resp, err := App.HttpClient.Get(thumbEntry.Text)
				if err != nil {
					log.Println("Erro ao buscar thumbnail:", err)
					return
				}
				defer resp.Body.Close()
				img, _, err := image.Decode(resp.Body)
				if err != nil {
					log.Println("Erro ao decodificar imagem da thumbnail:", err)
					return
				}
				hash, err := blurhash.Encode(4, 3, img)
				if err != nil {
					log.Println("Erro ao gerar BlurHash:", err)
					return
				}

				preEvent.BlurHash = hash
				log.Println("BlurHash gerado:", preEvent.BlurHash)
				// A tag será adicionada antes de publicar
			}()
		}
		if dateEntry.Text != "" {
			// Adiciona a data de publicação no formato ISO 8601
			publishedAt, err := time.Parse("01/02/2006", dateEntry.Text)
			if err != nil {
				log.Println("Erro ao analisar a data:", err)
				dialog.ShowError(err, win)
				return
			}
			t = append(t, nostr.Tag{"published_at", fmt.Sprintf("%d", publishedAt.Unix())})
		}

		for _, tag := range preEvent.Tags {
			trimmedTag := strings.TrimSpace(tag)
			if trimmedTag != "" {
				t = append(t, nostr.Tag{"t", trimmedTag})
			}
		}
		for _, i := range preEvent.Indexers {
			trimmedI := strings.TrimSpace(i)
			if trimmedI != "" {
				t = append(t, nostr.Tag{"i", trimmedI})
			}
		}
		if preEvent.Nsfw {
			t = append(t, nostr.Tag{"content-warning"})
		}

		// Cria o evento Nostr
		evt = nostr.Event{
			Content:   descriptionEntry.Text,
			Tags:      t,
			CreatedAt: nostr.Now(),
			PubKey:    App.Npub,
			Kind:      preEvent.Kind,
		}
		if err := evt.Sign(App.Nsec); err != nil {
			dialog.ShowError(fmt.Errorf("Erro ao assinar o evento: %w", err), win)
			return
		}

		//eventOutput.SetText(fmt.Sprintf("ID: %s\nKind: %d\nPubKey: %s...", evt.ID, evt.Kind, evt.PubKey[:10]))
		log.Println("Evento Nostr: ", evt.String())
		eventOutput.SetText(evt.String())
		eventOutput.Enable()
		dialog.ShowInformation("Sucesso", "Evento gerado com sucesso!", win)
	})

	publishEventButton := widget.NewButton("Publicar Evento", func() {
		if evt.ID == "" {
			dialog.ShowInformation("Atenção", "Por favor, gere o evento primeiro.", win)
			return
		}
		// Adiciona a tag blurhash se foi gerada
		if preEvent.BlurHash != "" {
			evt.Tags = append(evt.Tags, nostr.Tag{"blurhash", preEvent.BlurHash})
		}

		// FUNCIONALIDADE IMPLEMENTADA: Diálogo de status da publicação.
		// Mostra uma lista de relays e o resultado do envio para cada um.
		statusMap := make(map[string]string)
		var wg sync.WaitGroup
		App.Mutex.Lock()
		for url := range App.Relays {
			wg.Add(1)
			go func(relayURL string) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				relay, err := nostr.RelayConnect(ctx, relayURL)
				if err != nil {
					statusMap[relayURL] = "Falha ao conectar"
					return
				}
				err = relay.Publish(ctx, evt)
				if err != nil {
					statusMap[relayURL] = fmt.Sprintf("Falha: %v", err)
				} else {
					statusMap[relayURL] = "Sucesso"
				}
				relay.Close()
			}(url)
		}
		App.Mutex.Unlock()
		wg.Wait()

		// Cria a UI para o diálogo de resultados
		var resultsData []string
		for k, v := range statusMap {
			resultsData = append(resultsData, fmt.Sprintf("%s: %s", k, v))
		}
		resultsList := widget.NewList(
			func() int { return len(resultsData) },
			func() fyne.CanvasObject { return widget.NewLabel("") },
			func(i widget.ListItemID, o fyne.CanvasObject) {
				o.(*widget.Label).SetText(resultsData[i])
			},
		)
		resultDialog := dialog.NewCustom("Resultado da Publicação", "Fechar", container.NewScroll(resultsList), win)
		resultDialog.Resize(fyne.NewSize(400, 300))
		resultDialog.Show()
	})
	resetFormButton := widget.NewButton("Limpar Formulário", func() {
		fileBlossom = nil
		titleEntry.SetText("")
		summaryEntry.SetText("")
		descriptionEntry.SetText("")
		tagsLabel.SetText("Tags (opcional):")
		preEvent = &model.PreEvent{
			Kind:    nostr.KindShortVideoEvent,
			PrivKey: App.Nsec,
		}
		videoTypeEntry.SetSelectedIndex(0)
		imageEntry.SetText("")
		thumbEntry.SetText("")

		dateEntry.SetDate(nil)
		dateEntry.SetValidationError(nil)
		//dateEntry é opcional, deve ser limpo, mas não obrigatório
		nsfwCheck.SetChecked(false)
		fileSizeLabel.SetText("Tamanho do Arquivo: (selecione um arquivo)")
		bUrlsLabel.SetText("Blossom URLs: (após upload)")
		eventOutput.SetText("")
		eventOutput.Disable()
		titleEntry.FocusGained()
	})

	// --- Layout da Tela ---
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Tipo de Vídeo", Widget: videoTypeEntry},
			{Text: "Título", Widget: titleEntry},
			{Text: "Resumo", Widget: summaryEntry},
			{Text: "Tags", Widget: container.NewHBox(tagsLabel, tagsOpenDialogButton)},
			{Text: "NSFW", Widget: nsfwCheck},
			{Text: "URL Imagem", Widget: imageEntry},
			{Text: "URL Thumbnail", Widget: thumbEntry},
			{Text: "Data de Publicação", Widget: dateEntry},
			{Text: "Indexadores", Widget: container.NewHBox(fynetooltip.AddWindowToolTipLayer(indexersLabel, win.Canvas()), indexerButton)},
		},
	}

	inputContainer := container.NewVBox(
		container.NewCenter(container.NewHBox(selectFileButton, defineManualUrlButton)),
		fileSizeLabel,
		bUrlsLabel,
		widget.NewSeparator(),
		form,
		widget.NewLabel("Descrição"),
		descriptionEntry,
	)

	actionsContainer := container.NewVBox(
		container.NewCenter(container.NewHBox(generateEventButton, resetFormButton, publishEventButton)),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Evento Gerado", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		eventOutput,
	)

	return container.NewBorder(nil, actionsContainer, nil, nil, container.NewScroll(inputContainer))
}

func fileScreen(win fyne.Window) fyne.CanvasObject {
	preEvent := &model.PreEvent{
		Kind:    nostr.KindFileMetadata,
		PrivKey: App.Nsec,
	}
	var evt nostr.Event
	var fileBlossom []model.BlossomResponse

	// --- Widgets da UI ---
	titleEntry := widget.NewEntry()
	titleEntry.SetPlaceHolder("Título do vídeo...")

	dateEntry := widget.NewDateEntry()
	dateEntry.SetPlaceHolder("Data de Publicação")

	summaryEntry := widget.NewEntry()
	summaryEntry.SetPlaceHolder("Resumo do arquivo...")

	descriptionEntry := widget.NewMultiLineEntry()
	descriptionEntry.SetPlaceHolder("Descrição detalhada...")

	tagsLabel := widget.NewLabel("Tags (opcional):")
	tagsOpenDialogButton := widget.NewButton("Adicionar", func() {
		newTag := widget.NewEntry()
		tagSaveButton := widget.NewButton("Salvar", func() {
			tagsRaw := strings.TrimSpace(newTag.Text)
			if tagsRaw == "" {
				dialog.ShowInformation("Atenção", "Por favor, insira uma Tag válida.",
					win)
				return
			}

			preEvent.Tags = append(preEvent.Tags, tagsRaw)
			tagsLabel.SetText("Tags: " + strings.Join(preEvent.Tags, ", "))
			newTag.SetText("") // Limpa o campo após salvar
			dialog.ShowInformation("Sucesso", "Indexador adicionado com sucesso!", win)
		})
		dialog.NewCustom("Adicionar Tag", "Fechar", container.NewVBox(
			widget.NewLabel("Digite a Tag:"),
			newTag,
			tagSaveButton,
		), win).Show()
		log.Println("Botão de adicionar indexador clicado")
	})

	indexersLabel := ttwidget.NewLabel("Indexadores (opcional):")
	indexersLabel.SetToolTip("Indexadores são usados para categorizar e facilitar a busca de eventos.\nExemplos comuns incluem nomes de plataformas ou serviços relacionados ao conteúdo do vídeo.")

	indexerButton := widget.NewButton("Adicionar", func() {
		newIndexer := widget.NewEntry()
		indexerSaveButton := widget.NewButton("Salvar", func() {
			indexer := strings.TrimSpace(newIndexer.Text)
			if indexer == "" {
				dialog.ShowInformation("Atenção", "Por favor, insira uma Tag Index válida.",
					win)
				return
			}

			preEvent.Indexers = append(preEvent.Indexers, indexer)
			indexersLabel.SetText("Indexadores: " + strings.Join(preEvent.Indexers, ", "))
			newIndexer.SetText("") // Limpa o campo após salvar
			dialog.ShowInformation("Sucesso", "Indexador adicionado com sucesso!", win)
		})
		dialog.NewCustom("Adicionar Indexador", "Fechar", container.NewVBox(
			widget.NewLabel("Digite o indexador:"),
			newIndexer,
			indexerSaveButton,
		), win).Show()
		log.Println("Botão de adicionar indexador clicado")
	})

	nsfwCheck := widget.NewCheck("Conteudo Sensível", func(b bool) {
		if b {
			preEvent.Nsfw = b
		}
	})

	fileSizeLabel := widget.NewLabel("Tamanho do Arquivo: (selecione um arquivo)")

	eventOutput := widget.NewMultiLineEntry()
	eventOutput.SetPlaceHolder("O evento Nostr gerado aparecerá aqui...")
	eventOutput.Disable()

	selectFileButton := widget.NewButton("Selecionar Arquivo", func() {
		dialog.ShowFileOpen(func(file fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if file == nil {
				return
			}
			preEvent.Path = file.URI().Path()
			preEvent.MimeType = file.URI().MimeType()
			f, err := os.Open(file.URI().Path())
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			defer f.Close()

			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				dialog.ShowError(err, win)
				return
			}
			preEvent.Sha256 = fmt.Sprintf("%x", h.Sum(nil))

			f.Seek(0, 0)
			buffer := make([]byte, 512)
			f.Read(buffer)
			preEvent.MimeType = http.DetectContentType(buffer)

			stat, _ := f.Stat()
			preEvent.Size = stat.Size()
			fileSizeLabel.SetText(fmt.Sprintf("Tamanho: %d bytes | MIME: %s", preEvent.Size, preEvent.MimeType))

			// Envio ao servidor Blossom
			if len(App.BlossomServers) >= 1 {
				fBlossom, errs := blossom.SendFile(App.HttpClient, *preEvent, *App)
				if len(errs) > 0 {
					var errMsgs []string
					for _, e := range errs {
						errMsgs = append(errMsgs, e.Error())
					}
					dialog.ShowError(fmt.Errorf("Erros ao enviar para Blossom:\n%s", strings.Join(errMsgs, "\n")), win)
					return
				}
				fileBlossom = fBlossom
			} else {
				dialog.ShowInformation("Atenção", "Nenhum servidor Blossom configurado. Por favor, adicione um servidor na aba Configurações.", win)
				return
			}
			if len(fileBlossom) > 0 {
				var fileURLs string
				for _, f := range fileBlossom {
					fileURLs += fmt.Sprintf("URL: %s\n", f.URL)
				}

				log.Println("URLs geradas pelo Blossom:", fileURLs)
				fileSizeLabel.SetText("Blossom Link gerado:\n" + fileURLs)
			}
		}, win)
	})

	publishButton := widget.NewButton("Gerar e Publicar", func() {
		if preEvent.Path == "" {
			dialog.ShowInformation("Atenção", "Por favor, selecione um arquivo primeiro.", win)
			return
		}
		if App.Nsec == "" {
			dialog.ShowInformation("Atenção", "Por favor, configure sua chave NSEC.", win)
			return
		}

		// Monta as tags
		t := nostr.Tags{
			nostr.Tag{"d", fmt.Sprintf("%s.%d", App.UniqueID, time.Now().Unix())},
			nostr.Tag{"x", preEvent.Sha256},
			nostr.Tag{"m", preEvent.MimeType},
			nostr.Tag{"size", fmt.Sprintf("%d", preEvent.Size)},
			nostr.Tag{"url", fileBlossom[0].URL},
		}
		if summaryEntry.Text != "" {
			t = append(t, nostr.Tag{"summary", summaryEntry.Text})
		}
		if titleEntry.Text != "" {
			t = append(t, nostr.Tag{"title", titleEntry.Text})
		}
		if preEvent.Nsfw {
			t = append(t, nostr.Tag{"content-warning"})
		}
		for _, tag := range preEvent.Tags {
			trimmedTag := strings.TrimSpace(tag)
			if trimmedTag != "" {
				t = append(t, nostr.Tag{"t", trimmedTag})
			}
		}
		if len(fileBlossom) > 1 {
			for _, f := range fileBlossom[1:] {
				t = append(t, nostr.Tag{"fallback", f.URL})
			}
		}
		for _, i := range preEvent.Indexers {
			trimmedI := strings.TrimSpace(i)
			if trimmedI != "" {
				t = append(t, nostr.Tag{"i", trimmedI})
			}
		}
		for _, r := range App.Relays {
			t = append(t, nostr.Tag{"r", r.URL})
		}
		if dateEntry.Text != "" {
			// Adiciona a data de publicação no formato ISO 8601
			publishedAt, err := time.Parse("01/02/2006", dateEntry.Text)
			if err != nil {
				log.Println("Erro ao analisar a data:", err)
				dialog.ShowError(err, win)
				return
			}
			t = append(t, nostr.Tag{"published_at", fmt.Sprintf("%d", publishedAt.Unix())})
		}

		// Decodifica a NSEC e cria o evento
		evt = nostr.Event{
			Content:   descriptionEntry.Text,
			Tags:      t,
			CreatedAt: nostr.Now(),
			PubKey:    App.Npub,
			Kind:      preEvent.Kind,
		}
		evt.Sign(App.Nsec)
		eventOutput.SetText(fmt.Sprintf("ID: %s\nKind: %d", evt.ID, evt.Kind))
		eventOutput.Enable()

		// Publica o evento (lógica similar à tela de vídeo)
		// ... (a lógica de publicação pode ser extraída para uma função helper para evitar repetição)
		dialog.ShowInformation("Publicado", "Evento de arquivo enviado com sucesso (simulação)!", win)
	})

	resetFormButton := widget.NewButton("Limpar Formulário", func() {
		titleEntry.SetText("")
		summaryEntry.SetText("")
		descriptionEntry.SetText("")
		tagsLabel.SetText("Tags (opcional):")
		preEvent = &model.PreEvent{
			Kind:    nostr.KindFileMetadata,
			PrivKey: App.Nsec,
		}
		fileSizeLabel.SetText("Tamanho do Arquivo: (selecione um arquivo)")
		eventOutput.SetText("")
		eventOutput.Disable()
		fileBlossom = nil // Limpa os links do Blossom
		dateEntry.SetDate(nil)
		dateEntry.SetValidationError(nil)
	})

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Título", Widget: titleEntry},
			{Text: "Resumo", Widget: summaryEntry},
			{Text: "Tags", Widget: container.NewHBox(tagsLabel, tagsOpenDialogButton)},
			{Text: "NSFW", Widget: nsfwCheck},
			{Text: "Indexadores", Widget: container.NewHBox(fynetooltip.AddWindowToolTipLayer(indexersLabel, win.Canvas()), indexerButton)},
			{Text: "Data de Publicação", Widget: dateEntry},
		},
	}
	inputContainer := container.NewVBox(
		selectFileButton,
		fileSizeLabel,
		form,
		widget.NewLabel("Descrição"),
		descriptionEntry,
	)
	actionsContainer := container.NewVBox(
		container.NewCenter(container.NewHBox(publishButton, resetFormButton)),
		widget.NewLabelWithStyle("Evento Gerado", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		eventOutput,
	)

	return container.NewBorder(nil, actionsContainer, nil, nil, container.NewScroll(inputContainer))
}

// settingsScreen permite a configuração de relays, servidores e da chave privada.
func settingsScreen(win fyne.Window) fyne.CanvasObject {
	blossomServerEntry := widget.NewEntry()
	blossomServerEntry.SetPlaceHolder("https://blossom.server.com")
	var blossomListWidget *widget.List
	var selectedBlossomID widget.ListItemID = -1

	// --- Gerenciamento de Servidores Blossom ---
	blossomListWidget = widget.NewList(
		func() int {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			return len(App.BlossomServers)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			keys := make([]string, 0, len(App.BlossomServers))
			for k := range App.BlossomServers {
				keys = append(keys, k)
			}
			o.(*widget.Label).SetText(keys[i])
		},
	)
	blossomListWidget.OnSelected = func(id widget.ListItemID) {
		selectedBlossomID = id
		App.Mutex.Lock()
		defer App.Mutex.Unlock()
		keys := make([]string, 0, len(App.BlossomServers))
		for k := range App.BlossomServers {
			keys = append(keys, k)
		}
		blossomServerEntry.SetText(keys[id])
	}
	blossomListWidget.OnUnselected = func(id widget.ListItemID) {
		if selectedBlossomID == id {
			selectedBlossomID = -1
			blossomServerEntry.SetText("")
		}
	}

	// FUNCIONALIDADE IMPLEMENTADA: CRUD (Create, Read, Update, Delete) para servidores Blossom.
	addBlossomButton := widget.NewButton("Adicionar", func() {
		url := strings.TrimSpace(blossomServerEntry.Text)
		if url == "" {
			return
		}
		urlXD, err := urlX.Parse(url)
		if err != nil || (urlXD.Scheme != "http" && urlXD.Scheme != "https") {
			dialog.ShowError(fmt.Errorf("URL inválida: %s", url), win)
			return
		}
		App.Mutex.Lock()
		App.BlossomServers[url] = url
		App.Mutex.Unlock()
		blossomListWidget.Refresh()
		blossomListWidget.UnselectAll()
		blossomServerEntry.SetText("")
	})
	updateBlossomButton := widget.NewButton("Atualizar", func() {
		if selectedBlossomID == -1 {
			dialog.ShowInformation("Atenção", "Selecione um servidor Blossom para atualizar.", win)
			return
		}
		newURL := strings.TrimSpace(blossomServerEntry.Text)
		if newURL == "" {
			return
		}
		App.Mutex.Lock()
		keys := make([]string, 0, len(App.BlossomServers))
		for k := range App.BlossomServers {
			keys = append(keys, k)
		}
		oldURL := keys[selectedBlossomID]
		if oldURL != newURL {
			delete(App.BlossomServers, oldURL)
			App.BlossomServers[newURL] = newURL
		}
		App.Mutex.Unlock()
		blossomListWidget.Refresh()
	})
	deleteBlossomButton := widget.NewButton("Deletar", func() {
		if selectedBlossomID == -1 {
			dialog.ShowInformation("Atenção", "Selecione um servidor Blossom para deletar.", win)
			return
		}
		App.Mutex.Lock()
		keys := make([]string, 0, len(App.BlossomServers))
		for k := range App.BlossomServers {
			keys = append(keys, k)
		}
		urlToDelete := keys[selectedBlossomID]
		delete(App.BlossomServers, urlToDelete)
		App.Mutex.Unlock()
		blossomListWidget.Refresh()
		blossomListWidget.UnselectAll()
		blossomServerEntry.SetText("")
	})
	blossomBox := container.NewBorder(
		widget.NewLabelWithStyle("Servidores Blossom", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.NewVBox(blossomServerEntry, container.NewHBox(addBlossomButton, updateBlossomButton, deleteBlossomButton)),
		nil, nil,
		container.NewScroll(blossomListWidget),
	)

	// --- Gerenciamento de Relays ---
	relayEntry := widget.NewEntry()
	relayEntry.SetPlaceHolder("wss://...")
	var relayListWidget *widget.List
	var selectedRelayID widget.ListItemID = -1

	relayListWidget = widget.NewList(
		func() int {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			return len(App.Relays)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			App.Mutex.Lock()
			defer App.Mutex.Unlock()
			keys := make([]string, 0, len(App.Relays))
			for k := range App.Relays {
				keys = append(keys, k)
			}
			o.(*widget.Label).SetText(keys[i])
		},
	)
	relayListWidget.OnSelected = func(id widget.ListItemID) {
		selectedRelayID = id
		App.Mutex.Lock()
		defer App.Mutex.Unlock()
		keys := make([]string, 0, len(App.Relays))
		for k := range App.Relays {
			keys = append(keys, k)
		}
		relayEntry.SetText(keys[id])
	}
	relayListWidget.OnUnselected = func(id widget.ListItemID) {
		if selectedRelayID == id {
			selectedRelayID = -1
			relayEntry.SetText("")
		}
	}

	// FUNCIONALIDADE IMPLEMENTADA: CRUD (Create, Read, Update, Delete) para relays.
	addRelayButton := widget.NewButton("Adicionar", func() {
		url := strings.TrimSpace(relayEntry.Text)
		if url == "" {
			return
		}
		urlXD, err := urlX.Parse(url)
		if err != nil || (urlXD.Scheme != "ws" && urlXD.Scheme != "wss") {
			dialog.ShowError(fmt.Errorf("URL inválida: %s", url), win)
			return
		}

		App.Mutex.Lock()
		App.Relays[url] = &model.RelayStatus{URL: url, Status: "Desconectado"}
		App.Mutex.Unlock()
		relayListWidget.Refresh()
		relayListWidget.UnselectAll()
		relayEntry.SetText("")
	})
	updateRelayButton := widget.NewButton("Atualizar", func() {
		if selectedRelayID == -1 {
			dialog.ShowInformation("Atenção", "Selecione um relay para atualizar.", win)
			return
		}
		newURL := strings.TrimSpace(relayEntry.Text)
		if newURL == "" {
			return
		}
		App.Mutex.Lock()
		keys := make([]string, 0, len(App.Relays))
		for k := range App.Relays {
			keys = append(keys, k)
		}
		oldURL := keys[selectedRelayID]
		if oldURL != newURL {
			delete(App.Relays, oldURL)
			App.Relays[newURL] = &model.RelayStatus{URL: newURL, Status: "Desconectado"}
		}
		App.Mutex.Unlock()
		relayListWidget.Refresh()
	})
	deleteRelayButton := widget.NewButton("Deletar", func() {
		if selectedRelayID == -1 {
			dialog.ShowInformation("Atenção", "Selecione um relay para deletar.", win)
			return
		}
		App.Mutex.Lock()
		keys := make([]string, 0, len(App.Relays))
		for k := range App.Relays {
			keys = append(keys, k)
		}
		urlToDelete := keys[selectedRelayID]
		delete(App.Relays, urlToDelete)
		App.Mutex.Unlock()
		relayListWidget.Refresh()
		relayListWidget.UnselectAll()
		relayEntry.SetText("")
	})

	relayBox := container.NewBorder(
		widget.NewLabelWithStyle("Relays", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		container.NewVBox(relayEntry, container.NewHBox(addRelayButton, updateRelayButton, deleteRelayButton)),
		nil, nil,
		container.NewScroll(relayListWidget),
	)

	// --- NSEC ---
	// FUNCIONALIDADE IMPLEMENTADA: Salva a NSEC no estado global da aplicação.
	nsecEntry := widget.NewPasswordEntry()
	nsecEntry.SetPlaceHolder("nsec...")
	if App.Nsec != "" {
		nsecEntry.SetText(App.Nsec)
	}
	saveNsecButton := widget.NewButton("Salvar NSEC", func() {
		nsec := strings.TrimSpace(nsecEntry.Text)
		_, hexNsec, err := nip19.Decode(nsec)
		if err != nil {
			dialog.ShowError(fmt.Errorf("NSEC inválida: %w", err), win)
			return
		}
		hexNpub, err := nostr.GetPublicKey(hexNsec.(string)) // Verifica se a chave é válida
		if err != nil {
			dialog.ShowError(fmt.Errorf("Chave NSEC inválida: %w", err),
				win)
			return
		}
		App.Mutex.Lock()
		App.Nsec = hexNsec.(string)
		App.Npub = hexNpub
		App.Mutex.Unlock()
		log.Println("NSEC Salva com sucesso.")
		dialog.ShowInformation("Sucesso", "Chave NSEC foi salva.", win)
	})

	nsecBox := container.NewVBox(
		widget.NewLabelWithStyle("Chave Privada (NSEC)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nsecEntry,
		saveNsecButton,
	)

	return container.NewVBox(relayBox, widget.NewSeparator(), blossomBox, widget.NewSeparator(), nsecBox)
}
