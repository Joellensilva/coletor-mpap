package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
	"strings"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)


type crawler struct {
	collectionTimeout time.Duration
	timeBetweenSteps  time.Duration
	year              string
	month             string
	output            string
}

func (c crawler) crawl() ([]string, error) {
	// Chromedp setup.
	log.SetOutput(os.Stderr) // Enviando logs para o stderr para não afetar a execução do coletor.
	alloc, allocCancel := chromedp.NewExecAllocator(
		context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true), // mude para false para executar com navegador visível.
			chromedp.NoSandbox,
			chromedp.DisableGPU,
		)...,
	)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(
		alloc,
		chromedp.WithLogf(log.Printf), // remover comentário para depurar
	)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, c.collectionTimeout)
	defer cancel()

	// NOTA IMPORTANTE: os prefixos dos nomes dos arquivos tem que ser igual
	// ao esperado no parser MPAP.

	// Contracheque
	log.Printf("Realizando seleção (%s/%s)...", c.month, c.year)
	if err := c.abreCaixaDialogo(ctx, "contra"); err != nil {
		log.Fatalf("Erro no setup:%v", err)
	}
	log.Printf("Seleção realizada com sucesso!\n")
	cqFname := c.downloadFilePath("contracheque")
	log.Printf("Fazendo download do contracheque (%s)...", cqFname)
	if err := c.exportaPlanilha(ctx, cqFname); err != nil {
		log.Fatalf("Erro fazendo download do contracheque: %v", err)
	}
	log.Printf("Download realizado com sucesso!\n")

	// Indenizações
	if c.year != "2018" {
		log.Printf("Realizando seleção (%s/%s)...", c.month, c.year)
		if err := c.abreCaixaDialogo(ctx, "inde"); err != nil {
			log.Fatalf("Erro no setup:%v", err)
		}
		log.Printf("Seleção realizada com sucesso!\n")
		iFname := c.downloadFilePath("indenizatorias")
		log.Printf("Fazendo download das indenizações (%s)...", iFname)
		if err := c.exportaPlanilha(ctx, iFname); err != nil {
			log.Fatalf("Erro fazendo download dos indenizações: %v", err)
		}
		log.Printf("Download realizado com sucesso!\n")

		return []string{cqFname, iFname}, nil
	}

	// Retorna caminhos completos dos arquivos baixados.
	return []string{cqFname}, nil
}

func (c crawler) downloadFilePath(prefix string) string {
	return filepath.Join(c.output, fmt.Sprintf("membros-ativos-%s-%s-%s.xls", prefix, c.month, c.year))
}

func (c crawler) abreCaixaDialogo(ctx context.Context, tipo string) error {
	var baseURL string
	var selectYear string
	if tipo == "contra"{
		baseURL = "http://www.mpap.mp.br/transparencia/index.php?pg=consulta_folha_membros_ativos"
		selectYear = `//select[@id="ano"]`
	} else {
		baseURL = "https://www.mpap.mp.br/transparencia/index.php?pg=consulta_verbas_indenizatorias"
		selectYear = `//select[@id="ano_verbas"]`
	}

	return chromedp.Run(ctx,
		chromedp.Navigate(baseURL),
		chromedp.Sleep(c.timeBetweenSteps),

		// Seleciona ano
		chromedp.SetValue(selectYear, c.year, chromedp.BySearch),
		chromedp.Sleep(c.timeBetweenSteps),

		// Seleciona mes
		chromedp.SetValue(`//select[@id="mes"]`, strings.TrimPrefix(c.month, "0"), chromedp.BySearch, chromedp.NodeVisible),
		chromedp.Sleep(c.timeBetweenSteps),

		// Busca
		chromedp.Click(`//*[@id="enviar"]`, chromedp.BySearch, chromedp.NodeVisible),
		chromedp.Sleep(c.timeBetweenSteps),
		
		// Altera o diretório de download
		browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllowAndName).
			WithDownloadPath(c.output).
			WithEventsEnabled(true),
	)
}

// exportaPlanilha clica no botão correto para exportar para excel, espera um tempo para download renomeia o arquivo.
func (c crawler) exportaPlanilha(ctx context.Context, fName string) error {
	if strings.Contains(fName, "contracheque") {
		chromedp.Run(ctx,
			// Clica no botão de download 
			chromedp.Click(`/html/body/div[1]/center/div/fieldset/div[3]/form/button[2]`, chromedp.BySearch, chromedp.NodeVisible),
			chromedp.Sleep(c.timeBetweenSteps),
		)
	} else {
		chromedp.Run(ctx,
			// Clica no botão de download 
			chromedp.Click(`/html/body/div[1]/center/div/fieldset/div/form/button[2]`, chromedp.BySearch, chromedp.NodeVisible),
			chromedp.Sleep(c.timeBetweenSteps),
		)
	}
	
	if err := nomeiaDownload(c.output, fName); err != nil {
		return fmt.Errorf("erro renomeando arquivo (%s): %v", fName, err)
	}
	if _, err := os.Stat(fName); os.IsNotExist(err) {
		return fmt.Errorf("download do arquivo de %s não realizado", fName)
	}
	return nil
}

// nomeiaDownload dá um nome ao último arquivo modificado dentro do diretório
// passado como parâmetro nomeiaDownload dá pega um arquivo
func nomeiaDownload(output, fName string) error {
	// Identifica qual foi o ultimo arquivo
	files, err := os.ReadDir(output)
	if err != nil {
		return fmt.Errorf("erro lendo diretório %s: %v", output, err)
	}
	var newestFPath string
	var newestTime int64 = 0
	for _, f := range files {
		fPath := filepath.Join(output, f.Name())
		fi, err := os.Stat(fPath)
		if err != nil {
			return fmt.Errorf("erro obtendo informações sobre arquivo %s: %v", fPath, err)
		}
		currTime := fi.ModTime().Unix()
		if currTime > newestTime {
			newestTime = currTime
			newestFPath = fPath
		}
	}
	// Renomeia o ultimo arquivo modificado.
	if err := os.Rename(newestFPath, fName); err != nil {
		return fmt.Errorf("erro renomeando último arquivo modificado (%s)->(%s): %v", newestFPath, fName, err)
	}
	return nil
}